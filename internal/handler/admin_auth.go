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

// AdminAuthHandler handles Web Admin Console authentication, self-service 2FA, and overview dashboard queries.
type AdminAuthHandler struct {
	db       *gorm.DB
	authSvc  *service.AuthService
	keySvc   *service.KeyService
	totpSvc  *service.TOTPService
	emailSvc *service.EmailService
}

// NewAdminAuthHandler creates a new AdminAuthHandler.
func NewAdminAuthHandler(
	db *gorm.DB,
	authSvc *service.AuthService,
	keySvc *service.KeyService,
	totpSvc *service.TOTPService,
	emailSvc *service.EmailService,
) *AdminAuthHandler {
	return &AdminAuthHandler{
		db:       db,
		authSvc:  authSvc,
		keySvc:   keySvc,
		totpSvc:  totpSvc,
		emailSvc: emailSvc,
	}
}

// Login authenticates an administrator for the Web Admin Console, including TOTP 2FA or email OTP challenges.
func (h *AdminAuthHandler) Login(c *gin.Context) {
	var req struct {
		Username         string `json:"username"`
		Password         string `json:"password"`
		TFACode          string `json:"tfa_code"`
		VerificationCode string `json:"verification_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	user, err := h.authSvc.AuthenticateLocal(req.Username, req.Password)
	if errors.Is(err, service.ErrInvalidCredentials) {
		user, err = h.authSvc.AuthenticateLDAP(req.Username, req.Password)
	}
	if err != nil {
		h.db.Create(&model.LoginLog{
			Username:   req.Username,
			AuthSource: "admin_web",
			IP:         c.ClientIP(),
			Status:     "failed",
			Message:    err.Error(),
		})
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	if !user.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "administrator privileges required"})
		return
	}

	if user.TOTPEnabled {
		code := strings.TrimSpace(req.TFACode)
		if code == "" {
			c.JSON(http.StatusOK, gin.H{
				"type":    "tfa_check",
				"message": "请输入 6 位动态验证器验证码或备用恢复码",
			})
			return
		}
		if !h.totpSvc.VerifyUserTFA(user, code) {
			h.db.Create(&model.LoginLog{
				UserID:     user.ID,
				Username:   user.Username,
				AuthSource: "admin_2fa",
				IP:         c.ClientIP(),
				Status:     "failed",
				Message:    "invalid 2FA code",
			})
			c.JSON(http.StatusUnauthorized, gin.H{"error": "2FA 验证码或恢复码无效"})
			return
		}
	} else {
		adminDeviceUUID := "admin-web:" + c.ClientIP()
		if h.emailSvc.NeedsEmailVerification(user, adminDeviceUUID) {
			code := strings.TrimSpace(req.VerificationCode)
			if code == "" {
				if sendErr := h.emailSvc.SendVerificationCode(user, "admin-web", adminDeviceUUID, c.ClientIP()); sendErr != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "发送登录验证邮件失败: " + sendErr.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"type":         "email_check",
					"masked_email": h.emailSvc.MaskEmail(user.Email),
					"message":      "登录验证码已发送至管理员邮箱",
				})
				return
			}
			if verErr := h.emailSvc.VerifyEmailCode(user.ID, code); verErr != nil {
				h.db.Create(&model.LoginLog{
					UserID:     user.ID,
					Username:   user.Username,
					AuthSource: "admin_email",
					IP:         c.ClientIP(),
					Status:     "failed",
					Message:    verErr.Error(),
				})
				c.JSON(http.StatusUnauthorized, gin.H{"error": verErr.Error()})
				return
			}
			h.emailSvc.MarkDeviceTrusted(user.ID, "admin-web", adminDeviceUUID, c.ClientIP())
		}
	}

	token, err := h.authSvc.IssueToken(user, "admin-web", "", c.ClientIP())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	h.db.Create(&model.LoginLog{
		UserID:     user.ID,
		Username:   user.Username,
		AuthSource: "admin_web",
		IP:         c.ClientIP(),
		Status:     "success",
		Message:    "admin console login",
	})

	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"user":         user,
	})
}

// Me returns the currently signed-in admin user profile.
func (h *AdminAuthHandler) Me(c *gin.Context) {
	user := middleware.CurrentUser(c)
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// ChangePassword updates the current admin user's password.
func (h *AdminAuthHandler) ChangePassword(c *gin.Context) {
	user := middleware.CurrentUser(c)
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if len(req.NewPassword) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new password must be at least 6 characters"})
		return
	}
	if user.PasswordHash != "" && !h.authSvc.VerifyPassword(user.PasswordHash, req.OldPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current password is incorrect"})
		return
	}

	hash, err := h.authSvc.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}
	if err := h.db.Model(user).Update("password_hash", hash).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "password updated"})
}

// GetMy2FA returns the current administrator's 2FA and email verification status.
func (h *AdminAuthHandler) GetMy2FA(c *gin.Context) {
	user := middleware.CurrentUser(c)
	c.JSON(http.StatusOK, gin.H{
		"totp_enabled":             user.TOTPEnabled,
		"email":                    user.Email,
		"email_verify_enabled":     user.EmailVerifyEnabled,
		"recovery_codes_remaining": len(user.TOTPRecoveryCodes),
	})
}

// SetupMy2FA initiates TOTP setup for the current administrator and returns secret, URI, and QR PNG data URI.
func (h *AdminAuthHandler) SetupMy2FA(c *gin.Context) {
	user := middleware.CurrentUser(c)
	secret, uri, qrDataURI, err := h.totpSvc.BeginSetup(user, "RustDesk-API")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize 2FA setup"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"secret":      secret,
		"otpauth_uri": uri,
		"qr_data_uri": qrDataURI,
	})
}

// ConfirmMy2FA verifies the first TOTP code, enables 2FA, and returns 6 single-use recovery codes.
func (h *AdminAuthHandler) ConfirmMy2FA(c *gin.Context) {
	user := middleware.CurrentUser(c)
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	codes, err := h.totpSvc.ConfirmSetup(user, req.Code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"totp_enabled":   true,
		"recovery_codes": codes,
	})
}

// DisableMy2FA disables TOTP 2FA for the current administrator after password + 2FA verification.
func (h *AdminAuthHandler) DisableMy2FA(c *gin.Context) {
	user := middleware.CurrentUser(c)
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if user.PasswordHash != "" && !h.authSvc.VerifyPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "当前密码不正确"})
		return
	}
	if user.TOTPEnabled && !h.totpSvc.VerifyUserTFA(user, req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "2FA 验证码或恢复码不正确"})
		return
	}
	if err := h.totpSvc.DisableTOTP(user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to disable 2FA"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"totp_enabled": false})
}

// Dashboard returns aggregate counts, recent logs, and client config summary for the admin overview page.
func (h *AdminAuthHandler) Dashboard(c *gin.Context) {
	var userCount, peerCount, onlinePeerCount, abCount, connCount, loginCount int64
	cutoff := time.Now().Add(-3 * time.Minute)
	dayAgo := time.Now().Add(-24 * time.Hour)

	h.db.Model(&model.User{}).Count(&userCount)
	h.db.Model(&model.Peer{}).Count(&peerCount)
	h.db.Model(&model.Peer{}).Where("last_online_at >= ?", cutoff).Count(&onlinePeerCount)
	h.db.Model(&model.AddressBookCollection{}).Count(&abCount)
	h.db.Model(&model.AuditConn{}).Where("created_at >= ?", dayAgo).Count(&connCount)
	h.db.Model(&model.LoginLog{}).Where("created_at >= ?", dayAgo).Count(&loginCount)

	var recentConns []model.AuditConn
	h.db.Order("id desc").Limit(8).Find(&recentConns)

	var recentLogins []model.LoginLog
	h.db.Order("id desc").Limit(8).Find(&recentLogins)

	cfgInfo := h.keySvc.GetClientConfig(RequestBaseURL(c))

	c.JSON(http.StatusOK, gin.H{
		"stats": gin.H{
			"users":            userCount,
			"peers":            peerCount,
			"online_peers":     onlinePeerCount,
			"address_books":    abCount,
			"conns_24h":        connCount,
			"logins_24h":       loginCount,
		},
		"server_config": cfgInfo,
		"recent_conns":  recentConns,
		"recent_logins": recentLogins,
	})
}
