package router

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rustdesk-api/internal/config"
	"rustdesk-api/internal/handler"
	"rustdesk-api/internal/middleware"
	"rustdesk-api/internal/service"
	"rustdesk-api/web"
)

// New creates and configures the full Gin HTTP engine for RustDesk Client API, Admin Console, and WebSocket proxies.
func New(db *gorm.DB, cfg *config.Config) *gin.Engine {
	if cfg.AppMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), middleware.CORS(), middleware.Gzip())

	startTime := time.Now().Unix()

	authSvc := service.NewAuthService(db, cfg)
	oauthSvc := service.NewOAuthService(db, authSvc)
	abSvc := service.NewABService(db)
	keySvc := service.NewKeyService(cfg)
	updateSvc := service.NewUpdateService()
	totpSvc := service.NewTOTPService(db)
	emailSvc := service.NewEmailService(db)
	strategySvc := service.NewStrategyService(db)

	clientAuthH := handler.NewClientAuthHandler(db, authSvc, oauthSvc, totpSvc, emailSvc)
	clientOAuthH := handler.NewClientOAuthHandler(authSvc, oauthSvc)
	clientPeerH := handler.NewClientPeerHandler(db, cfg, strategySvc, startTime)
	clientABH := handler.NewClientABHandler(db, cfg, abSvc)
	clientAuditH := handler.NewClientAuditHandler(db)
	proxyWCH := handler.NewProxyWebClientHandler(cfg, keySvc)

	adminAuthH := handler.NewAdminAuthHandler(db, authSvc, keySvc, totpSvc, emailSvc)
	adminUserH := handler.NewAdminUserHandler(db, authSvc, abSvc, totpSvc)
	adminDeviceH := handler.NewAdminDeviceHandler(db)
	adminABH := handler.NewAdminABHandler(db)
	adminAuditH := handler.NewAdminAuditHandler(db)
	adminSysH := handler.NewAdminSystemHandler(db, authSvc, keySvc, updateSvc, emailSvc, strategySvc)

	authMW := middleware.AuthRequired(authSvc)
	adminMW := middleware.AdminRequired()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": cfg.AppVersion,
		})
	})

	// WebSocket proxy routes forwarding to official hbbs (21118) and hbbr (21119)
	r.GET("/ws/id", proxyWCH.ProxyWS(cfg.HBBSWSTarget))
	r.GET("/ws/relay", proxyWCH.ProxyWS(cfg.HBBRWSTarget))

	// RustDesk Web Client static hosting
	r.GET("/webclient/*filepath", proxyWCH.ServeWebClient)

	// Embedded Web Admin Console
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/_admin/")
	})
	if subFS, err := fs.Sub(web.AdminFS, "admin"); err == nil {
		r.GET("/_admin/*filepath", serveEmbeddedAdmin(subFS))
	}

	// RustDesk Official Client API
	api := r.Group("/api")
	{
		api.POST("/login", clientAuthH.Login)
		api.GET("/login-options", clientAuthH.LoginOptions)
		api.POST("/oidc/auth", clientOAuthH.OIDCAuth)
		api.GET("/oidc/auth-query", clientOAuthH.AuthQuery)
		api.GET("/oidc/callback", clientOAuthH.Callback)
		api.GET("/oauth/callback", clientOAuthH.Callback)

		api.POST("/heartbeat", clientPeerH.Heartbeat)
		api.POST("/sysinfo", clientPeerH.Sysinfo)
		api.GET("/sysinfover", clientPeerH.SysinfoVer)
		api.POST("/sysinfover", clientPeerH.SysinfoVer)

		api.POST("/audit/conn", clientAuditH.LogConn)
		api.POST("/audit/file", clientAuditH.LogFile)
		api.POST("/audit/alarm", clientAuditH.LogAlarm)
		api.POST("/audit", clientAuditH.LogConn)

		authed := api.Group("")
		authed.Use(authMW)
		{
			authed.POST("/logout", clientAuthH.Logout)
			authed.GET("/currentUser", clientAuthH.CurrentUser)
			authed.POST("/currentUser", clientAuthH.CurrentUser)

			authed.GET("/peers", clientPeerH.ListPeers)
			authed.GET("/devices", clientPeerH.ListPeers)
			authed.GET("/users", clientPeerH.ListUsers)
			authed.GET("/device-group/accessible", clientPeerH.AccessibleDeviceGroups)

			// Legacy Address Book (v1)
			authed.GET("/ab", clientABH.LegacyGet)
			authed.POST("/ab/get", clientABH.LegacyGet)
			authed.POST("/ab", clientABH.LegacySave)

			// Personal & Shared Address Book (v2)
			authed.GET("/ab/personal", clientABH.PersonalProfile)
			authed.POST("/ab/personal", clientABH.PersonalProfile)
			authed.GET("/ab/shared/profiles", clientABH.SharedProfiles)
			authed.POST("/ab/shared/profiles", clientABH.SharedProfiles)
			authed.GET("/ab/peers", clientABH.ListPeers)
			authed.POST("/ab/peers", clientABH.ListPeers)
			authed.GET("/ab/tags/:guid", clientABH.ListTags)
			authed.POST("/ab/tags/:guid", clientABH.ListTags)

			authed.POST("/ab/peer/add/:guid", clientABH.AddPeers)
			authed.PUT("/ab/peer/update/:guid", clientABH.UpdatePeers)
			authed.POST("/ab/peer/update/:guid", clientABH.UpdatePeers)
			authed.DELETE("/ab/peer/:guid", clientABH.DeletePeers)

			authed.POST("/ab/tag/add/:guid", clientABH.AddTags)
			authed.PUT("/ab/tag/update/:guid", clientABH.UpdateTags)
			authed.POST("/ab/tag/update/:guid", clientABH.UpdateTags)
			authed.PUT("/ab/tag/rename/:guid", clientABH.RenameTag)
			authed.POST("/ab/tag/rename/:guid", clientABH.RenameTag)
			authed.DELETE("/ab/tag/:guid", clientABH.DeleteTags)

			authed.GET("/ab/settings", clientABH.Settings)
			authed.POST("/ab/settings", clientABH.Settings)
		}
	}

	// Web Admin Console API
	adminAPI := r.Group("/admin/api")
	{
		adminAPI.POST("/login", adminAuthH.Login)

		protected := adminAPI.Group("")
		protected.Use(authMW, adminMW)
		{
			protected.GET("/me", adminAuthH.Me)
			protected.PUT("/me/password", adminAuthH.ChangePassword)
			protected.GET("/me/2fa", adminAuthH.GetMy2FA)
			protected.POST("/me/2fa/setup", adminAuthH.SetupMy2FA)
			protected.POST("/me/2fa/confirm", adminAuthH.ConfirmMy2FA)
			protected.POST("/me/2fa/disable", adminAuthH.DisableMy2FA)
			protected.GET("/dashboard", adminAuthH.Dashboard)

			protected.GET("/users", adminUserH.ListUsers)
			protected.POST("/users", adminUserH.CreateUser)
			protected.PUT("/users/:id", adminUserH.UpdateUser)
			protected.DELETE("/users/:id", adminUserH.DeleteUser)
			protected.POST("/users/:id/revoke-tokens", adminUserH.RevokeTokens)
			protected.POST("/users/:id/reset-2fa", adminUserH.ResetUser2FA)

			protected.GET("/groups", adminUserH.ListGroups)
			protected.POST("/groups", adminUserH.CreateGroup)
			protected.PUT("/groups/:id", adminUserH.UpdateGroup)
			protected.DELETE("/groups/:id", adminUserH.DeleteGroup)

			protected.GET("/devices", adminDeviceH.ListDevices)
			protected.PUT("/devices/:id", adminDeviceH.UpdateDevice)
			protected.DELETE("/devices/:id", adminDeviceH.DeleteDevice)

			protected.GET("/device-groups", adminDeviceH.ListDeviceGroups)
			protected.POST("/device-groups", adminDeviceH.CreateDeviceGroup)
			protected.PUT("/device-groups/:id", adminDeviceH.UpdateDeviceGroup)
			protected.DELETE("/device-groups/:id", adminDeviceH.DeleteDeviceGroup)

			protected.GET("/ab/collections", adminABH.ListCollections)
			protected.POST("/ab/collections", adminABH.CreateCollection)
			protected.PUT("/ab/collections/:guid", adminABH.UpdateCollection)
			protected.DELETE("/ab/collections/:guid", adminABH.DeleteCollection)
			protected.GET("/ab/collections/:guid/rules", adminABH.ListRules)
			protected.POST("/ab/collections/:guid/rules", adminABH.CreateRule)
			protected.DELETE("/ab/collections/:guid/rules/:ruleId", adminABH.DeleteRule)
			protected.GET("/ab/collections/:guid/peers", adminABH.ListPeers)
			protected.POST("/ab/collections/:guid/peers", adminABH.UpsertPeer)
			protected.DELETE("/ab/collections/:guid/peers/:peerId", adminABH.DeletePeer)
			protected.GET("/ab/collections/:guid/tags", adminABH.ListTags)
			protected.POST("/ab/collections/:guid/tags", adminABH.UpsertTag)
			protected.DELETE("/ab/collections/:guid/tags/:name", adminABH.DeleteTag)

			protected.GET("/audit/conn", adminAuditH.ListConnLogs)
			protected.GET("/audit/file", adminAuditH.ListFileLogs)
			protected.GET("/audit/alarm", adminAuditH.ListAlarmLogs)
			protected.GET("/audit/login", adminAuditH.ListLoginLogs)
			protected.DELETE("/audit/purge", adminAuditH.PurgeLogs)

			protected.GET("/oauth/providers", adminSysH.ListOAuthProviders)
			protected.POST("/oauth/providers", adminSysH.CreateOAuthProvider)
			protected.PUT("/oauth/providers/:id", adminSysH.UpdateOAuthProvider)
			protected.DELETE("/oauth/providers/:id", adminSysH.DeleteOAuthProvider)

			protected.GET("/ldap", adminSysH.GetLDAP)
			protected.PUT("/ldap", adminSysH.UpdateLDAP)
			protected.POST("/ldap/test", adminSysH.TestLDAP)

			protected.GET("/smtp", adminSysH.GetSMTP)
			protected.PUT("/smtp", adminSysH.UpdateSMTP)
			protected.POST("/smtp/test", adminSysH.TestSMTP)
			protected.GET("/client-strategy", adminSysH.GetClientStrategy)
			protected.PUT("/client-strategy", adminSysH.UpdateClientStrategy)

			protected.GET("/system/server-config", adminSysH.ServerConfig)
			protected.GET("/system/updates", adminSysH.OfficialUpdates)
		}
	}

	return r
}

type embeddedAsset struct {
	raw         []byte
	gz          []byte
	etag        string
	contentType string
	cacheCtrl   string
}

func serveEmbeddedAdmin(subFS fs.FS) gin.HandlerFunc {
	assets := make(map[string]embeddedAsset, 8)
	_ = fs.WalkDir(subFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		raw, readErr := fs.ReadFile(subFS, path)
		if readErr != nil {
			return nil
		}
		ctype := mime.TypeByExtension(filepath.Ext(path))
		if ctype == "" {
			ctype = http.DetectContentType(raw)
		}
		sum := sha256.Sum256(raw)
		etag := `"` + hex.EncodeToString(sum[:8]) + `"`
		cacheCtrl := "public, max-age=3600"
		if path == "index.html" {
			cacheCtrl = "no-cache"
		}

		var gzBytes []byte
		if len(raw) >= 256 {
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".html" || ext == ".js" || ext == ".css" || ext == ".svg" || ext == ".json" {
				var buf bytes.Buffer
				if gw, gwErr := gzip.NewWriterLevel(&buf, gzip.BestCompression); gwErr == nil {
					_, _ = gw.Write(raw)
					if gw.Close() == nil && buf.Len() < len(raw) {
						gzBytes = buf.Bytes()
					}
				}
			}
		}

		assets[path] = embeddedAsset{
			raw:         raw,
			gz:          gzBytes,
			etag:        etag,
			contentType: ctype,
			cacheCtrl:   cacheCtrl,
		}
		return nil
	})

	return func(c *gin.Context) {
		p := strings.TrimSpace(strings.TrimPrefix(c.Param("filepath"), "/"))
		if p == "" {
			p = "index.html"
		}
		if strings.Contains(p, "..") {
			c.Status(http.StatusForbidden)
			return
		}

		asset, ok := assets[p]
		if !ok {
			asset, ok = assets["index.html"]
			if !ok {
				c.Status(http.StatusNotFound)
				return
			}
		}

		c.Header("ETag", asset.etag)
		c.Header("Cache-Control", asset.cacheCtrl)
		if len(asset.gz) > 0 {
			c.Header("Vary", "Accept-Encoding")
		}
		if match := c.GetHeader("If-None-Match"); match != "" && strings.Contains(match, asset.etag) {
			c.Status(http.StatusNotModified)
			return
		}
		if len(asset.gz) > 0 && strings.Contains(strings.ToLower(c.GetHeader("Accept-Encoding")), "gzip") {
			c.Header("Content-Encoding", "gzip")
			c.Data(http.StatusOK, asset.contentType, asset.gz)
			return
		}
		c.Data(http.StatusOK, asset.contentType, asset.raw)
	}
}
