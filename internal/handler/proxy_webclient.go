package handler

import (
	"html"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"rustdesk-api/internal/config"
	"rustdesk-api/internal/service"
)

// ProxyWebClientHandler serves WebSocket reverse proxying to hbbs/hbbr and hosts static RustDesk Web Client files.
type ProxyWebClientHandler struct {
	cfg    *config.Config
	keySvc *service.KeyService
}

// NewProxyWebClientHandler creates a new ProxyWebClientHandler.
func NewProxyWebClientHandler(cfg *config.Config, keySvc *service.KeyService) *ProxyWebClientHandler {
	return &ProxyWebClientHandler{cfg: cfg, keySvc: keySvc}
}

// ProxyWS creates a reverse proxy handler forwarding WebSocket/HTTP upgrades to official hbbs or hbbr.
func (h *ProxyWebClientHandler) ProxyWS(targetRaw string) gin.HandlerFunc {
	target, err := url.Parse(targetRaw)
	if err != nil {
		return func(c *gin.Context) {
			c.JSON(http.StatusBadGateway, gin.H{"error": "invalid websocket upstream target"})
		}
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = target.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, proxyErr error) {
		http.Error(w, "upstream websocket unavailable: "+proxyErr.Error(), http.StatusBadGateway)
	}
	return func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}

// ServeWebClient serves static official RustDesk Flutter Web Client assets or a setup guide if not yet populated.
func (h *ProxyWebClientHandler) ServeWebClient(c *gin.Context) {
	indexPath := filepath.Join(h.cfg.WebClientDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		h.renderGuidePage(c)
		return
	}

	reqPath := strings.TrimPrefix(c.Param("filepath"), "/")
	if reqPath == "" {
		c.File(indexPath)
		return
	}

	cleaned := filepath.Clean(reqPath)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		c.Status(http.StatusForbidden)
		return
	}

	baseAbs, err := filepath.Abs(h.cfg.WebClientDir)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	targetAbs, err := filepath.Abs(filepath.Join(baseAbs, cleaned))
	if err != nil {
		c.Status(http.StatusForbidden)
		return
	}
	if targetAbs != baseAbs && !strings.HasPrefix(targetAbs, baseAbs+string(filepath.Separator)) {
		c.Status(http.StatusForbidden)
		return
	}

	if info, err := os.Stat(targetAbs); err == nil && !info.IsDir() {
		c.File(targetAbs)
		return
	}
	c.File(indexPath)
}

func (h *ProxyWebClientHandler) renderGuidePage(c *gin.Context) {
	info := h.keySvc.GetClientConfig(RequestBaseURL(c))
	body := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>RustDesk Web 客户端托管引导</title>
<style>
  body { margin:0; font-family: system-ui, -apple-system, sans-serif; background:#0f172a; color:#e2e8f0; display:flex; align-items:center; justify-content:center; min-height:100vh; padding:24px; }
  .card { background:#1e293b; border:1px solid #334155; border-radius:16px; padding:28px; max-width:720px; width:100%; box-shadow:0 24px 60px rgba(0,0,0,0.45); }
  h1 { margin:0 0 8px; font-size:22px; color:#f8fafc; }
  p { color:#94a3b8; line-height:1.6; }
  .kv { background:#0f172a; border:1px solid #334155; border-radius:10px; padding:14px; margin:16px 0; font-family: ui-monospace, monospace; font-size:13px; overflow-x:auto; }
  .row { margin:6px 0; }
  .lbl { color:#38bdf8; }
  code { background:#0f172a; padding:2px 6px; border-radius:6px; color:#38bdf8; }
  .actions { display:flex; gap:12px; flex-wrap:wrap; margin-top:20px; }
  .btn { display:inline-block; padding:10px 16px; border-radius:8px; text-decoration:none; font-weight:600; font-size:14px; }
  .btn-primary { background:#2563eb; color:#fff; }
  .btn-secondary { background:#334155; color:#f8fafc; }
</style>
</head>
<body>
  <div class="card">
    <h1>RustDesk Web 客户端尚未初始化</h1>
    <p>当前 <code>` + html.EscapeString(h.cfg.WebClientDir) + `</code> 目录中未检测到静态 Web 客户端资源。WebSocket 代理 <code>/ws/id</code>（hbbs:21118）与 <code>/ws/relay</code>（hbbr:21119）已就绪。</p>
    <div class="kv">
      <div class="row"><span class="lbl">ID Server   :</span> ` + html.EscapeString(info.IDServer) + `</div>
      <div class="row"><span class="lbl">Relay Server:</span> ` + html.EscapeString(info.RelayServer) + `</div>
      <div class="row"><span class="lbl">API Server  :</span> ` + html.EscapeString(info.APIServer) + `</div>
      <div class="row"><span class="lbl">Public Key  :</span> ` + html.EscapeString(info.PublicKey) + `</div>
    </div>
    <p>在宿主机运行以下命令即可从官方 GitHub 构建并同步最新的 Flutter Web 客户端到 <code>./data/webclient</code>：</p>
    <div class="kv">./scripts/sync-webclient.sh --build-official</div>
    <div class="actions">
      <a class="btn btn-primary" href="/_admin/">前往管理后台</a>
      <a class="btn btn-secondary" href="https://rustdesk.com/web/" target="_blank" rel="noopener">访问官方 Web Client</a>
    </div>
  </div>
</body>
</html>`
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(body))
}
