package handler

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rustdesk-api/internal/config"
	"rustdesk-api/internal/middleware"
	"rustdesk-api/internal/model"
	"rustdesk-api/internal/service"
)

type peerTouchRecord struct {
	uuid      string
	ver       string
	ip        string
	lastWrite int64
}

// ClientPeerHandler serves heartbeat, sysinfo, peers, users, and accessible device group endpoints.
type ClientPeerHandler struct {
	db          *gorm.DB
	cfg         *config.Config
	strategySvc *service.StrategyService
	startTime   int64
	touchMu     sync.RWMutex
	touchCache  map[string]peerTouchRecord
}

// NewClientPeerHandler creates a new ClientPeerHandler.
func NewClientPeerHandler(db *gorm.DB, cfg *config.Config, strategySvc *service.StrategyService, startTime int64) *ClientPeerHandler {
	return &ClientPeerHandler{
		db:          db,
		cfg:         cfg,
		strategySvc: strategySvc,
		startTime:   startTime,
		touchCache:  make(map[string]peerTouchRecord, 256),
	}
}

type heartbeatRequest struct {
	ID         string           `json:"id"`
	UUID       string           `json:"uuid"`
	Ver        model.FlexString `json:"ver"`
	ModifiedAt int64            `json:"modified_at"`
}

// Heartbeat records client presence and returns the server's current strategy timestamp.
func (h *ClientPeerHandler) Heartbeat(c *gin.Context) {
	var req heartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid heartbeat payload"})
		return
	}

	peerID := strings.TrimSpace(req.ID)
	uuidVal := strings.TrimSpace(req.UUID)
	verStr := req.Ver.String()
	clientIP := c.ClientIP()
	now := time.Now()
	nowUnix := now.Unix()

	if peerID != "" {
		skipDB := false
		h.touchMu.RLock()
		rec, ok := h.touchCache[peerID]
		h.touchMu.RUnlock()
		if ok && nowUnix-rec.lastWrite < 30 && rec.uuid == uuidVal && rec.ver == verStr && rec.ip == clientIP {
			skipDB = true
		}

		if !skipDB {
			var peer model.Peer
			err := h.db.Where("peer_id = ?", peerID).First(&peer).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if createErr := h.db.Create(&model.Peer{
					PeerID:       peerID,
					UUID:         uuidVal,
					Version:      verStr,
					LastIP:       clientIP,
					LastOnlineAt: &now,
				}).Error; createErr == nil {
					h.recordPeerTouch(peerID, uuidVal, verStr, clientIP, nowUnix)
				}
			} else if err == nil {
				if peer.UUID == "" || uuidVal == "" || peer.UUID == uuidVal {
					updates := map[string]interface{}{
						"last_online_at": &now,
						"last_ip":        clientIP,
					}
					if peer.UUID == "" && uuidVal != "" {
						updates["uuid"] = uuidVal
					}
					if verStr != "" {
						updates["version"] = verStr
					}
					if h.db.Model(&peer).Updates(updates).Error == nil {
						effUUID := peer.UUID
						if effUUID == "" {
							effUUID = uuidVal
						}
						effVer := peer.Version
						if verStr != "" {
							effVer = verStr
						}
						h.recordPeerTouch(peerID, effUUID, effVer, clientIP, nowUnix)
					}
				}
			}
		}
	}

	if h.strategySvc != nil {
		if snap := h.strategySvc.GetSnapshot(); snap != nil && snap.Strategy.Enabled {
			if req.ModifiedAt == 0 || req.ModifiedAt < snap.ModifiedAt {
				c.JSON(http.StatusOK, gin.H{
					"modified_at": snap.ModifiedAt,
					"disconnect":  []string{},
					"strategy": gin.H{
						"config_options": snap.ConfigOptions,
					},
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"modified_at": snap.ModifiedAt,
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"modified_at": nowUnix,
	})
}

func (h *ClientPeerHandler) recordPeerTouch(peerID, uuidVal, verStr, ip string, nowUnix int64) {
	h.touchMu.Lock()
	if len(h.touchCache) >= 20000 {
		for k, v := range h.touchCache {
			if nowUnix-v.lastWrite > 60 {
				delete(h.touchCache, k)
			}
		}
		if len(h.touchCache) >= 20000 {
			h.touchCache = make(map[string]peerTouchRecord, 256)
		}
	}
	h.touchCache[peerID] = peerTouchRecord{
		uuid:      uuidVal,
		ver:       verStr,
		ip:        ip,
		lastWrite: nowUnix,
	}
	h.touchMu.Unlock()
}

type sysinfoRequest struct {
	ID       string           `json:"id"`
	UUID     string           `json:"uuid"`
	Hostname string           `json:"hostname"`
	Username string           `json:"username"`
	OS       string           `json:"os"`
	CPU      string           `json:"cpu"`
	Memory   model.FlexString `json:"memory"`
	Version  model.FlexString `json:"version"`
}

// Sysinfo records hardware and OS inventory while protecting against PeerID spoofing via UUID checks.
func (h *ClientPeerHandler) Sysinfo(c *gin.Context) {
	var req sysinfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sysinfo payload"})
		return
	}

	peerID := strings.TrimSpace(req.ID)
	uuidVal := strings.TrimSpace(req.UUID)
	if peerID == "" || uuidVal == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id and uuid are required"})
		return
	}

	now := time.Now()
	var peer model.Peer
	err := h.db.Where("peer_id = ?", peerID).First(&peer).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		peer = model.Peer{
			PeerID:       peerID,
			UUID:         uuidVal,
			Hostname:     req.Hostname,
			Username:     req.Username,
			OS:           req.OS,
			CPU:          req.CPU,
			Memory:       req.Memory.String(),
			Version:      req.Version.String(),
			LastIP:       c.ClientIP(),
			LastOnlineAt: &now,
		}
		if createErr := h.db.Create(&peer).Error; createErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save sysinfo"})
			return
		}
		h.recordPeerTouch(peerID, uuidVal, peer.Version, c.ClientIP(), now.Unix())
		c.String(http.StatusOK, "SYSINFO_UPDATED")
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if peer.UUID != "" && peer.UUID != uuidVal {
		c.JSON(http.StatusForbidden, gin.H{"error": "device uuid mismatch"})
		return
	}

	updates := map[string]interface{}{
		"uuid":           uuidVal,
		"hostname":       req.Hostname,
		"username":       req.Username,
		"os":             req.OS,
		"cpu":            req.CPU,
		"memory":         req.Memory.String(),
		"last_ip":        c.ClientIP(),
		"last_online_at": &now,
	}
	if req.Version.String() != "" {
		updates["version"] = req.Version.String()
	}
	if h.db.Model(&peer).Updates(updates).Error == nil {
		verVal := peer.Version
		if req.Version.String() != "" {
			verVal = req.Version.String()
		}
		h.recordPeerTouch(peerID, uuidVal, verVal, c.ClientIP(), now.Unix())
	}
	c.String(http.StatusOK, "SYSINFO_UPDATED")
}

// SysinfoVer returns the API version and process start time.
func (h *ClientPeerHandler) SysinfoVer(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":    h.cfg.AppVersion,
		"start_time": h.startTime,
	})
}

// ListPeers returns devices accessible to the authenticated user.
func (h *ClientPeerHandler) ListPeers(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	current, pageSize := ParsePagination(c, 50)
	q := h.db.Model(&model.Peer{})

	if !user.IsAdmin && !h.cfg.AllowAllDevicesForUsers {
		groupIDs, err := AccessibleDeviceGroupIDs(h.db, user.ID, user.GroupID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query device access failed"})
			return
		}
		if len(groupIDs) > 0 {
			q = q.Where("user_id = ? OR device_group_id IN ?", user.ID, groupIDs)
		} else {
			q = q.Where("user_id = ?", user.ID)
		}
	}

	if v := trimWildcard(c.Query("id")); v != "" {
		q = q.Where("peer_id LIKE ?", "%"+v+"%")
	}
	if v := trimWildcard(c.Query("device_name")); v != "" {
		q = q.Where("hostname LIKE ? OR alias LIKE ?", "%"+v+"%", "%"+v+"%")
	}
	if v := trimWildcard(c.Query("user_name")); v != "" {
		q = q.Where("user_name LIKE ?", "%"+v+"%")
	}
	if v := trimWildcard(c.Query("device_group_name")); v != "" {
		q = q.Where("device_group_name LIKE ?", "%"+v+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count peers failed"})
		return
	}

	var peers []model.Peer
	if err := q.Order("last_online_at desc, id desc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&peers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fetch peers failed"})
		return
	}

	cutoff := time.Now().Add(-3 * time.Minute)
	data := make([]map[string]interface{}, 0, len(peers))
	for _, p := range peers {
		online := 0
		lastOnline := ""
		if p.LastOnlineAt != nil {
			lastOnline = p.LastOnlineAt.UTC().Format(time.RFC3339)
			if p.LastOnlineAt.After(cutoff) {
				online = 1
			}
		}
		data = append(data, map[string]interface{}{
			"id":                p.PeerID,
			"user_name":         p.UserName,
			"device_group_name": p.DeviceGroupName,
			"alias":             p.Alias,
			"note":              p.Note,
			"status":            online,
			"last_online":       lastOnline,
			"info": map[string]interface{}{
				"device_name": p.Hostname,
				"hostname":    p.Hostname,
				"username":    p.Username,
				"os":          p.OS,
				"cpu":         p.CPU,
				"memory":      p.Memory,
				"version":     p.Version,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"data":  data,
	})
}

// ListUsers returns users visible to the authenticated client session.
func (h *ClientPeerHandler) ListUsers(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	current, pageSize := ParsePagination(c, 50)
	q := h.db.Model(&model.User{}).Where("status = ?", 1)
	if !user.IsAdmin {
		if user.GroupID > 0 {
			q = q.Where("id = ? OR group_id = ?", user.ID, user.GroupID)
		} else {
			q = q.Where("id = ?", user.ID)
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count users failed"})
		return
	}

	var users []model.User
	if err := q.Order("id asc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fetch users failed"})
		return
	}

	data := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		grp := u.GroupName
		if grp == "" {
			grp = "default"
		}
		data = append(data, map[string]interface{}{
			"name":     u.Username,
			"email":    u.Email,
			"note":     u.Note,
			"status":   u.Status,
			"grp":      grp,
			"is_admin": u.IsAdmin,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"data":  data,
	})
}

// AccessibleDeviceGroups returns device groups accessible to the authenticated user.
func (h *ClientPeerHandler) AccessibleDeviceGroups(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	current, pageSize := ParsePagination(c, 50)
	q := h.db.Model(&model.DeviceGroup{})
	if !user.IsAdmin {
		ids, err := AccessibleDeviceGroupIDs(h.db, user.ID, user.GroupID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query device groups failed"})
			return
		}
		if len(ids) == 0 {
			c.JSON(http.StatusOK, gin.H{"total": 0, "data": []interface{}{}})
			return
		}
		q = q.Where("id IN ?", ids)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count device groups failed"})
		return
	}

	var groups []model.DeviceGroup
	if err := q.Order("id asc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&groups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fetch device groups failed"})
		return
	}

	data := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		data = append(data, map[string]interface{}{
			"id":                g.ID,
			"name":              g.Name,
			"device_group_name": g.Name,
			"note":              g.Note,
			"created_at":        g.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"data":  data,
	})
}

func trimWildcard(s string) string {
	return strings.Trim(strings.TrimSpace(s), "%")
}
