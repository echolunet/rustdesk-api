package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ReleaseAsset describes a downloadable official RustDesk artifact from GitHub Releases.
type ReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// ReleaseInfo describes the latest release metadata for an official RustDesk repository.
type ReleaseInfo struct {
	Repo        string         `json:"repo"`
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	PublishedAt string         `json:"published_at"`
	HTMLURL     string         `json:"html_url"`
	Assets      []ReleaseAsset `json:"assets"`
	Error       string         `json:"error,omitempty"`
}

// OfficialUpdatesPayload aggregates official client and server release details with Docker update commands.
type OfficialUpdatesPayload struct {
	CheckedAt     time.Time   `json:"checked_at"`
	Cached        bool        `json:"cached"`
	ClientRelease ReleaseInfo `json:"client_release"`
	ServerRelease ReleaseInfo `json:"server_release"`
	DockerImage   string      `json:"docker_image"`
	UpdateCmd     string      `json:"update_command"`
}

// UpdateService checks official GitHub releases for rustdesk/rustdesk and rustdesk/rustdesk-server.
type UpdateService struct {
	httpCli   *http.Client
	mu        sync.RWMutex
	cached    *OfficialUpdatesPayload
	cachedAt  time.Time
	cacheTTL  time.Duration
}

// NewUpdateService creates a new UpdateService with a 30-minute in-memory release cache.
func NewUpdateService() *UpdateService {
	return &UpdateService{
		httpCli:  &http.Client{Timeout: 8 * time.Second},
		cacheTTL: 30 * time.Minute,
	}
}

// CheckOfficialUpdates retrieves the latest official RustDesk client and server release details.
func (s *UpdateService) CheckOfficialUpdates(forceRefresh bool) OfficialUpdatesPayload {
	if !forceRefresh {
		s.mu.RLock()
		if s.cached != nil && time.Since(s.cachedAt) < s.cacheTTL {
			out := *s.cached
			out.Cached = true
			s.mu.RUnlock()
			return out
		}
		s.mu.RUnlock()
	}

	clientRel := s.fetchLatestRelease("rustdesk/rustdesk", 12)
	serverRel := s.fetchLatestRelease("rustdesk/rustdesk-server", 10)

	payload := OfficialUpdatesPayload{
		CheckedAt:     time.Now(),
		Cached:        false,
		ClientRelease: clientRel,
		ServerRelease: serverRel,
		DockerImage:   "rustdesk/rustdesk-server:latest",
		UpdateCmd:     "./scripts/update.sh",
	}

	s.mu.Lock()
	s.cached = &payload
	s.cachedAt = time.Now()
	s.mu.Unlock()

	return payload
}

func (s *UpdateService) fetchLatestRelease(repo string, maxAssets int) ReleaseInfo {
	info := ReleaseInfo{
		Repo:    repo,
		HTMLURL: fmt.Sprintf("https://github.com/%s/releases/latest", repo),
		Assets:  []ReleaseAsset{},
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "rustdesk-api-selfhosted")

	resp, err := s.httpCli.Do(req)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		info.Error = fmt.Sprintf("GitHub API returned HTTP %d", resp.StatusCode)
		return info
	}

	var gh struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		PublishedAt string `json:"published_at"`
		HTMLURL     string `json:"html_url"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		info.Error = err.Error()
		return info
	}

	info.TagName = gh.TagName
	info.Name = gh.Name
	info.PublishedAt = gh.PublishedAt
	if gh.HTMLURL != "" {
		info.HTMLURL = gh.HTMLURL
	}

	for _, a := range gh.Assets {
		lower := strings.ToLower(a.Name)
		if strings.HasSuffix(lower, ".sha256") || strings.HasSuffix(lower, ".sig") {
			continue
		}
		info.Assets = append(info.Assets, ReleaseAsset{
			Name: a.Name,
			URL:  a.BrowserDownloadURL,
			Size: a.Size,
		})
		if len(info.Assets) >= maxAssets {
			break
		}
	}
	return info
}
