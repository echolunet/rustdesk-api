package service

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"rustdesk-api/internal/model"
)

// ErrInvalidStrategyOptions indicates malformed extra_options JSON in ClientStrategy updates.
var ErrInvalidStrategyOptions = errors.New("invalid extra_options JSON")

// StrategySnapshot holds an immutable view of the active client strategy and prebuilt config_options map.
type StrategySnapshot struct {
	Strategy      model.ClientStrategy
	ConfigOptions map[string]string
	ModifiedAt    int64
}

// StrategyService caches the singleton ClientStrategy in memory for lock-free heartbeat reads.
type StrategyService struct {
	db    *gorm.DB
	mu    sync.Mutex
	cache atomic.Pointer[StrategySnapshot]
}

// NewStrategyService creates a new StrategyService and preloads the strategy snapshot.
func NewStrategyService(db *gorm.DB) *StrategyService {
	s := &StrategyService{db: db}
	_, _ = s.Reload()
	return s
}

func defaultStrategyModel() model.ClientStrategy {
	strat := model.ClientStrategy{
		Enabled:             true,
		ImageQuality:        "custom",
		CustomFPS:           60,
		CustomImageQuality:  75,
		CodecPreference:     "auto",
		EnableHWCodec:       true,
		EnableAudio:         true,
		EnableClipboard:     true,
		EnableFileTransfer:  true,
		EnableKeyboard:      true,
		EnableTunnel:        true,
		EnableRemoteRestart: true,
		StrategyModifiedAt:  time.Now().Unix(),
	}
	strat.Normalize()
	return strat
}

func buildStrategySnapshot(strat model.ClientStrategy) *StrategySnapshot {
	mod := strat.StrategyModifiedAt
	if mod <= 0 {
		mod = time.Now().Unix()
	}
	return &StrategySnapshot{
		Strategy:      strat,
		ConfigOptions: strat.ToConfigOptions(),
		ModifiedAt:    mod,
	}
}

// Reload queries the singleton ClientStrategy row from the database and refreshes the atomic cache.
func (s *StrategyService) Reload() (*StrategySnapshot, error) {
	var strat model.ClientStrategy
	err := s.db.Order("id asc").First(&strat).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		snap := buildStrategySnapshot(defaultStrategyModel())
		s.cache.Store(snap)
		return snap, nil
	}
	if err != nil {
		return nil, err
	}
	strat.Normalize()
	snap := buildStrategySnapshot(strat)
	s.cache.Store(snap)
	return snap, nil
}

// GetSnapshot returns the cached StrategySnapshot without DB reads on the heartbeat hot path.
func (s *StrategyService) GetSnapshot() *StrategySnapshot {
	if snap := s.cache.Load(); snap != nil {
		return snap
	}
	snap, _ := s.Reload()
	return snap
}

// Update validates, persists, monotonically bumps StrategyModifiedAt, and atomically updates the cached snapshot.
func (s *StrategyService) Update(candidate model.ClientStrategy) (*StrategySnapshot, error) {
	candidate.Normalize()
	if _, err := candidate.ParseExtraOptions(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidStrategyOptions, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	nowUnix := time.Now().Unix()
	var existing model.ClientStrategy
	err := s.db.Order("id asc").First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		candidate.StrategyModifiedAt = nowUnix
		if err := s.db.Create(&candidate).Error; err != nil {
			return nil, err
		}
		snap := buildStrategySnapshot(candidate)
		s.cache.Store(snap)
		return snap, nil
	}
	if err != nil {
		return nil, err
	}

	if nowUnix <= existing.StrategyModifiedAt {
		nowUnix = existing.StrategyModifiedAt + 1
	}

	existing.Enabled = candidate.Enabled
	existing.ImageQuality = candidate.ImageQuality
	existing.CustomFPS = candidate.CustomFPS
	existing.CustomImageQuality = candidate.CustomImageQuality
	existing.CodecPreference = candidate.CodecPreference
	existing.EnableHWCodec = candidate.EnableHWCodec
	existing.EnableAudio = candidate.EnableAudio
	existing.EnableClipboard = candidate.EnableClipboard
	existing.EnableFileTransfer = candidate.EnableFileTransfer
	existing.EnableKeyboard = candidate.EnableKeyboard
	existing.EnableTunnel = candidate.EnableTunnel
	existing.EnableRemoteRestart = candidate.EnableRemoteRestart
	existing.EnableRecordSession = candidate.EnableRecordSession
	existing.DirectServer = candidate.DirectServer
	existing.VerificationMethod = candidate.VerificationMethod
	existing.ApproveMode = candidate.ApproveMode
	existing.ExtraOptions = candidate.ExtraOptions
	existing.StrategyModifiedAt = nowUnix

	if err := s.db.Save(&existing).Error; err != nil {
		return nil, err
	}
	snap := buildStrategySnapshot(existing)
	s.cache.Store(snap)
	return snap, nil
}
