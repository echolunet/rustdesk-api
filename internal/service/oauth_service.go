package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"rustdesk-api/internal/model"
)

// OAuthService orchestrates OIDC, GitHub, and Google SSO login handshakes for RustDesk clients.
type OAuthService struct {
	db      *gorm.DB
	authSvc *AuthService
	httpCli *http.Client
}

// NewOAuthService creates a new OAuthService.
func NewOAuthService(db *gorm.DB, authSvc *AuthService) *OAuthService {
	return &OAuthService{
		db:      db,
		authSvc: authSvc,
		httpCli: &http.Client{Timeout: 10 * time.Second},
	}
}

type oidcDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

type resolvedEndpoints struct {
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	Scopes      string
}

type externalIdentity struct {
	ExternalID string
	Username   string
	Email      string
}

// ListEnabledOptions builds the /api/login-options JSON string array expected by RustDesk clients.
func (s *OAuthService) ListEnabledOptions() ([]string, error) {
	var providers []model.OAuthProvider
	if err := s.db.Where("enabled = ?", true).Order("id asc").Find(&providers).Error; err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return []string{}, nil
	}

	options := make([]string, 0, len(providers)+1)
	commonItems := make([]map[string]string, 0, len(providers))
	for _, p := range providers {
		options = append(options, "oidc/"+p.Name)
		item := map[string]string{
			"name": p.Name,
		}
		if p.Icon != "" {
			item["icon"] = p.Icon
		}
		commonItems = append(commonItems, item)
	}

	if raw, err := json.Marshal(commonItems); err == nil {
		options = append(options, "common-oidc/"+string(raw))
	}
	return options, nil
}

// StartAuth creates a pending OAuthSession and returns the tracking code and IdP authorization URL.
func (s *OAuthService) StartAuth(providerName, deviceID, deviceUUID, deviceInfo, baseURL string) (string, string, error) {
	var provider model.OAuthProvider
	if err := s.db.Where("name = ? AND enabled = ?", providerName, true).First(&provider).Error; err != nil {
		return "", "", fmt.Errorf("oauth provider %q not found or disabled", providerName)
	}

	endpoints, err := s.resolveEndpoints(&provider)
	if err != nil {
		return "", "", err
	}

	code, err := randomHex(16)
	if err != nil {
		return "", "", err
	}

	redirectURI := strings.TrimSpace(provider.RedirectURL)
	if redirectURI == "" {
		redirectURI = strings.TrimRight(baseURL, "/") + "/api/oidc/callback"
	}

	u, err := url.Parse(endpoints.AuthURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid auth url: %w", err)
	}
	q := u.Query()
	q.Set("client_id", provider.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", endpoints.Scopes)
	q.Set("state", code)
	u.RawQuery = q.Encode()

	sess := model.OAuthSession{
		Code:         code,
		ProviderName: provider.Name,
		DeviceID:     deviceID,
		DeviceUUID:   deviceUUID,
		DeviceInfo:   deviceInfo,
		Status:       "pending",
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	if err := s.db.Create(&sess).Error; err != nil {
		return "", "", err
	}
	return code, u.String(), nil
}

// HandleCallback processes the IdP redirect callback, exchanges the code, provisions/matches the user, and stores a JWT.
func (s *OAuthService) HandleCallback(idpCode, state, baseURL, clientIP string) (*model.User, error) {
	if strings.TrimSpace(idpCode) == "" || strings.TrimSpace(state) == "" {
		return nil, errors.New("missing authorization code or state")
	}

	var sess model.OAuthSession
	if err := s.db.Where("code = ?", state).First(&sess).Error; err != nil {
		return nil, errors.New("invalid or unknown OAuth session")
	}
	if time.Now().After(sess.ExpiresAt) {
		s.db.Model(&sess).Updates(map[string]interface{}{"status": "failed", "error_msg": "session expired"})
		return nil, errors.New("OAuth session expired, please retry from RustDesk client")
	}
	if sess.Status != "pending" {
		return nil, errors.New("OAuth session has already been processed")
	}

	var provider model.OAuthProvider
	if err := s.db.Where("name = ? AND enabled = ?", sess.ProviderName, true).First(&provider).Error; err != nil {
		s.markSessionFailed(&sess, "provider unavailable")
		return nil, errors.New("OAuth provider is disabled or missing")
	}

	endpoints, err := s.resolveEndpoints(&provider)
	if err != nil {
		s.markSessionFailed(&sess, err.Error())
		return nil, err
	}

	redirectURI := strings.TrimSpace(provider.RedirectURL)
	if redirectURI == "" {
		redirectURI = strings.TrimRight(baseURL, "/") + "/api/oidc/callback"
	}

	accessToken, err := s.exchangeCode(endpoints.TokenURL, provider.ClientID, provider.ClientSecret, idpCode, redirectURI)
	if err != nil {
		s.markSessionFailed(&sess, err.Error())
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	ident, err := s.fetchIdentity(&provider, endpoints.UserInfoURL, accessToken)
	if err != nil {
		s.markSessionFailed(&sess, err.Error())
		return nil, fmt.Errorf("fetch user profile failed: %w", err)
	}

	user, err := s.resolveOrCreateUser(&provider, ident)
	if err != nil {
		s.markSessionFailed(&sess, err.Error())
		return nil, err
	}

	jwtToken, err := s.authSvc.IssueToken(user, sess.DeviceID, sess.DeviceUUID, clientIP)
	if err != nil {
		s.markSessionFailed(&sess, err.Error())
		return nil, err
	}

	s.db.Model(&sess).Updates(map[string]interface{}{
		"status":       "completed",
		"user_id":      user.ID,
		"access_token": jwtToken,
		"error_msg":    "",
	})

	s.db.Create(&model.LoginLog{
		UserID:     user.ID,
		Username:   user.Username,
		AuthSource: "oauth:" + provider.Name,
		DeviceID:   sess.DeviceID,
		DeviceUUID: sess.DeviceUUID,
		IP:         clientIP,
		Status:     "success",
		Message:    "SSO login completed",
	})

	return user, nil
}

// QueryAuth checks the state of an OAuthSession; returns (user, token, pending, err).
func (s *OAuthService) QueryAuth(code string) (*model.User, string, bool, error) {
	s.db.Where("expires_at < ?", time.Now().Add(-1*time.Hour)).Delete(&model.OAuthSession{})

	var sess model.OAuthSession
	if err := s.db.Where("code = ?", code).First(&sess).Error; err != nil {
		return nil, "", false, errors.New("session not found")
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, "", false, errors.New("session expired")
	}

	switch sess.Status {
	case "pending":
		return nil, "", true, nil
	case "failed":
		msg := sess.ErrorMsg
		if msg == "" {
			msg = "authentication failed"
		}
		return nil, "", false, errors.New(msg)
	case "completed":
		var user model.User
		if err := s.db.First(&user, sess.UserID).Error; err != nil {
			return nil, "", false, errors.New("user not found")
		}
		if user.Status != 1 {
			return nil, "", false, ErrUserDisabled
		}
		token := sess.AccessToken
		s.db.Model(&sess).Updates(map[string]interface{}{
			"status":       "consumed",
			"access_token": "",
		})
		return &user, token, false, nil
	default:
		return nil, "", false, errors.New("session already consumed")
	}
}

func (s *OAuthService) markSessionFailed(sess *model.OAuthSession, msg string) {
	if len(msg) > 500 {
		msg = msg[:500]
	}
	s.db.Model(sess).Updates(map[string]interface{}{
		"status":    "failed",
		"error_msg": msg,
	})
}

func (s *OAuthService) resolveEndpoints(p *model.OAuthProvider) (*resolvedEndpoints, error) {
	res := &resolvedEndpoints{
		AuthURL:     strings.TrimSpace(p.AuthURL),
		TokenURL:    strings.TrimSpace(p.TokenURL),
		UserInfoURL: strings.TrimSpace(p.UserInfoURL),
		Scopes:      strings.TrimSpace(p.Scopes),
	}

	switch strings.ToLower(p.ProviderType) {
	case "github":
		if res.AuthURL == "" {
			res.AuthURL = "https://github.com/login/oauth/authorize"
		}
		if res.TokenURL == "" {
			res.TokenURL = "https://github.com/login/oauth/access_token"
		}
		if res.UserInfoURL == "" {
			res.UserInfoURL = "https://api.github.com/user"
		}
		if res.Scopes == "" {
			res.Scopes = "read:user user:email"
		}
	case "google":
		if res.AuthURL == "" {
			res.AuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
		}
		if res.TokenURL == "" {
			res.TokenURL = "https://oauth2.googleapis.com/token"
		}
		if res.UserInfoURL == "" {
			res.UserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
		}
		if res.Scopes == "" {
			res.Scopes = "openid email profile"
		}
	default:
		if res.Scopes == "" {
			res.Scopes = "openid email profile"
		}
		if (res.AuthURL == "" || res.TokenURL == "" || res.UserInfoURL == "") && strings.TrimSpace(p.Issuer) != "" {
			discURL := strings.TrimRight(strings.TrimSpace(p.Issuer), "/") + "/.well-known/openid-configuration"
			resp, err := s.httpCli.Get(discURL)
			if err != nil {
				return nil, fmt.Errorf("fetch oidc discovery: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				return nil, fmt.Errorf("oidc discovery returned HTTP %d", resp.StatusCode)
			}
			var disc oidcDiscovery
			if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
				return nil, fmt.Errorf("decode oidc discovery: %w", err)
			}
			if res.AuthURL == "" {
				res.AuthURL = disc.AuthorizationEndpoint
			}
			if res.TokenURL == "" {
				res.TokenURL = disc.TokenEndpoint
			}
			if res.UserInfoURL == "" {
				res.UserInfoURL = disc.UserinfoEndpoint
			}
		}
	}

	if res.AuthURL == "" || res.TokenURL == "" || res.UserInfoURL == "" {
		return nil, errors.New("incomplete OAuth endpoints (check Issuer or AuthURL/TokenURL/UserInfoURL)")
	}
	return res, nil
}

func (s *OAuthService) exchangeCode(tokenURL, clientID, clientSecret, code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpCli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		vals, parseErr := url.ParseQuery(string(body))
		if parseErr != nil {
			return "", err
		}
		if tok := vals.Get("access_token"); tok != "" {
			return tok, nil
		}
		return "", fmt.Errorf("unexpected token response: %s", string(body))
	}
	if tokenResp.Error != "" {
		return "", fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	if tokenResp.AccessToken == "" {
		return "", errors.New("empty access_token returned by provider")
	}
	return tokenResp.AccessToken, nil
}

func (s *OAuthService) fetchIdentity(p *model.OAuthProvider, userInfoURL, accessToken string) (*externalIdentity, error) {
	req, err := http.NewRequest(http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "rustdesk-api")

	resp, err := s.httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("userinfo HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	ident := &externalIdentity{}
	if strings.EqualFold(p.ProviderType, "github") {
		if idVal, ok := raw["id"]; ok {
			ident.ExternalID = fmt.Sprintf("%v", idVal)
		}
		ident.Username = firstNonEmpty(stringVal(raw["login"]), stringVal(raw["name"]))
		ident.Email = stringVal(raw["email"])
		if ident.Email == "" {
			ident.Email = s.fetchGitHubPrimaryEmail(accessToken)
		}
	} else {
		ident.ExternalID = firstNonEmpty(stringVal(raw["sub"]), stringVal(raw["id"]))
		ident.Email = stringVal(raw["email"])
		ident.Username = firstNonEmpty(
			stringVal(raw["preferred_username"]),
			stringVal(raw["nickname"]),
			stringVal(raw["name"]),
			strings.Split(ident.Email, "@")[0],
		)
	}

	if ident.ExternalID == "" {
		ident.ExternalID = ident.Email
	}
	if ident.ExternalID == "" {
		return nil, errors.New("provider did not return sub or id")
	}
	if ident.Username == "" {
		ident.Username = p.Name + "_" + ident.ExternalID
	}
	return ident, nil
}

func (s *OAuthService) fetchGitHubPrimaryEmail(accessToken string) string {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "rustdesk-api")

	resp, err := s.httpCli.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ""
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return ""
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email
		}
	}
	return ""
}

func (s *OAuthService) resolveOrCreateUser(p *model.OAuthProvider, ident *externalIdentity) (*model.User, error) {
	authSource := "oauth:" + p.Name

	var user model.User
	err := s.db.Where("auth_source = ? AND external_id = ?", authSource, ident.ExternalID).First(&user).Error
	if err == nil {
		if user.Status != 1 {
			return nil, ErrUserDisabled
		}
		if ident.Email != "" && user.Email != ident.Email {
			s.db.Model(&user).Update("email", ident.Email)
			user.Email = ident.Email
		}
		return &user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if !p.AutoRegister {
		return nil, errors.New("account does not exist and auto-registration is disabled")
	}

	defaultGrp, err := s.authSvc.EnsureDefaultGroup()
	if err != nil {
		return nil, err
	}

	candidate := sanitizeUsername(ident.Username)
	if candidate == "" {
		candidate = p.Name + "_user"
	}
	username := candidate
	for i := 1; i <= 50; i++ {
		var count int64
		s.db.Model(&model.User{}).Where("username = ?", username).Count(&count)
		if count == 0 {
			break
		}
		username = fmt.Sprintf("%s_%d", candidate, i)
	}

	user = model.User{
		Username:   username,
		Email:      ident.Email,
		Status:     1,
		IsAdmin:    false,
		GroupID:    defaultGrp.ID,
		GroupName:  defaultGrp.Name,
		AuthSource: authSource,
		ExternalID: ident.ExternalID,
	}
	if err := s.db.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

var nonUsernameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeUsername(s string) string {
	s = strings.TrimSpace(s)
	s = nonUsernameChars.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func stringVal(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func randomHex(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
