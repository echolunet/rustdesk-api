package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rustdesk-api/internal/middleware"
	"rustdesk-api/internal/model"
	"rustdesk-api/internal/service"
)

// ClientAuthHandler serves RustDesk desktop/mobile/web client authentication routes.
type ClientAuthHandler struct {
	db       *gorm.DB
	authSvc  *service.AuthService
	oauthSvc *service.OAuthService
	totpSvc  *service.TOTPService
	emailSvc *service.EmailService
}

// NewClientAuthHandler creates a new ClientAuthHandler.
func NewClientAuthHandler(
	db *gorm.DB,
	authSvc *service.AuthService,
	oauthSvc *service.OAuthService,
	totpSvc *service.TOTPService,
	emailSvc *service.EmailService,
) *ClientAuthHandler {
	return &ClientAuthHandler{
		db:       db,
		authSvc:  authSvc,
		oauthSvc: oauthSvc,
		totpSvc:  totpSvc,
		emailSvc: emailSvc,
	}
}

type clientLoginRequest struct {
	Username         string                 `json:"username"`
	Password         string                 `json:"password"`
	ID               string                 `json:"id"`
	UUID             string                 `json:"uuid"`
	AutoLogin        bool                   `json:"autoLogin"`
	Type             string                 `json:"type"`
	VerificationCode string                 `json:"verificationCode"`
	TFACode          string                 `json:"tfaCode"`
	Secret           string                 `json:"secret"`
	DeviceInfo       map[string]interface{} `json:"deviceInfo"`
}

// Login authenticates a RustDesk client via local credentials or LDAP and returns a JWT access token.
func (h *ClientAuthHandler) Login(c *gin.Context) {
	var req clientLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid login payload"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	platform := ""
	if req.DeviceInfo != nil {
		if osName, ok := req.DeviceInfo["os"].(string); ok {
			platform = osName
		} else if typ, ok := req.DeviceInfo["type"].(string); ok {
			platform = typ
		}
	}

	user, err := h.authSvc.AuthenticateLocal(req.Username, req.Password)
	if errors.Is(err, service.ErrInvalidCredentials) {
		user, err = h.authSvc.AuthenticateLDAP(req.Username, req.Password)
	}
	if err != nil {
		h.db.Create(&model.LoginLog{
			Username:   req.Username,
			AuthSource: "client",
			DeviceID:   req.ID,
			DeviceUUID: req.UUID,
			Platform:   platform,
			IP:         c.ClientIP(),
			Status:     "failed",
			Message:    err.Error(),
		})
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if user.TOTPEnabled {
		code := strings.TrimSpace(req.TFACode)
		if code == "" {
			code = strings.TrimSpace(req.VerificationCode)
		}
		if code == "" {
			challengeSecret, chErr := h.totpSvc.CreateChallenge(user.ID, req.UUID, c.ClientIP())
			if chErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create 2FA challenge"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"type":    "tfa_check",
				"secret":  challengeSecret,
				"message": "Two-factor authentication code required",
			})
			return
		}
		if !h.totpSvc.VerifyUserTFA(user, code) {
			h.db.Create(&model.LoginLog{
				UserID:     user.ID,
				Username:   user.Username,
				AuthSource: "client_2fa",
				DeviceID:   req.ID,
				DeviceUUID: req.UUID,
				Platform:   platform,
				IP:         c.ClientIP(),
				Status:     "failed",
				Message:    "invalid 2FA code",
			})
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 2FA verification code"})
			return
		}
	} else if h.emailSvc.NeedsEmailVerification(user, req.UUID) {
		code := strings.TrimSpace(req.VerificationCode)
		if code == "" {
			if sendErr := h.emailSvc.SendVerificationCode(user, req.ID, req.UUID, c.ClientIP()); sendErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": sendErr.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"type":    "email_check",
				"message": "Verification code sent to your email",
			})
			return
		}
		if verErr := h.emailSvc.VerifyEmailCode(user.ID, code); verErr != nil {
			h.db.Create(&model.LoginLog{
				UserID:     user.ID,
				Username:   user.Username,
				AuthSource: "client_email",
				DeviceID:   req.ID,
				DeviceUUID: req.UUID,
				Platform:   platform,
				IP:         c.ClientIP(),
				Status:     "failed",
				Message:    verErr.Error(),
			})
			c.JSON(http.StatusBadRequest, gin.H{"error": verErr.Error()})
			return
		}
		h.emailSvc.MarkDeviceTrusted(user.ID, req.ID, req.UUID, c.ClientIP())
	}

	token, err := h.authSvc.IssueToken(user, req.ID, req.UUID, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	if strings.TrimSpace(req.ID) != "" {
		h.bindPeerToUser(strings.TrimSpace(req.ID), strings.TrimSpace(req.UUID), user, c.ClientIP())
	}

	h.db.Create(&model.LoginLog{
		UserID:     user.ID,
		Username:   user.Username,
		AuthSource: user.AuthSource,
		DeviceID:   req.ID,
		DeviceUUID: req.UUID,
		Platform:   platform,
		IP:         c.ClientIP(),
		Status:     "success",
		Message:    "client login",
	})

	c.JSON(http.StatusOK, gin.H{
		"type":         "access_token",
		"access_token": token,
		"user":         h.authSvc.BuildClientUserPayload(user),
	})
}

func (h *ClientAuthHandler) bindPeerToUser(peerID, uuidVal string, user *model.User, ip string) {
	now := time.Now()
	var peer model.Peer
	err := h.db.Where("peer_id = ?", peerID).First(&peer).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h.db.Create(&model.Peer{
			PeerID:       peerID,
			UUID:         uuidVal,
			UserID:       user.ID,
			UserName:     user.Username,
			LastIP:       ip,
			LastOnlineAt: &now,
		})
		return
	}
	if err != nil {
		return
	}
	if peer.UUID == "" || uuidVal == "" || peer.UUID == uuidVal {
		updates := map[string]interface{}{
			"user_id":   user.ID,
			"user_name": user.Username,
			"last_ip":   ip,
		}
		if peer.UUID == "" && uuidVal != "" {
			updates["uuid"] = uuidVal
		}
		h.db.Model(&peer).Updates(updates)
	}
}

// Logout revokes the current client JWT token.
func (h *ClientAuthHandler) Logout(c *gin.Context) {
	jti := middleware.CurrentJTI(c)
	_ = h.authSvc.RevokeToken(jti)
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": "ok"})
}

// CurrentUser returns the currently authenticated user profile in RustDesk client format.
func (h *ClientAuthHandler) CurrentUser(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, h.authSvc.BuildClientUserPayload(user))
}

// LoginOptions returns enabled OIDC/OAuth login option identifiers for the RustDesk client UI.
func (h *ClientAuthHandler) LoginOptions(c *gin.Context) {
	options, err := h.oauthSvc.ListEnabledOptions()
	if err != nil {
		c.JSON(http.StatusOK, []string{})
		return
	}
	c.JSON(http.StatusOK, options)
}
