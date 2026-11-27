package service

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"rustdesk-api/internal/config"
	"rustdesk-api/internal/model"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserDisabled       = errors.New("user account is disabled")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

// Claims defines the JWT claims payload issued by this API server.
type Claims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

type cachedTokenEntry struct {
	user           model.User
	tokenExpiresAt time.Time
	cacheExpiresAt time.Time
}

// AuthService handles password hashing, JWT issuance/validation, and LDAP authentication.
type AuthService struct {
	db         *gorm.DB
	cfg        *config.Config
	jwtSecret  []byte
	cacheMu    sync.RWMutex
	tokenCache map[string]cachedTokenEntry
}

// NewAuthService creates a new AuthService.
func NewAuthService(db *gorm.DB, cfg *config.Config) *AuthService {
	return &AuthService{
		db:         db,
		cfg:        cfg,
		jwtSecret:  []byte(cfg.JWTSecret),
		tokenCache: make(map[string]cachedTokenEntry, 256),
	}
}

// HashPassword hashes a plaintext password using bcrypt.
func (s *AuthService) HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword compares a bcrypt password hash against a plaintext candidate.
func (s *AuthService) VerifyPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// EnsureDefaultGroup guarantees the "default" user group exists and returns it.
func (s *AuthService) EnsureDefaultGroup() (*model.Group, error) {
	var grp model.Group
	if err := s.db.Where("name = ?", "default").First(&grp).Error; err == nil {
		return &grp, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	grp = model.Group{Name: "default", Note: "Default user group", Type: 1}
	if err := s.db.Create(&grp).Error; err != nil {
		return nil, err
	}
	return &grp, nil
}

// IssueToken creates a persistent UserToken row and returns a signed HS256 JWT.
func (s *AuthService) IssueToken(user *model.User, deviceID, deviceUUID, ip string) (string, error) {
	if user.Status != 1 {
		return "", ErrUserDisabled
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(s.cfg.JWTExpireHours) * time.Hour)
	jti := uuid.NewString()

	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   user.Username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", err
	}

	tokenRow := model.UserToken{
		TokenID:    jti,
		UserID:     user.ID,
		Username:   user.Username,
		DeviceID:   deviceID,
		DeviceUUID: deviceUUID,
		IP:         ip,
		ExpiresAt:  expiresAt,
		Revoked:    false,
	}
	if err := s.db.Create(&tokenRow).Error; err != nil {
		return "", err
	}

	s.db.Model(&model.User{}).Where("id = ?", user.ID).Update("last_login_at", now)
	return signed, nil
}

// ValidateToken verifies the JWT signature, token record status, and active user status.
func (s *AuthService) ValidateToken(tokenStr string) (*model.User, string, error) {
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return nil, "", ErrInvalidToken
	}

	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil || !parsed.Valid || claims.ID == "" {
		return nil, "", ErrInvalidToken
	}

	now := time.Now()
	s.cacheMu.RLock()
	entry, ok := s.tokenCache[claims.ID]
	s.cacheMu.RUnlock()
	if ok && now.Before(entry.cacheExpiresAt) && now.Before(entry.tokenExpiresAt) && entry.user.Status == 1 {
		userCopy := entry.user
		return &userCopy, claims.ID, nil
	}

	var tokenRow model.UserToken
	if err := s.db.Where("token_id = ? AND revoked = ?", claims.ID, false).First(&tokenRow).Error; err != nil {
		return nil, "", ErrInvalidToken
	}
	if now.After(tokenRow.ExpiresAt) {
		return nil, "", ErrInvalidToken
	}

	var user model.User
	if err := s.db.First(&user, tokenRow.UserID).Error; err != nil {
		return nil, "", ErrInvalidToken
	}
	if user.Status != 1 {
		return nil, "", ErrUserDisabled
	}

	s.cacheMu.Lock()
	if len(s.tokenCache) >= 4096 {
		for k, v := range s.tokenCache {
			if !now.Before(v.cacheExpiresAt) || !now.Before(v.tokenExpiresAt) {
				delete(s.tokenCache, k)
			}
		}
		if len(s.tokenCache) >= 4096 {
			s.tokenCache = make(map[string]cachedTokenEntry, 256)
		}
	}
	s.tokenCache[claims.ID] = cachedTokenEntry{
		user:           user,
		tokenExpiresAt: tokenRow.ExpiresAt,
		cacheExpiresAt: now.Add(15 * time.Second),
	}
	s.cacheMu.Unlock()

	return &user, claims.ID, nil
}

// RevokeToken marks a specific JWT session token as revoked and evicts its cache entry.
func (s *AuthService) RevokeToken(jti string) error {
	if jti == "" {
		return nil
	}
	s.cacheMu.Lock()
	delete(s.tokenCache, jti)
	s.cacheMu.Unlock()
	return s.db.Model(&model.UserToken{}).Where("token_id = ?", jti).Update("revoked", true).Error
}

// RevokeUserTokens revokes all active tokens belonging to a user and evicts cached sessions.
func (s *AuthService) RevokeUserTokens(userID uint) error {
	s.InvalidateUserCache(userID)
	return s.db.Model(&model.UserToken{}).Where("user_id = ? AND revoked = ?", userID, false).Update("revoked", true).Error
}

// InvalidateUserCache removes all cached token entries associated with userID.
func (s *AuthService) InvalidateUserCache(userID uint) {
	if userID == 0 {
		return
	}
	s.cacheMu.Lock()
	for jti, entry := range s.tokenCache {
		if entry.user.ID == userID {
			delete(s.tokenCache, jti)
		}
	}
	s.cacheMu.Unlock()
}

// ClearUserCache clears the entire in-memory token validation cache.
func (s *AuthService) ClearUserCache() {
	s.cacheMu.Lock()
	s.tokenCache = make(map[string]cachedTokenEntry, 256)
	s.cacheMu.Unlock()
}

// AuthenticateLocal verifies username and password against local database users.
func (s *AuthService) AuthenticateLocal(username, password string) (*model.User, error) {
	var user model.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, ErrInvalidCredentials
	}
	if user.Status != 1 {
		return nil, ErrUserDisabled
	}
	if !s.VerifyPassword(user.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	return &user, nil
}

// AuthenticateLDAP validates user credentials against the configured LDAP / Active Directory server.
func (s *AuthService) AuthenticateLDAP(username, password string) (*model.User, error) {
	if strings.TrimSpace(username) == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	var cfg model.LDAPConfig
	if err := s.db.First(&cfg).Error; err != nil || !cfg.Enabled || strings.TrimSpace(cfg.URL) == "" {
		return nil, ErrInvalidCredentials
	}

	conn, err := s.dialLDAP(&cfg)
	if err != nil {
		return nil, fmt.Errorf("ldap connect: %w", err)
	}
	defer conn.Close()

	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			return nil, fmt.Errorf("ldap service bind: %w", err)
		}
	}

	filterTemplate := strings.TrimSpace(cfg.UserFilter)
	if filterTemplate == "" {
		filterTemplate = "(&(objectClass=person)(|(uid=%s)(sAMAccountName=%s)(mail=%s)))"
	}
	escaped := ldap.EscapeFilter(username)
	filter := strings.ReplaceAll(filterTemplate, "%s", escaped)

	usernameAttr := strings.TrimSpace(cfg.UsernameAttr)
	if usernameAttr == "" {
		usernameAttr = "uid"
	}
	emailAttr := strings.TrimSpace(cfg.EmailAttr)
	if emailAttr == "" {
		emailAttr = "mail"
	}

	searchReq := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		2,
		10,
		false,
		filter,
		[]string{"dn", usernameAttr, emailAttr, "cn", "displayName"},
		nil,
	)
	res, err := conn.Search(searchReq)
	if err != nil || len(res.Entries) != 1 {
		return nil, ErrInvalidCredentials
	}

	entry := res.Entries[0]
	if err := conn.Bind(entry.DN, password); err != nil {
		return nil, ErrInvalidCredentials
	}

	resolvedName := strings.TrimSpace(entry.GetAttributeValue(usernameAttr))
	if resolvedName == "" {
		resolvedName = username
	}
	resolvedEmail := strings.TrimSpace(entry.GetAttributeValue(emailAttr))

	var user model.User
	err = s.db.Where("username = ?", resolvedName).First(&user).Error
	if err == nil {
		if user.Status != 1 {
			return nil, ErrUserDisabled
		}
		if resolvedEmail != "" && user.Email == "" {
			s.db.Model(&user).Update("email", resolvedEmail)
			user.Email = resolvedEmail
		}
		return &user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if !cfg.AutoRegister {
		return nil, ErrInvalidCredentials
	}

	defaultGrp, err := s.EnsureDefaultGroup()
	if err != nil {
		return nil, err
	}

	user = model.User{
		Username:   resolvedName,
		Email:      resolvedEmail,
		Status:     1,
		IsAdmin:    false,
		GroupID:    defaultGrp.ID,
		GroupName:  defaultGrp.Name,
		AuthSource: "ldap",
		ExternalID: entry.DN,
	}
	if err := s.db.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// TestLDAPConnection verifies network connectivity and bind credentials for an LDAP config.
func (s *AuthService) TestLDAPConnection(cfg *model.LDAPConfig) error {
	if strings.TrimSpace(cfg.URL) == "" {
		return errors.New("LDAP URL is required")
	}
	conn, err := s.dialLDAP(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			return fmt.Errorf("bind failed: %w", err)
		}
	}
	if strings.TrimSpace(cfg.BaseDN) != "" {
		req := ldap.NewSearchRequest(
			cfg.BaseDN,
			ldap.ScopeBaseObject,
			ldap.NeverDerefAliases,
			1,
			5,
			false,
			"(objectClass=*)",
			[]string{"dn"},
			nil,
		)
		if _, err := conn.Search(req); err != nil {
			return fmt.Errorf("base DN search failed: %w", err)
		}
	}
	return nil
}

func (s *AuthService) dialLDAP(cfg *model.LDAPConfig) (*ldap.Conn, error) {
	if strings.HasPrefix(strings.ToLower(cfg.URL), "ldaps://") {
		return ldap.DialURL(cfg.URL, ldap.DialWithTLSConfig(&tls.Config{
			InsecureSkipVerify: cfg.SkipTLSVerify,
		}))
	}
	return ldap.DialURL(cfg.URL)
}

// BuildClientUserPayload formats a User into the JSON object expected by official RustDesk clients.
func (s *AuthService) BuildClientUserPayload(user *model.User) map[string]interface{} {
	grp := user.GroupName
	if grp == "" {
		grp = "default"
	}
	return map[string]interface{}{
		"name":     user.Username,
		"email":    user.Email,
		"note":     user.Note,
		"status":   user.Status,
		"grp":      grp,
		"is_admin": user.IsAdmin,
		"info":     map[string]interface{}{},
	}
}
