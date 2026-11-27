package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

const gzipMinSize = 1024

var gzipWriterPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		return w
	},
}

type gzipResponseWriter struct {
	gin.ResponseWriter
	status      int
	size        int
	wroteHeader bool
	decided     bool
	compress    bool
	buf         bytes.Buffer
	gz          *gzip.Writer
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if code <= 0 || w.wroteHeader {
		return
	}
	w.status = code
	w.wroteHeader = true
	if code < 200 || (code >= 300 && code < 400) || code == http.StatusNoContent {
		w.decided = true
		w.compress = false
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *gzipResponseWriter) WriteHeaderNow() {
	if !w.decided {
		if w.status == 0 {
			w.status = http.StatusOK
		}
		w.decide(false)
	}
	w.ResponseWriter.WriteHeaderNow()
}

func (w *gzipResponseWriter) Status() int {
	if w.status != 0 {
		return w.status
	}
	return w.ResponseWriter.Status()
}

func (w *gzipResponseWriter) Size() int {
	if w.size > 0 {
		return w.size
	}
	return w.ResponseWriter.Size()
}

func (w *gzipResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *gzipResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.decided {
		if w.compress && w.gz != nil {
			n, err := w.gz.Write(p)
			w.size += n
			return n, err
		}
		n, err := w.ResponseWriter.Write(p)
		w.size += n
		return n, err
	}

	if w.buf.Len()+len(p) < gzipMinSize {
		n, err := w.buf.Write(p)
		w.size += n
		return n, err
	}

	w.decide(true)
	if w.compress && w.gz != nil {
		if w.buf.Len() > 0 {
			_, _ = w.gz.Write(w.buf.Bytes())
			w.buf.Reset()
		}
		n, err := w.gz.Write(p)
		w.size += n
		return n, err
	}

	if w.buf.Len() > 0 {
		_, _ = w.ResponseWriter.Write(w.buf.Bytes())
		w.buf.Reset()
	}
	n, err := w.ResponseWriter.Write(p)
	w.size += n
	return n, err
}

func (w *gzipResponseWriter) Flush() {
	if !w.decided {
		w.decide(w.buf.Len() >= gzipMinSize)
		if w.compress && w.gz != nil {
			if w.buf.Len() > 0 {
				_, _ = w.gz.Write(w.buf.Bytes())
				w.buf.Reset()
			}
		} else if w.buf.Len() > 0 {
			_, _ = w.ResponseWriter.Write(w.buf.Bytes())
			w.buf.Reset()
		}
	}
	if w.compress && w.gz != nil {
		_ = w.gz.Flush()
	}
	w.ResponseWriter.Flush()
}

func (w *gzipResponseWriter) decide(allowCompress bool) {
	if w.decided {
		return
	}
	w.decided = true
	if w.status == 0 {
		w.status = http.StatusOK
	}

	if !allowCompress || w.Header().Get("Content-Encoding") != "" {
		w.compress = false
		w.ResponseWriter.WriteHeader(w.status)
		return
	}

	ct := strings.ToLower(strings.TrimSpace(w.Header().Get("Content-Type")))
	if ct == "" && w.buf.Len() > 0 {
		ct = strings.ToLower(http.DetectContentType(w.buf.Bytes()))
	}
	if !isCompressibleType(ct) {
		w.compress = false
		w.ResponseWriter.WriteHeader(w.status)
		return
	}

	w.compress = true
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Encoding", "gzip")
	if !strings.Contains(strings.ToLower(w.Header().Get("Vary")), "accept-encoding") {
		w.Header().Add("Vary", "Accept-Encoding")
	}

	gz := gzipWriterPool.Get().(*gzip.Writer)
	gz.Reset(w.ResponseWriter)
	w.gz = gz
	w.ResponseWriter.WriteHeader(w.status)
}

func (w *gzipResponseWriter) finish() {
	if !w.decided {
		w.decide(w.buf.Len() >= gzipMinSize)
		if w.compress && w.gz != nil {
			if w.buf.Len() > 0 {
				_, _ = w.gz.Write(w.buf.Bytes())
				w.buf.Reset()
			}
		} else if w.buf.Len() > 0 {
			_, _ = w.ResponseWriter.Write(w.buf.Bytes())
			w.buf.Reset()
		}
	}
	if w.gz != nil {
		_ = w.gz.Close()
		w.gz.Reset(io.Discard)
		gzipWriterPool.Put(w.gz)
		w.gz = nil
	}
}

func isCompressibleType(ct string) bool {
	if ct == "" {
		return true
	}
	for _, prefix := range []string{"image/", "video/", "audio/", "application/octet-stream", "application/zip", "application/gzip"} {
		if strings.HasPrefix(ct, prefix) {
			return false
		}
	}
	return true
}

// Gzip compresses eligible HTTP responses larger than 1KB when the client advertises gzip support.
func Gzip() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodHead ||
			!strings.Contains(strings.ToLower(c.GetHeader("Accept-Encoding")), "gzip") ||
			strings.EqualFold(c.GetHeader("Upgrade"), "websocket") ||
			strings.HasPrefix(c.Request.URL.Path, "/ws/") ||
			strings.HasPrefix(c.Request.URL.Path, "/_admin/") {
			c.Next()
			return
		}

		origWriter := c.Writer
		gw := &gzipResponseWriter{ResponseWriter: origWriter}
		c.Writer = gw
		defer func() {
			gw.finish()
			c.Writer = origWriter
		}()

		c.Next()
	}
}
