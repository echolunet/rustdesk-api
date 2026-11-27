package router_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"rustdesk-api/internal/config"
	"rustdesk-api/internal/database"
	"rustdesk-api/internal/model"
	"rustdesk-api/internal/router"
	"rustdesk-api/internal/service"
)

func setupTestServer(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "id_ed25519.pub")
	if err := os.WriteFile(keyPath, []byte("test-official-ed25519-public-key\n"), 0o644); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	cfg := &config.Config{
		AppHost:             "127.0.0.1",
		AppPort:             21114,
		AppMode:             "test",
		AppVersion:          "1.0.0-test",
		DataDir:             tmpDir,
		WebClientDir:        filepath.Join(tmpDir, "webclient"),
		DBType:              "sqlite",
		DBDSN:               filepath.Join(tmpDir, "test.db"),
		JWTSecret:           "test-jwt-secret-key-at-least-32-chars",
		JWTExpireHours:      24,
		AdminInitUser:       "admin",
		AdminInitPassword:   "AdminPass123!",
		RustDeskIDServer:    "rd.example.com:21116",
		RustDeskRelayServer: "rd.example.com:21117",
		RustDeskAPIServer:   "https://rd.example.com",
		RustDeskKeyFile:           keyPath,
		HBBSWSTarget:              "http://127.0.0.1:21118",
		HBBRWSTarget:              "http://127.0.0.1:21119",
		ClientStrategyEnabled:     true,
		ClientDefaultImageQuality: "custom",
		ClientDefaultFPS:          60,
		ClientDefaultQuality:      75,
		ClientDefaultCodec:        "auto",
		ClientEnableHWCodec:       true,
		ClientEnableAudio:         true,
		ClientEnableClipboard:     true,
		ClientEnableFileTransfer:  true,
		ClientEnableKeyboard:      true,
		ClientEnableTunnel:        true,
		ClientEnableRemoteRestart: true,
	}

	db, err := database.Init(cfg)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	return router.New(db, cfg), db
}

func doJSON(t *testing.T, h http.Handler, method, path, token string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEndToEndClientAndAdminAPIs(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Health & Admin UI check
	rec := doJSON(t, srv, http.MethodGet, "/health", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /health, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, srv, http.MethodGet, "/_admin/", "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "RustDesk") {
		t.Fatalf("expected admin html on /_admin/, got %d", rec.Code)
	}

	// 2. Client Login & currentUser
	rec = doJSON(t, srv, http.MethodPost, "/api/login", "", map[string]interface{}{
		"username": "admin",
		"password": "AdminPass123!",
		"id":       "100200300",
		"uuid":     "uuid-host-1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed (%d): %s", rec.Code, rec.Body.String())
	}
	var loginResp struct {
		Type        string `json:"type"`
		AccessToken string `json:"access_token"`
		User        struct {
			Name    string `json:"name"`
			IsAdmin bool   `json:"is_admin"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResp.Type != "access_token" || loginResp.AccessToken == "" || !loginResp.User.IsAdmin {
		t.Fatalf("unexpected login response: %+v", loginResp)
	}
	token := loginResp.AccessToken

	rec = doJSON(t, srv, http.MethodGet, "/api/currentUser", token, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"admin"`) {
		t.Fatalf("currentUser failed (%d): %s", rec.Code, rec.Body.String())
	}

	// 3. Heartbeat & Sysinfo (including UUID spoofing protection)
	rec = doJSON(t, srv, http.MethodPost, "/api/heartbeat", "", map[string]interface{}{
		"id":          "100200300",
		"uuid":        "uuid-host-1",
		"ver":         "1.3.8",
		"modified_at": 0,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat failed (%d): %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, srv, http.MethodPost, "/api/sysinfo", "", map[string]interface{}{
		"id":       "100200300",
		"uuid":     "uuid-host-1",
		"hostname": "dev-workstation",
		"username": "alice",
		"os":       "Windows 11",
		"cpu":      "AMD Ryzen",
		"memory":   32,
		"version":  "1.3.8",
	})
	if rec.Code != http.StatusOK || rec.Body.String() != "SYSINFO_UPDATED" {
		t.Fatalf("sysinfo failed (%d): %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, srv, http.MethodPost, "/api/sysinfo", "", map[string]interface{}{
		"id":       "100200300",
		"uuid":     "spoofed-uuid-999",
		"hostname": "evil",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on UUID mismatch, got %d", rec.Code)
	}

	// 4. Modern Address Book v2 & Legacy v1 Sync
	rec = doJSON(t, srv, http.MethodPost, "/api/ab/personal", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ab personal failed (%d): %s", rec.Code, rec.Body.String())
	}
	var personal struct {
		GUID string `json:"guid"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &personal)
	if personal.GUID == "" {
		t.Fatalf("empty personal AB guid")
	}

	rec = doJSON(t, srv, http.MethodPost, "/api/ab/peer/add/"+personal.GUID, token, map[string]interface{}{
		"id":       "987654321",
		"alias":    "Prod Server",
		"hostname": "prod-01",
		"platform": "Linux",
		"tags":     []string{"Ops"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("ab peer add failed (%d): %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, srv, http.MethodPost, "/api/ab/tag/rename/"+personal.GUID, token, map[string]interface{}{
		"old": "Ops",
		"new": "Production",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("ab tag rename failed (%d): %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, srv, http.MethodGet, "/api/ab", token, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Production") || !strings.Contains(rec.Body.String(), "987654321") {
		t.Fatalf("legacy ab sync failed (%d): %s", rec.Code, rec.Body.String())
	}

	// 5. Audit & Peers
	rec = doJSON(t, srv, http.MethodPost, "/api/audit/conn", "", map[string]interface{}{
		"action":    "new",
		"peer_id":   "100200300",
		"from_peer": "987654321",
		"from_name": "admin",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("audit conn failed (%d): %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, srv, http.MethodGet, "/api/peers", token, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "dev-workstation") {
		t.Fatalf("peers list failed (%d): %s", rec.Code, rec.Body.String())
	}

	// 6. Admin Server Config & Reversed Base64 Client Import String
	rec = doJSON(t, srv, http.MethodGet, "/admin/api/system/server-config", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("server-config failed (%d): %s", rec.Code, rec.Body.String())
	}
	var sysCfg struct {
		PublicKey    string `json:"public_key"`
		ConfigString string `json:"config_string"`
		QRDataURI    string `json:"qr_data_uri"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &sysCfg)
	if sysCfg.PublicKey != "test-official-ed25519-public-key" || sysCfg.ConfigString == "" || !strings.HasPrefix(sysCfg.QRDataURI, "data:image/png;base64,") {
		t.Fatalf("unexpected server-config: %+v", sysCfg)
	}
	rev := []byte(sysCfg.ConfigString)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	decoded, err := base64.RawStdEncoding.DecodeString(string(rev))
	if err != nil || !strings.Contains(string(decoded), "test-official-ed25519-public-key") {
		t.Fatalf("config_string decode check failed: err=%v decoded=%s", err, string(decoded))
	}

	// 7. OAuth login-options & OIDC auth start
	rec = doJSON(t, srv, http.MethodPost, "/admin/api/oauth/providers", token, map[string]interface{}{
		"name":          "github",
		"provider_type": "github",
		"display_name":  "GitHub",
		"client_id":     "gh-client-id",
		"client_secret": "gh-secret",
		"auto_register": true,
		"enabled":       true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create oauth provider failed (%d): %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, srv, http.MethodGet, "/api/login-options", "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "oidc/github") {
		t.Fatalf("login-options failed (%d): %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, srv, http.MethodPost, "/api/oidc/auth", "", map[string]interface{}{
		"op":   "github",
		"id":   "100200300",
		"uuid": "uuid-host-1",
	})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "github.com/login/oauth/authorize") {
		t.Fatalf("oidc auth start failed (%d): %s", rec.Code, rec.Body.String())
	}
}

func TestTOTP2FAAndEmailVerificationFlows(t *testing.T) {
	srv, db := setupTestServer(t)

	// 1. Admin login & setup TOTP 2FA
	rec := doJSON(t, srv, http.MethodPost, "/admin/api/login", "", map[string]interface{}{
		"username": "admin",
		"password": "AdminPass123!",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("admin login failed (%d): %s", rec.Code, rec.Body.String())
	}
	var adminLogin struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &adminLogin)
	token := adminLogin.AccessToken

	rec = doJSON(t, srv, http.MethodPost, "/admin/api/me/2fa/setup", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa setup failed (%d): %s", rec.Code, rec.Body.String())
	}
	var setupResp struct {
		Secret     string `json:"secret"`
		OTPAuthURI string `json:"otpauth_uri"`
		QRDataURI  string `json:"qr_data_uri"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &setupResp)
	if setupResp.Secret == "" || !strings.HasPrefix(setupResp.OTPAuthURI, "otpauth://totp/") || !strings.HasPrefix(setupResp.QRDataURI, "data:image/png;base64,") {
		t.Fatalf("unexpected 2fa setup response: %+v", setupResp)
	}

	totpSvc := service.NewTOTPService(db)
	validCode, err := totpSvc.GenerateCode(setupResp.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp code: %v", err)
	}

	rec = doJSON(t, srv, http.MethodPost, "/admin/api/me/2fa/confirm", token, map[string]interface{}{
		"code": validCode,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("2fa confirm failed (%d): %s", rec.Code, rec.Body.String())
	}
	var confirmResp struct {
		TOTPEnabled   bool     `json:"totp_enabled"`
		RecoveryCodes []string `json:"recovery_codes"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &confirmResp)
	if !confirmResp.TOTPEnabled || len(confirmResp.RecoveryCodes) != 6 {
		t.Fatalf("unexpected 2fa confirm response: %+v", confirmResp)
	}

	// 2. Client login receives tfa_check when code omitted, rejects invalid code, accepts valid code & single-use recovery code
	rec = doJSON(t, srv, http.MethodPost, "/api/login", "", map[string]interface{}{
		"username": "admin",
		"password": "AdminPass123!",
		"id":       "200300400",
		"uuid":     "uuid-2fa-device",
	})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tfa_check"`) {
		t.Fatalf("expected tfa_check challenge, got (%d): %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, srv, http.MethodPost, "/api/login", "", map[string]interface{}{
		"username": "admin",
		"password": "AdminPass123!",
		"id":       "200300400",
		"uuid":     "uuid-2fa-device",
		"tfaCode":  "000000",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid 2fa code, got %d", rec.Code)
	}

	rec = doJSON(t, srv, http.MethodPost, "/api/login", "", map[string]interface{}{
		"username": "admin",
		"password": "AdminPass123!",
		"id":       "200300400",
		"uuid":     "uuid-2fa-device",
		"tfaCode":  validCode,
	})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"access_token"`) {
		t.Fatalf("expected access_token with valid tfaCode, got (%d): %s", rec.Code, rec.Body.String())
	}

	// Recovery code should work once and fail on reuse
	recovery := confirmResp.RecoveryCodes[0]
	rec = doJSON(t, srv, http.MethodPost, "/api/login", "", map[string]interface{}{
		"username": "admin",
		"password": "AdminPass123!",
		"tfaCode":  recovery,
	})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"access_token"`) {
		t.Fatalf("expected access_token with recovery code, got (%d): %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, srv, http.MethodPost, "/api/login", "", map[string]interface{}{
		"username": "admin",
		"password": "AdminPass123!",
		"tfaCode":  recovery,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when reusing spent recovery code, got %d", rec.Code)
	}

	// 3. Admin reset-2fa restores direct login
	rec = doJSON(t, srv, http.MethodPost, "/admin/api/users/1/reset-2fa", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset-2fa failed (%d): %s", rec.Code, rec.Body.String())
	}

	// 4. SMTP configuration masking and email verification trusted-device flow
	rec = doJSON(t, srv, http.MethodPut, "/admin/api/smtp", token, map[string]interface{}{
		"enabled":             true,
		"host":                "smtp.example.com",
		"port":                587,
		"security":            "starttls",
		"username":            "noreply@example.com",
		"password":            "super-secret-smtp-pass",
		"from_address":        "noreply@example.com",
		"from_name":           "RustDesk Security",
		"email_login_policy":  "new_device",
		"code_expire_minutes": 10,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update smtp failed (%d): %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, srv, http.MethodGet, "/admin/api/smtp", token, nil)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "super-secret-smtp-pass") || !strings.Contains(rec.Body.String(), `"has_password":true`) {
		t.Fatalf("unexpected get smtp response (%d): %s", rec.Code, rec.Body.String())
	}

	db.Model(&model.User{}).Where("id = ?", 1).Update("email", "admin@example.com")
	var adminUser model.User
	db.First(&adminUser, 1)

	emailSvc := service.NewEmailService(db)
	if !emailSvc.NeedsEmailVerification(&adminUser, "brand-new-uuid") {
		t.Fatalf("expected new device to require email verification")
	}

	sum := sha256.Sum256([]byte("654321"))
	db.Create(&model.VerificationCode{
		UserID:     adminUser.ID,
		Purpose:    "email_login",
		CodeHash:   hex.EncodeToString(sum[:]),
		DeviceUUID: "brand-new-uuid",
		ExpiresAt:  time.Now().Add(5 * time.Minute),
	})

	rec = doJSON(t, srv, http.MethodPost, "/api/login", "", map[string]interface{}{
		"username":         "admin",
		"password":         "AdminPass123!",
		"id":               "300400500",
		"uuid":             "brand-new-uuid",
		"verificationCode": "654321",
	})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"access_token"`) {
		t.Fatalf("expected email verification login to succeed, got (%d): %s", rec.Code, rec.Body.String())
	}

	if emailSvc.NeedsEmailVerification(&adminUser, "brand-new-uuid") {
		t.Fatalf("expected verified device to be marked trusted under new_device policy")
	}
}

func TestClientStrategyHeartbeatPush(t *testing.T) {
	srv, _ := setupTestServer(t)

	rec := doJSON(t, srv, http.MethodPost, "/admin/api/login", "", map[string]interface{}{
		"username": "admin",
		"password": "AdminPass123!",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("admin login failed (%d): %s", rec.Code, rec.Body.String())
	}
	var adminLogin struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &adminLogin)
	token := adminLogin.AccessToken

	// 1. Default seeded strategy exposes 60 FPS
	rec = doJSON(t, srv, http.MethodGet, "/admin/api/client-strategy", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get client strategy failed (%d): %s", rec.Code, rec.Body.String())
	}
	var stratResp struct {
		Strategy struct {
			Enabled            bool   `json:"enabled"`
			CustomFPS          int    `json:"custom_fps"`
			StrategyModifiedAt int64  `json:"strategy_modified_at"`
		} `json:"strategy"`
		ConfigOptions map[string]string `json:"config_options"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &stratResp)
	if !stratResp.Strategy.Enabled || stratResp.Strategy.CustomFPS != 60 || stratResp.ConfigOptions["custom-fps"] != "60" {
		t.Fatalf("unexpected default client strategy: %+v", stratResp)
	}

	// 2. Reject invalid extra_options JSON
	rec = doJSON(t, srv, http.MethodPut, "/admin/api/client-strategy", token, map[string]interface{}{
		"enabled":       true,
		"custom_fps":    60,
		"extra_options": "{bad-json",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on invalid extra_options json, got %d", rec.Code)
	}

	// 3. Update strategy to 120 FPS and verify protected anchor keys are stripped from extra_options
	rec = doJSON(t, srv, http.MethodPut, "/admin/api/client-strategy", token, map[string]interface{}{
		"enabled":               true,
		"image_quality":         "custom",
		"custom_fps":            120,
		"custom_image_quality":  90,
		"codec_preference":      "vp9",
		"enable_hwcodec":        true,
		"enable_audio":          false,
		"enable_clipboard":      true,
		"enable_file_transfer":  false,
		"enable_keyboard":       true,
		"enable_tunnel":         true,
		"enable_remote_restart": true,
		"enable_record_session": false,
		"direct_server":         true,
		"extra_options":         `{"allow-remove-wallpaper":"Y","api-server":"https://evil.example"}`,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update client strategy failed (%d): %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &stratResp)
	if stratResp.ConfigOptions["custom-fps"] != "120" || stratResp.ConfigOptions["enable-audio"] != "N" || stratResp.ConfigOptions["direct-server"] != "Y" {
		t.Fatalf("unexpected updated config_options: %+v", stratResp.ConfigOptions)
	}
	if _, forbidden := stratResp.ConfigOptions["api-server"]; forbidden {
		t.Fatalf("protected key api-server must not be overridden by extra_options")
	}
	modUnix := stratResp.Strategy.StrategyModifiedAt

	// 4. Client heartbeat with stale modified_at receives pushed strategy
	rec = doJSON(t, srv, http.MethodPost, "/api/heartbeat", "", map[string]interface{}{
		"id":          "400500600",
		"uuid":        "uuid-strategy-test",
		"ver":         "1.3.0",
		"modified_at": 0,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat failed (%d): %s", rec.Code, rec.Body.String())
	}
	var hbPush struct {
		ModifiedAt int64 `json:"modified_at"`
		Strategy   *struct {
			ConfigOptions map[string]string `json:"config_options"`
		} `json:"strategy"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &hbPush)
	if hbPush.ModifiedAt != modUnix || hbPush.Strategy == nil || hbPush.Strategy.ConfigOptions["custom-fps"] != "120" {
		t.Fatalf("expected pushed strategy in heartbeat response, got: %s", rec.Body.String())
	}

	// 5. Client heartbeat with current modified_at skips redundant strategy payload
	rec = doJSON(t, srv, http.MethodPost, "/api/heartbeat", "", map[string]interface{}{
		"id":          "400500600",
		"uuid":        "uuid-strategy-test",
		"ver":         "1.3.0",
		"modified_at": modUnix,
	})
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"strategy"`) {
		t.Fatalf("expected synced heartbeat without strategy push, got (%d): %s", rec.Code, rec.Body.String())
	}
}

func TestPerformanceCachesAndCompression(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Pre-compressed embedded Web Admin static asset with ETag & 304 Not Modified
	req := httptest.NewRequest(http.MethodGet, "/_admin/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /_admin/app.js, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip Content-Encoding on /_admin/app.js, got %q", rec.Header().Get("Content-Encoding"))
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("expected ETag header on /_admin/app.js")
	}
	gzReader, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("open gzip reader for app.js: %v", err)
	}
	rawJS, err := io.ReadAll(gzReader)
	_ = gzReader.Close()
	if err != nil || !strings.Contains(string(rawJS), "renderStrategy") {
		t.Fatalf("unexpected gunzipped app.js content (%v)", err)
	}

	req304 := httptest.NewRequest(http.MethodGet, "/_admin/app.js", nil)
	req304.Header.Set("If-None-Match", etag)
	rec304 := httptest.NewRecorder()
	srv.ServeHTTP(rec304, req304)
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("expected 304 Not Modified with matching ETag, got %d", rec304.Code)
	}

	// 2. API gzip compression + Auth token cache hit & immediate revocation
	rec = doJSON(t, srv, http.MethodPost, "/api/login", "", map[string]interface{}{
		"username": "admin",
		"password": "AdminPass123!",
		"id":       "700800900",
		"uuid":     "uuid-perf-cache",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed (%d): %s", rec.Code, rec.Body.String())
	}
	var loginResp struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &loginResp)
	token := loginResp.AccessToken

	dashReq := httptest.NewRequest(http.MethodGet, "/admin/api/dashboard", nil)
	dashReq.Header.Set("Authorization", "Bearer "+token)
	dashReq.Header.Set("Accept-Encoding", "gzip")
	dashRec := httptest.NewRecorder()
	srv.ServeHTTP(dashRec, dashReq)
	if dashRec.Code != http.StatusOK {
		t.Fatalf("dashboard failed (%d): %s", dashRec.Code, dashRec.Body.String())
	}
	if dashRec.Header().Get("Content-Encoding") == "gzip" {
		zr, zErr := gzip.NewReader(bytes.NewReader(dashRec.Body.Bytes()))
		if zErr != nil {
			t.Fatalf("gunzip dashboard: %v", zErr)
		}
		plainDash, rErr := io.ReadAll(zr)
		_ = zr.Close()
		if rErr != nil || !strings.Contains(string(plainDash), `"server_config"`) {
			t.Fatalf("unexpected gunzipped dashboard payload (%v)", rErr)
		}
	}

	// Second call hits AuthService 15s token cache, then logout immediately invalidates it
	rec = doJSON(t, srv, http.MethodGet, "/api/currentUser", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected cached token auth 200, got %d", rec.Code)
	}
	rec = doJSON(t, srv, http.MethodPost, "/api/logout", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout failed (%d)", rec.Code)
	}
	rec = doJSON(t, srv, http.MethodGet, "/api/currentUser", token, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected immediate 401 after logout cache eviction, got %d", rec.Code)
	}
}
