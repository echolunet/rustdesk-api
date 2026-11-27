package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds all runtime settings loaded from environment variables.
type Config struct {
	AppHost      string
	AppPort      int
	AppMode      string
	AppVersion   string
	DataDir      string
	WebClientDir string

	DBType string
	DBDSN  string

	JWTSecret      string
	JWTExpireHours int

	AdminInitUser     string
	AdminInitPassword string

	RustDeskIDServer    string
	RustDeskRelayServer string
	RustDeskAPIServer   string
	RustDeskKeyFile     string
	RustDeskKey         string

	HBBSWSTarget string
	HBBRWSTarget string

	AllowAllDevicesForUsers bool
	MaxPeerOneAB            int

	ClientStrategyEnabled     bool
	ClientDefaultImageQuality string
	ClientDefaultFPS          int
	ClientDefaultQuality      int
	ClientDefaultCodec        string
	ClientEnableHWCodec       bool
	ClientEnableAudio         bool
	ClientEnableClipboard     bool
	ClientEnableFileTransfer  bool
	ClientEnableKeyboard      bool
	ClientEnableTunnel        bool
	ClientEnableRemoteRestart bool
	ClientEnableRecordSession bool
	ClientDirectServer        bool
	ClientVerificationMethod  string
	ClientApproveMode         string
}

// Load reads configuration from environment variables and ensures required directories and JWT secret exist.
func Load() (*Config, error) {
	dataDir := getEnv("DATA_DIR", "/app/data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	webClientDir := getEnv("WEBCLIENT_DIR", "/app/webclient")

	dbType := strings.ToLower(strings.TrimSpace(getEnv("DB_TYPE", "sqlite")))
	dbDSN := strings.TrimSpace(os.Getenv("DB_DSN"))
	if dbDSN == "" && dbType == "sqlite" {
		dbDSN = filepath.Join(dataDir, "rustdesk-api.db")
	}

	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if jwtSecret == "" {
		var err error
		jwtSecret, err = loadOrCreateSecret(filepath.Join(dataDir, ".jwt_secret"))
		if err != nil {
			return nil, fmt.Errorf("load jwt secret: %w", err)
		}
	}

	cfg := &Config{
		AppHost:                 getEnv("APP_HOST", "0.0.0.0"),
		AppPort:                 getEnvInt("APP_PORT", 21114),
		AppMode:                 getEnv("GIN_MODE", "release"),
		AppVersion:              getEnv("APP_VERSION", "1.0.0"),
		DataDir:                 dataDir,
		WebClientDir:            webClientDir,
		DBType:                  dbType,
		DBDSN:                   dbDSN,
		JWTSecret:               jwtSecret,
		JWTExpireHours:          getEnvInt("JWT_EXPIRE_HOURS", 168),
		AdminInitUser:           getEnv("ADMIN_INIT_USER", "admin"),
		AdminInitPassword:       strings.TrimSpace(os.Getenv("ADMIN_INIT_PASSWORD")),
		RustDeskIDServer:        strings.TrimSpace(os.Getenv("RUSTDESK_ID_SERVER")),
		RustDeskRelayServer:     strings.TrimSpace(os.Getenv("RUSTDESK_RELAY_SERVER")),
		RustDeskAPIServer:       strings.TrimSpace(os.Getenv("RUSTDESK_API_SERVER")),
		RustDeskKeyFile:         getEnv("RUSTDESK_KEY_FILE", "/data/server/id_ed25519.pub"),
		RustDeskKey:             strings.TrimSpace(os.Getenv("RUSTDESK_KEY")),
		HBBSWSTarget:            getEnv("HBBS_WS_TARGET", "http://hbbs:21118"),
		HBBRWSTarget:            getEnv("HBBR_WS_TARGET", "http://hbbr:21119"),
		AllowAllDevicesForUsers: getEnvBool("ALLOW_ALL_DEVICES_FOR_USERS", false),
		MaxPeerOneAB:            getEnvInt("MAX_PEER_ONE_AB", 0),

		ClientStrategyEnabled:     getEnvBool("CLIENT_STRATEGY_ENABLED", true),
		ClientDefaultImageQuality: getEnv("CLIENT_DEFAULT_IMAGE_QUALITY", "custom"),
		ClientDefaultFPS:          clampInt(getEnvInt("CLIENT_DEFAULT_FPS", 60), 5, 120),
		ClientDefaultQuality:      clampInt(getEnvInt("CLIENT_DEFAULT_QUALITY", 75), 10, 100),
		ClientDefaultCodec:        getEnv("CLIENT_DEFAULT_CODEC", "auto"),
		ClientEnableHWCodec:       getEnvBool("CLIENT_ENABLE_HWCODEC", true),
		ClientEnableAudio:         getEnvBool("CLIENT_ENABLE_AUDIO", true),
		ClientEnableClipboard:     getEnvBool("CLIENT_ENABLE_CLIPBOARD", true),
		ClientEnableFileTransfer:  getEnvBool("CLIENT_ENABLE_FILE_TRANSFER", true),
		ClientEnableKeyboard:      getEnvBool("CLIENT_ENABLE_KEYBOARD", true),
		ClientEnableTunnel:        getEnvBool("CLIENT_ENABLE_TUNNEL", true),
		ClientEnableRemoteRestart: getEnvBool("CLIENT_ENABLE_REMOTE_RESTART", true),
		ClientEnableRecordSession: getEnvBool("CLIENT_ENABLE_RECORD_SESSION", false),
		ClientDirectServer:        getEnvBool("CLIENT_DIRECT_SERVER", false),
		ClientVerificationMethod:  strings.TrimSpace(os.Getenv("CLIENT_VERIFICATION_METHOD")),
		ClientApproveMode:         strings.TrimSpace(os.Getenv("CLIENT_APPROVE_MODE")),
	}
	return cfg, nil
}

// ListenAddr returns host:port string for the HTTP server.
func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.AppHost, c.AppPort)
}

func loadOrCreateSecret(path string) (string, error) {
	if raw, err := os.ReadFile(path); err == nil {
		s := strings.TrimSpace(string(raw))
		if len(s) >= 32 {
			return s, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "y"
}

func clampInt(val, minVal, maxVal int) int {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}
