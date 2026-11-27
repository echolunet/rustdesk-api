package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ClientStrategy stores centralized RustDesk client default configuration pushed via heartbeat.
type ClientStrategy struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	Enabled             bool      `gorm:"default:true" json:"enabled"`
	ImageQuality        string    `gorm:"size:32;default:custom" json:"image_quality"`
	CustomFPS           int       `gorm:"default:60" json:"custom_fps"`
	CustomImageQuality  int       `gorm:"default:75" json:"custom_image_quality"`
	CodecPreference     string    `gorm:"size:32;default:auto" json:"codec_preference"`
	EnableHWCodec       bool      `gorm:"default:true" json:"enable_hwcodec"`
	EnableAudio         bool      `gorm:"default:true" json:"enable_audio"`
	EnableClipboard     bool      `gorm:"default:true" json:"enable_clipboard"`
	EnableFileTransfer  bool      `gorm:"default:true" json:"enable_file_transfer"`
	EnableKeyboard      bool      `gorm:"default:true" json:"enable_keyboard"`
	EnableTunnel        bool      `gorm:"default:true" json:"enable_tunnel"`
	EnableRemoteRestart bool      `gorm:"default:true" json:"enable_remote_restart"`
	EnableRecordSession bool      `gorm:"default:false" json:"enable_record_session"`
	DirectServer        bool      `gorm:"default:false" json:"direct_server"`
	VerificationMethod  string    `gorm:"size:32;default:" json:"verification_method"`
	ApproveMode         string    `gorm:"size:32;default:" json:"approve_mode"`
	ExtraOptions        string    `gorm:"type:text" json:"extra_options"`
	StrategyModifiedAt  int64     `gorm:"not null" json:"strategy_modified_at"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

var protectedStrategyKeys = map[string]bool{
	"api-server":               true,
	"key":                      true,
	"custom-rendezvous-server": true,
	"relay-server":             true,
	"rendezvous-servers":       true,
}

// ClampInt constrains value into [minVal, maxVal], returning fallback when value is 0.
func ClampInt(val, minVal, maxVal, fallback int) int {
	if val == 0 {
		return fallback
	}
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

// Normalize clamps numeric ranges and normalizes enum fields on ClientStrategy.
func (s *ClientStrategy) Normalize() {
	switch strings.ToLower(strings.TrimSpace(s.ImageQuality)) {
	case "best", "balanced", "low", "custom":
		s.ImageQuality = strings.ToLower(strings.TrimSpace(s.ImageQuality))
	default:
		s.ImageQuality = "custom"
	}

	s.CustomFPS = ClampInt(s.CustomFPS, 5, 120, 60)
	s.CustomImageQuality = ClampInt(s.CustomImageQuality, 10, 100, 75)

	switch strings.ToLower(strings.TrimSpace(s.CodecPreference)) {
	case "auto", "vp9", "vp8", "h264", "h265", "av1":
		s.CodecPreference = strings.ToLower(strings.TrimSpace(s.CodecPreference))
	default:
		s.CodecPreference = "auto"
	}

	switch strings.ToLower(strings.TrimSpace(s.VerificationMethod)) {
	case "use-both-passwords", "use-temporary-password", "use-permanent-password":
		s.VerificationMethod = strings.ToLower(strings.TrimSpace(s.VerificationMethod))
	default:
		s.VerificationMethod = ""
	}

	switch strings.ToLower(strings.TrimSpace(s.ApproveMode)) {
	case "both", "password", "click":
		s.ApproveMode = strings.ToLower(strings.TrimSpace(s.ApproveMode))
	default:
		s.ApproveMode = ""
	}

	s.ExtraOptions = strings.TrimSpace(s.ExtraOptions)
}

// ParseExtraOptions validates ExtraOptions JSON and returns sanitized non-protected options.
func (s *ClientStrategy) ParseExtraOptions() (map[string]string, error) {
	result := make(map[string]string)
	rawText := strings.TrimSpace(s.ExtraOptions)
	if rawText == "" {
		return result, nil
	}

	var rawMap map[string]any
	if err := json.Unmarshal([]byte(rawText), &rawMap); err != nil {
		return nil, fmt.Errorf("extra_options must be a valid JSON object: %w", err)
	}

	for k, v := range rawMap {
		key := strings.TrimSpace(k)
		if key == "" || protectedStrategyKeys[strings.ToLower(key)] {
			continue
		}
		if v == nil {
			continue
		}
		switch val := v.(type) {
		case string:
			result[key] = val
		case bool:
			result[key] = strategyBool(val)
		case float64:
			result[key] = strconv.FormatFloat(val, 'f', -1, 64)
		default:
			encoded, err := json.Marshal(val)
			if err != nil {
				return nil, fmt.Errorf("invalid value for extra option %q", key)
			}
			result[key] = string(encoded)
		}
	}
	return result, nil
}

// ToConfigOptions builds the config_options map sent to RustDesk clients inside heartbeat responses.
func (s *ClientStrategy) ToConfigOptions() map[string]string {
	copyStrat := *s
	copyStrat.Normalize()

	opts := map[string]string{
		"image-quality":          copyStrat.ImageQuality,
		"custom-fps":             strconv.Itoa(copyStrat.CustomFPS),
		"custom-image-quality":   strconv.Itoa(copyStrat.CustomImageQuality),
		"enable-hwcodec":         strategyBool(copyStrat.EnableHWCodec),
		"enable-audio":           strategyBool(copyStrat.EnableAudio),
		"enable-clipboard":       strategyBool(copyStrat.EnableClipboard),
		"enable-file-transfer":   strategyBool(copyStrat.EnableFileTransfer),
		"enable-keyboard":        strategyBool(copyStrat.EnableKeyboard),
		"enable-tunnel":          strategyBool(copyStrat.EnableTunnel),
		"enable-remote-restart":  strategyBool(copyStrat.EnableRemoteRestart),
		"enable-record-session":  strategyBool(copyStrat.EnableRecordSession),
		"direct-server":          strategyBool(copyStrat.DirectServer),
	}

	if copyStrat.CodecPreference != "" && copyStrat.CodecPreference != "auto" {
		opts["codec-preference"] = copyStrat.CodecPreference
	}
	if copyStrat.VerificationMethod != "" {
		opts["verification-method"] = copyStrat.VerificationMethod
	}
	if copyStrat.ApproveMode != "" {
		opts["approve-mode"] = copyStrat.ApproveMode
	}

	if extra, err := copyStrat.ParseExtraOptions(); err == nil {
		for k, v := range extra {
			opts[k] = v
		}
	}
	return opts
}

func strategyBool(v bool) string {
	if v {
		return "Y"
	}
	return "N"
}
