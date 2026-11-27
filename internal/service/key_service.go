package service

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"

	"rustdesk-api/internal/config"
)

// KeyService reads the official hbbs Ed25519 public key and generates one-click RustDesk client configurations.
type KeyService struct {
	cfg              *config.Config
	mu               sync.RWMutex
	cachedKey        string
	cachedKeyModTime time.Time
	cachedKeySize    int64
	qrCacheKey       string
	qrConfigString   string
	qrDataURI        string
}

// NewKeyService creates a new KeyService.
func NewKeyService(cfg *config.Config) *KeyService {
	return &KeyService{cfg: cfg}
}

// ClientConfigInfo bundles official server connection settings and the encoded RustDesk client import string.
type ClientConfigInfo struct {
	IDServer        string `json:"id_server"`
	RelayServer     string `json:"relay_server"`
	APIServer       string `json:"api_server"`
	PublicKey       string `json:"public_key"`
	KeySource       string `json:"key_source"`
	KeyExists       bool   `json:"key_exists"`
	KeyFilePath     string `json:"key_file_path"`
	ConfigString   string `json:"config_string"`
	CLICommand     string `json:"cli_command"`
	QRDataURI      string `json:"qr_data_uri"`
	WebClientReady bool   `json:"webclient_ready"`
	AppVersion     string `json:"app_version"`
}

// GetPublicKey retrieves the server public key from RUSTDESK_KEY or the mounted official hbbs id_ed25519.pub file.
func (s *KeyService) GetPublicKey() (string, string, bool) {
	if s.cfg.RustDeskKey != "" {
		return s.cfg.RustDeskKey, "env", true
	}
	info, err := os.Stat(s.cfg.RustDeskKeyFile)
	if err != nil {
		return "", "missing", false
	}

	s.mu.RLock()
	if s.cachedKey != "" && info.Size() == s.cachedKeySize && info.ModTime().Equal(s.cachedKeyModTime) {
		k := s.cachedKey
		s.mu.RUnlock()
		return k, "file", true
	}
	s.mu.RUnlock()

	raw, err := os.ReadFile(s.cfg.RustDeskKeyFile)
	if err != nil {
		return "", "missing", false
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", "missing", false
	}

	s.mu.Lock()
	s.cachedKey = trimmed
	s.cachedKeySize = info.Size()
	s.cachedKeyModTime = info.ModTime()
	s.mu.Unlock()
	return trimmed, "file", true
}

// GetClientConfig builds the connection parameters and reversed-base64 import string expected by `rustdesk --config`.
func (s *KeyService) GetClientConfig(requestBaseURL string) ClientConfigInfo {
	key, source, exists := s.GetPublicKey()

	apiURL := s.cfg.RustDeskAPIServer
	if apiURL == "" {
		apiURL = strings.TrimRight(requestBaseURL, "/")
	}

	idServer := s.cfg.RustDeskIDServer
	relayServer := s.cfg.RustDeskRelayServer
	if idServer == "" && apiURL != "" {
		idServer = extractHost(apiURL)
	}
	if relayServer == "" && idServer != "" {
		hostOnly := strings.Split(idServer, ":")[0]
		relayServer = hostOnly + ":21117"
	}

	cacheKey := idServer + "\x00" + relayServer + "\x00" + apiURL + "\x00" + key
	s.mu.RLock()
	if s.qrCacheKey == cacheKey && s.qrConfigString != "" {
		configString := s.qrConfigString
		qrDataURI := s.qrDataURI
		s.mu.RUnlock()
		cliCmd := "rustdesk --config \"" + configString + "\""
		_, wcErr := os.Stat(filepath.Join(s.cfg.WebClientDir, "index.html"))
		return ClientConfigInfo{
			IDServer:       idServer,
			RelayServer:    relayServer,
			APIServer:      apiURL,
			PublicKey:      key,
			KeySource:      source,
			KeyExists:      exists,
			KeyFilePath:    s.cfg.RustDeskKeyFile,
			ConfigString:   configString,
			CLICommand:     cliCmd,
			QRDataURI:      qrDataURI,
			WebClientReady: wcErr == nil,
			AppVersion:     s.cfg.AppVersion,
		}
	}
	s.mu.RUnlock()

	payload := map[string]string{
		"host":  idServer,
		"relay": relayServer,
		"api":   apiURL,
		"key":   key,
	}
	rawJSON, _ := json.Marshal(payload)
	encoded := base64.RawStdEncoding.EncodeToString(rawJSON)
	configString := reverseASCII(encoded)

	cliCmd := ""
	qrDataURI := ""
	if configString != "" {
		cliCmd = "rustdesk --config \"" + configString + "\""
		if png, err := qrcode.Encode("rustdesk://config/"+configString, qrcode.Medium, 220); err == nil {
			qrDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		}
		s.mu.Lock()
		s.qrCacheKey = cacheKey
		s.qrConfigString = configString
		s.qrDataURI = qrDataURI
		s.mu.Unlock()
	}

	_, wcErr := os.Stat(filepath.Join(s.cfg.WebClientDir, "index.html"))

	return ClientConfigInfo{
		IDServer:       idServer,
		RelayServer:    relayServer,
		APIServer:      apiURL,
		PublicKey:      key,
		KeySource:      source,
		KeyExists:      exists,
		KeyFilePath:    s.cfg.RustDeskKeyFile,
		ConfigString:   configString,
		CLICommand:     cliCmd,
		QRDataURI:      qrDataURI,
		WebClientReady: wcErr == nil,
		AppVersion:     s.cfg.AppVersion,
	}
}

func reverseASCII(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

func extractHost(rawURL string) string {
	s := strings.TrimPrefix(rawURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.Split(s, "/")[0]
	host := strings.Split(s, ":")[0]
	return host
}
