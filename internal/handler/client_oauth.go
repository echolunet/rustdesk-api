package handler

import (
	"encoding/json"
	"html"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"rustdesk-api/internal/service"
)

// ClientOAuthHandler handles OIDC/OAuth client SSO initiation, browser callback, and polling.
type ClientOAuthHandler struct {
	authSvc  *service.AuthService
	oauthSvc *service.OAuthService
}

// NewClientOAuthHandler creates a new ClientOAuthHandler.
func NewClientOAuthHandler(authSvc *service.AuthService, oauthSvc *service.OAuthService) *ClientOAuthHandler {
	return &ClientOAuthHandler{authSvc: authSvc, oauthSvc: oauthSvc}
}

type oidcAuthRequest struct {
	Op         string          `json:"op"`
	ID         string          `json:"id"`
	UUID       string          `json:"uuid"`
	DeviceInfo json.RawMessage `json:"deviceInfo"`
}

// OIDCAuth starts an SSO login handshake and returns a tracking code plus authorization URL.
func (h *ClientOAuthHandler) OIDCAuth(c *gin.Context) {
	var req oidcAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	req.Op = strings.TrimSpace(req.Op)
	if req.Op == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing provider name (op)"})
		return
	}

	code, authURL, err := h.oauthSvc.StartAuth(
		req.Op,
		strings.TrimSpace(req.ID),
		strings.TrimSpace(req.UUID),
		string(req.DeviceInfo),
		RequestBaseURL(c),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": code,
		"url":  authURL,
	})
}

// AuthQuery is polled by the RustDesk desktop/mobile client until the browser OAuth flow completes.
func (h *ClientOAuthHandler) AuthQuery(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing code"})
		return
	}

	user, token, pending, err := h.oauthSvc.QueryAuth(code)
	if pending {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type":         "access_token",
		"access_token": token,
		"user":         h.authSvc.BuildClientUserPayload(user),
	})
}

// Callback receives the external IdP authorization redirect and completes the OAuth session.
func (h *ClientOAuthHandler) Callback(c *gin.Context) {
	if errParam := strings.TrimSpace(c.Query("error")); errParam != "" {
		desc := strings.TrimSpace(c.Query("error_description"))
		if desc != "" {
			errParam = errParam + ": " + desc
		}
		h.renderCallbackPage(c, http.StatusBadRequest, false, "登录失败 / Sign-in Failed", errParam)
		return
	}

	idpCode := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))

	user, err := h.oauthSvc.HandleCallback(idpCode, state, RequestBaseURL(c), c.ClientIP())
	if err != nil {
		h.renderCallbackPage(c, http.StatusBadRequest, false, "认证失败 / Authentication Failed", err.Error())
		return
	}

	msg := "已作为 " + user.Username + " 完成登录，您现在可以关闭此浏览器标签页并返回 RustDesk 客户端。"
	h.renderCallbackPage(c, http.StatusOK, true, "登录成功 / Sign-in Successful", msg)
}

func (h *ClientOAuthHandler) renderCallbackPage(c *gin.Context, statusCode int, ok bool, title, detail string) {
	color := "#ef4444"
	badge := "ERROR"
	if ok {
		color = "#10b981"
		badge = "SUCCESS"
	}
	body := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + html.EscapeString(title) + `</title>
<style>
  body { margin:0; font-family: system-ui, -apple-system, sans-serif; background:#0f172a; color:#e2e8f0; display:flex; align-items:center; justify-content:center; min-height:100vh; }
  .card { background:#1e293b; border:1px solid #334155; border-radius:14px; padding:32px; max-width:460px; width:90%; text-align:center; box-shadow:0 20px 50px rgba(0,0,0,0.4); }
  .badge { display:inline-block; padding:4px 12px; border-radius:999px; font-size:12px; font-weight:700; letter-spacing:1px; background:` + color + `22; color:` + color + `; border:1px solid ` + color + `; margin-bottom:16px; }
  h1 { margin:0 0 12px; font-size:20px; color:#f8fafc; }
  p { margin:0; color:#94a3b8; line-height:1.6; word-break:break-word; }
</style>
</head>
<body>
  <div class="card">
    <div class="badge">` + badge + `</div>
    <h1>` + html.EscapeString(title) + `</h1>
    <p>` + html.EscapeString(detail) + `</p>
  </div>
</body>
</html>`
	c.Data(statusCode, "text/html; charset=utf-8", []byte(body))
}
