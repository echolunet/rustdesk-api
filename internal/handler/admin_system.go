package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rustdesk-api/internal/model"
	"rustdesk-api/internal/service"
)

const secretMask = "********"

// AdminSystemHandler manages OAuth providers, LDAP settings, SMTP settings, server keys/config, and official RustDesk update checks.
type AdminSystemHandler struct {
	db          *gorm.DB
	authSvc     *service.AuthService
	keySvc      *service.KeyService
	updateSvc   *service.UpdateService
	emailSvc    *service.EmailService
	strategySvc *service.StrategyService
}

// NewAdminSystemHandler creates a new AdminSystemHandler.
func NewAdminSystemHandler(
	db *gorm.DB,
	authSvc *service.AuthService,
	keySvc *service.KeyService,
	updateSvc *service.UpdateService,
	emailSvc *service.EmailService,
	strategySvc *service.StrategyService,
) *AdminSystemHandler {
	return &AdminSystemHandler{
		db:          db,
		authSvc:     authSvc,
		keySvc:      keySvc,
		updateSvc:   updateSvc,
		emailSvc:    emailSvc,
		strategySvc: strategySvc,
	}
}

type oauthProviderDTO struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	DisplayName  string `json:"display_name"`
	ClientID     string `json:"client_id"`
	HasSecret    bool   `json:"has_secret"`
	Issuer       string `json:"issuer"`
	AuthURL      string `json:"auth_url"`
	TokenURL     string `json:"token_url"`
	UserInfoURL  string `json:"user_info_url"`
	Scopes       string `json:"scopes"`
	RedirectURL  string `json:"redirect_url"`
	Icon         string `json:"icon"`
	AutoRegister bool   `json:"auto_register"`
	Enabled      bool   `json:"enabled"`
}

// ListOAuthProviders lists configured SSO providers with secrets masked.
func (h *AdminSystemHandler) ListOAuthProviders(c *gin.Context) {
	var list []model.OAuthProvider
	h.db.Order("id asc").Find(&list)
	out := make([]oauthProviderDTO, 0, len(list))
	for _, p := range list {
		out = append(out, oauthProviderDTO{
			ID:           p.ID,
			Name:         p.Name,
			ProviderType: p.ProviderType,
			DisplayName:  p.DisplayName,
			ClientID:     p.ClientID,
			HasSecret:    p.ClientSecret != "",
			Issuer:       p.Issuer,
			AuthURL:      p.AuthURL,
			TokenURL:     p.TokenURL,
			UserInfoURL:  p.UserInfoURL,
			Scopes:       p.Scopes,
			RedirectURL:  p.RedirectURL,
			Icon:         p.Icon,
			AutoRegister: p.AutoRegister,
			Enabled:      p.Enabled,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

type oauthProviderReq struct {
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	DisplayName  string `json:"display_name"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Issuer       string `json:"issuer"`
	AuthURL      string `json:"auth_url"`
	TokenURL     string `json:"token_url"`
	UserInfoURL  string `json:"user_info_url"`
	Scopes       string `json:"scopes"`
	RedirectURL  string `json:"redirect_url"`
	Icon         string `json:"icon"`
	AutoRegister bool   `json:"auto_register"`
	Enabled      bool   `json:"enabled"`
}

// CreateOAuthProvider creates a new OIDC/GitHub/Google SSO provider.
func (h *AdminSystemHandler) CreateOAuthProvider(c *gin.Context) {
	var req oauthProviderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.ClientID = strings.TrimSpace(req.ClientID)
	if req.Name == "" || req.ClientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and client_id are required"})
		return
	}
	if req.ProviderType == "" {
		req.ProviderType = "oidc"
	}

	p := model.OAuthProvider{
		Name:         req.Name,
		ProviderType: strings.ToLower(strings.TrimSpace(req.ProviderType)),
		DisplayName:  req.DisplayName,
		ClientID:     req.ClientID,
		ClientSecret: strings.TrimSpace(req.ClientSecret),
		Issuer:       strings.TrimSpace(req.Issuer),
		AuthURL:      strings.TrimSpace(req.AuthURL),
		TokenURL:     strings.TrimSpace(req.TokenURL),
		UserInfoURL:  strings.TrimSpace(req.UserInfoURL),
		Scopes:       strings.TrimSpace(req.Scopes),
		RedirectURL:  strings.TrimSpace(req.RedirectURL),
		Icon:         req.Icon,
		AutoRegister: req.AutoRegister,
		Enabled:      req.Enabled,
	}
	if err := h.db.Create(&p).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "provider name already exists"})
		return
	}
	p.ClientSecret = ""
	c.JSON(http.StatusCreated, p)
}

// UpdateOAuthProvider updates an existing SSO provider while preserving ClientSecret if omitted.
func (h *AdminSystemHandler) UpdateOAuthProvider(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}
	var p model.OAuthProvider
	if err := h.db.First(&p, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	var req oauthProviderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	updates := map[string]interface{}{
		"provider_type": strings.ToLower(strings.TrimSpace(req.ProviderType)),
		"display_name":  req.DisplayName,
		"client_id":     strings.TrimSpace(req.ClientID),
		"issuer":        strings.TrimSpace(req.Issuer),
		"auth_url":      strings.TrimSpace(req.AuthURL),
		"token_url":     strings.TrimSpace(req.TokenURL),
		"user_info_url": strings.TrimSpace(req.UserInfoURL),
		"scopes":        strings.TrimSpace(req.Scopes),
		"redirect_url":  strings.TrimSpace(req.RedirectURL),
		"icon":          req.Icon,
		"auto_register": req.AutoRegister,
		"enabled":       req.Enabled,
	}
	if strings.TrimSpace(req.Name) != "" {
		updates["name"] = strings.TrimSpace(req.Name)
	}
	sec := strings.TrimSpace(req.ClientSecret)
	if sec != "" && sec != secretMask {
		updates["client_secret"] = sec
	}

	if err := h.db.Model(&p).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update provider failed"})
		return
	}
	_ = h.db.First(&p, p.ID).Error
	p.ClientSecret = ""
	c.JSON(http.StatusOK, p)
}

// DeleteOAuthProvider deletes an SSO provider.
func (h *AdminSystemHandler) DeleteOAuthProvider(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return
	}
	h.db.Delete(&model.OAuthProvider{}, id)
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// GetLDAP returns the singleton LDAP configuration with BindPassword masked.
func (h *AdminSystemHandler) GetLDAP(c *gin.Context) {
	var cfg model.LDAPConfig
	if err := h.db.First(&cfg).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                cfg.ID,
		"enabled":           cfg.Enabled,
		"url":               cfg.URL,
		"skip_tls_verify":   cfg.SkipTLSVerify,
		"base_dn":           cfg.BaseDN,
		"bind_dn":           cfg.BindDN,
		"has_bind_password": cfg.BindPassword != "",
		"user_filter":       cfg.UserFilter,
		"username_attr":     cfg.UsernameAttr,
		"email_attr":        cfg.EmailAttr,
		"auto_register":     cfg.AutoRegister,
	})
}

// UpdateLDAP saves the singleton LDAP configuration.
func (h *AdminSystemHandler) UpdateLDAP(c *gin.Context) {
	var req model.LDAPConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	var existing model.LDAPConfig
	if err := h.db.First(&existing).Error; err != nil {
		if err := h.db.Create(&req).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "save ldap config failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "saved"})
		return
	}

	updates := map[string]interface{}{
		"enabled":         req.Enabled,
		"url":             strings.TrimSpace(req.URL),
		"skip_tls_verify": req.SkipTLSVerify,
		"base_dn":         strings.TrimSpace(req.BaseDN),
		"bind_dn":         strings.TrimSpace(req.BindDN),
		"user_filter":     strings.TrimSpace(req.UserFilter),
		"username_attr":   strings.TrimSpace(req.UsernameAttr),
		"email_attr":      strings.TrimSpace(req.EmailAttr),
		"auto_register":   req.AutoRegister,
	}
	pw := strings.TrimSpace(req.BindPassword)
	if pw != "" && pw != secretMask {
		updates["bind_password"] = pw
	}

	if err := h.db.Model(&existing).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update ldap config failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "saved"})
}

// TestLDAP tests connectivity and bind credentials against the configured LDAP server.
func (h *AdminSystemHandler) TestLDAP(c *gin.Context) {
	var req model.LDAPConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(req.BindPassword) == "" || req.BindPassword == secretMask {
		var existing model.LDAPConfig
		if err := h.db.First(&existing).Error; err == nil {
			req.BindPassword = existing.BindPassword
		}
	}
	if err := h.authSvc.TestLDAPConnection(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "LDAP connection and bind succeeded"})
}

// ServerConfig returns the official hbbs public key and one-click RustDesk client configuration string.
func (h *AdminSystemHandler) ServerConfig(c *gin.Context) {
	info := h.keySvc.GetClientConfig(RequestBaseURL(c))
	c.JSON(http.StatusOK, info)
}

// OfficialUpdates returns cached or freshly fetched official RustDesk client and server GitHub release info.
func (h *AdminSystemHandler) OfficialUpdates(c *gin.Context) {
	force := c.Query("refresh") == "1" || strings.EqualFold(c.Query("refresh"), "true")
	payload := h.updateSvc.CheckOfficialUpdates(force)
	c.JSON(http.StatusOK, payload)
}

// GetSMTP returns the singleton SMTP and email verification policy configuration with Password masked.
func (h *AdminSystemHandler) GetSMTP(c *gin.Context) {
	var cfg model.SMTPConfig
	if err := h.db.First(&cfg).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled":             false,
			"port":                587,
			"security":            "starttls",
			"from_name":           "RustDesk API",
			"email_login_policy":  "off",
			"code_expire_minutes": 10,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                  cfg.ID,
		"enabled":             cfg.Enabled,
		"host":                cfg.Host,
		"port":                cfg.Port,
		"security":            cfg.Security,
		"username":            cfg.Username,
		"has_password":        cfg.Password != "",
		"from_address":        cfg.FromAddress,
		"from_name":           cfg.FromName,
		"skip_tls_verify":     cfg.SkipTLSVerify,
		"email_login_policy":  cfg.EmailLoginPolicy,
		"code_expire_minutes": cfg.CodeExpireMinutes,
	})
}

type smtpConfigReq struct {
	Enabled           bool   `json:"enabled"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	Security          string `json:"security"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	FromAddress       string `json:"from_address"`
	FromName          string `json:"from_name"`
	SkipTLSVerify     bool   `json:"skip_tls_verify"`
	EmailLoginPolicy  string `json:"email_login_policy"`
	CodeExpireMinutes int    `json:"code_expire_minutes"`
	To                string `json:"to"`
}

func normalizeSMTPFields(req *smtpConfigReq) (int, string, string, int) {
	port := req.Port
	if port <= 0 {
		port = 587
	}
	sec := strings.ToLower(strings.TrimSpace(req.Security))
	if sec != "ssl" && sec != "none" {
		sec = "starttls"
	}
	policy := strings.ToLower(strings.TrimSpace(req.EmailLoginPolicy))
	if policy != "new_device" && policy != "always" {
		policy = "off"
	}
	exp := req.CodeExpireMinutes
	if exp <= 0 {
		exp = 10
	}
	return port, sec, policy, exp
}

// UpdateSMTP updates the singleton SMTP configuration and email verification policy.
func (h *AdminSystemHandler) UpdateSMTP(c *gin.Context) {
	var req smtpConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	port, sec, policy, exp := normalizeSMTPFields(&req)

	var existing model.SMTPConfig
	if err := h.db.First(&existing).Error; err != nil {
		row := model.SMTPConfig{
			Enabled:           req.Enabled,
			Host:              strings.TrimSpace(req.Host),
			Port:              port,
			Security:          sec,
			Username:          strings.TrimSpace(req.Username),
			Password:          strings.TrimSpace(req.Password),
			FromAddress:       strings.TrimSpace(req.FromAddress),
			FromName:          strings.TrimSpace(req.FromName),
			SkipTLSVerify:     req.SkipTLSVerify,
			EmailLoginPolicy:  policy,
			CodeExpireMinutes: exp,
		}
		if err := h.db.Create(&row).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "save smtp config failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "saved"})
		return
	}

	updates := map[string]interface{}{
		"enabled":             req.Enabled,
		"host":                strings.TrimSpace(req.Host),
		"port":                port,
		"security":            sec,
		"username":            strings.TrimSpace(req.Username),
		"from_address":        strings.TrimSpace(req.FromAddress),
		"from_name":           strings.TrimSpace(req.FromName),
		"skip_tls_verify":     req.SkipTLSVerify,
		"email_login_policy":  policy,
		"code_expire_minutes": exp,
	}
	pw := strings.TrimSpace(req.Password)
	if pw != "" && pw != secretMask {
		updates["password"] = pw
	}

	if err := h.db.Model(&existing).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update smtp config failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "saved"})
}

// TestSMTP sends a test verification email using the provided (or stored) SMTP settings.
func (h *AdminSystemHandler) TestSMTP(c *gin.Context) {
	var req smtpConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	port, sec, policy, exp := normalizeSMTPFields(&req)

	pw := strings.TrimSpace(req.Password)
	if pw == "" || pw == secretMask {
		if existing, err := h.emailSvc.GetConfig(); err == nil {
			pw = existing.Password
		}
	}

	cfg := &model.SMTPConfig{
		Enabled:           true,
		Host:              strings.TrimSpace(req.Host),
		Port:              port,
		Security:          sec,
		Username:          strings.TrimSpace(req.Username),
		Password:          pw,
		FromAddress:       strings.TrimSpace(req.FromAddress),
		FromName:          strings.TrimSpace(req.FromName),
		SkipTLSVerify:     req.SkipTLSVerify,
		EmailLoginPolicy:  policy,
		CodeExpireMinutes: exp,
	}
	if err := h.emailSvc.SendTestEmail(cfg, req.To); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "测试邮件发送成功，请查收收件箱"})
}

type clientStrategyReq struct {
	Enabled             bool   `json:"enabled"`
	ImageQuality        string `json:"image_quality"`
	CustomFPS           int    `json:"custom_fps"`
	CustomImageQuality  int    `json:"custom_image_quality"`
	CodecPreference     string `json:"codec_preference"`
	EnableHWCodec       bool   `json:"enable_hwcodec"`
	EnableAudio         bool   `json:"enable_audio"`
	EnableClipboard     bool   `json:"enable_clipboard"`
	EnableFileTransfer  bool   `json:"enable_file_transfer"`
	EnableKeyboard      bool   `json:"enable_keyboard"`
	EnableTunnel        bool   `json:"enable_tunnel"`
	EnableRemoteRestart bool   `json:"enable_remote_restart"`
	EnableRecordSession bool   `json:"enable_record_session"`
	DirectServer        bool   `json:"direct_server"`
	VerificationMethod  string `json:"verification_method"`
	ApproveMode         string `json:"approve_mode"`
	ExtraOptions        string `json:"extra_options"`
}

// GetClientStrategy returns the singleton RustDesk client default strategy and generated config_options.
func (h *AdminSystemHandler) GetClientStrategy(c *gin.Context) {
	snap, err := h.strategySvc.Reload()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load client strategy failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"strategy":       snap.Strategy,
		"config_options": snap.ConfigOptions,
	})
}

// UpdateClientStrategy updates centralized client defaults and bumps StrategyModifiedAt for heartbeat sync.
func (h *AdminSystemHandler) UpdateClientStrategy(c *gin.Context) {
	var req clientStrategyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	candidate := model.ClientStrategy{
		Enabled:             req.Enabled,
		ImageQuality:        req.ImageQuality,
		CustomFPS:           req.CustomFPS,
		CustomImageQuality:  req.CustomImageQuality,
		CodecPreference:     req.CodecPreference,
		EnableHWCodec:       req.EnableHWCodec,
		EnableAudio:         req.EnableAudio,
		EnableClipboard:     req.EnableClipboard,
		EnableFileTransfer:  req.EnableFileTransfer,
		EnableKeyboard:      req.EnableKeyboard,
		EnableTunnel:        req.EnableTunnel,
		EnableRemoteRestart: req.EnableRemoteRestart,
		EnableRecordSession: req.EnableRecordSession,
		DirectServer:        req.DirectServer,
		VerificationMethod:  req.VerificationMethod,
		ApproveMode:         req.ApproveMode,
		ExtraOptions:        req.ExtraOptions,
	}

	snap, err := h.strategySvc.Update(candidate)
	if err != nil {
		if errors.Is(err, service.ErrInvalidStrategyOptions) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update client strategy failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "saved",
		"strategy":       snap.Strategy,
		"config_options": snap.ConfigOptions,
	})
}
