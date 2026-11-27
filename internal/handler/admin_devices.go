package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rustdesk-api/internal/model"
)

// AdminDeviceHandler manages registered RustDesk devices (peers) and device groups with ACLs.
type AdminDeviceHandler struct {
	db *gorm.DB
}

// NewAdminDeviceHandler creates a new AdminDeviceHandler.
func NewAdminDeviceHandler(db *gorm.DB) *AdminDeviceHandler {
	return &AdminDeviceHandler{db: db}
}

// ListDevices returns paginated devices with online calculation and search filtering.
func (h *AdminDeviceHandler) ListDevices(c *gin.Context) {
	current, pageSize := ParsePagination(c, 20)
	q := h.db.Model(&model.Peer{})
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("peer_id LIKE ? OR hostname LIKE ? OR alias LIKE ? OR user_name LIKE ? OR os LIKE ?", like, like, like, like, like)
	}
	if dgID := strings.TrimSpace(c.Query("device_group_id")); dgID != "" {
		q = q.Where("device_group_id = ?", dgID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count devices failed"})
		return
	}

	var peers []model.Peer
	if err := q.Order("last_online_at desc, id desc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&peers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query devices failed"})
		return
	}

	cutoff := time.Now().Add(-3 * time.Minute)
	out := make([]map[string]interface{}, 0, len(peers))
	for _, p := range peers {
		online := p.LastOnlineAt != nil && p.LastOnlineAt.After(cutoff)
		out = append(out, map[string]interface{}{
			"id":                p.ID,
			"peer_id":           p.PeerID,
			"uuid":              p.UUID,
			"user_id":           p.UserID,
			"user_name":         p.UserName,
			"device_group_id":   p.DeviceGroupID,
			"device_group_name": p.DeviceGroupName,
			"hostname":          p.Hostname,
			"username":          p.Username,
			"os":                p.OS,
			"cpu":               p.CPU,
			"memory":            p.Memory,
			"version":           p.Version,
			"alias":             p.Alias,
			"note":              p.Note,
			"last_ip":           p.LastIP,
			"last_online_at":    p.LastOnlineAt,
			"online":            online,
		})
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "data": out})
}

// UpdateDevice updates owner, device group, alias, and note for a registered device.
func (h *AdminDeviceHandler) UpdateDevice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device id"})
		return
	}
	var peer model.Peer
	if err := h.db.First(&peer, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	var req struct {
		UserID        *uint  `json:"user_id"`
		DeviceGroupID *uint  `json:"device_group_id"`
		Alias         string `json:"alias"`
		Note          string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	updates := map[string]interface{}{
		"alias": req.Alias,
		"note":  req.Note,
	}
	if req.UserID != nil {
		if *req.UserID == 0 {
			updates["user_id"] = 0
			updates["user_name"] = ""
		} else {
			var u model.User
			if err := h.db.First(&u, *req.UserID).Error; err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "user not found"})
				return
			}
			updates["user_id"] = u.ID
			updates["user_name"] = u.Username
		}
	}
	if req.DeviceGroupID != nil {
		if *req.DeviceGroupID == 0 {
			updates["device_group_id"] = 0
			updates["device_group_name"] = ""
		} else {
			var dg model.DeviceGroup
			if err := h.db.First(&dg, *req.DeviceGroupID).Error; err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "device group not found"})
				return
			}
			updates["device_group_id"] = dg.ID
			updates["device_group_name"] = dg.Name
		}
	}

	if err := h.db.Model(&peer).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update device failed"})
		return
	}
	_ = h.db.First(&peer, peer.ID).Error
	c.JSON(http.StatusOK, peer)
}

// DeleteDevice removes a device record from the inventory.
func (h *AdminDeviceHandler) DeleteDevice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device id"})
		return
	}
	h.db.Delete(&model.Peer{}, id)
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ListDeviceGroups returns all device groups along with their granted user group and user IDs.
func (h *AdminDeviceHandler) ListDeviceGroups(c *gin.Context) {
	var groups []model.DeviceGroup
	if err := h.db.Order("id asc").Find(&groups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query device groups failed"})
		return
	}

	var accesses []model.DeviceGroupAccess
	h.db.Find(&accesses)

	ugMap := make(map[uint][]uint)
	uMap := make(map[uint][]uint)
	for _, a := range accesses {
		if a.UserGroupID > 0 {
			ugMap[a.DeviceGroupID] = append(ugMap[a.DeviceGroupID], a.UserGroupID)
		}
		if a.UserID > 0 {
			uMap[a.DeviceGroupID] = append(uMap[a.DeviceGroupID], a.UserID)
		}
	}

	out := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		ugIDs := ugMap[g.ID]
		if ugIDs == nil {
			ugIDs = []uint{}
		}
		uIDs := uMap[g.ID]
		if uIDs == nil {
			uIDs = []uint{}
		}
		out = append(out, map[string]interface{}{
			"id":              g.ID,
			"name":            g.Name,
			"note":            g.Note,
			"user_group_ids":  ugIDs,
			"user_ids":        uIDs,
			"created_at":      g.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

type deviceGroupReq struct {
	Name         string `json:"name"`
	Note         string `json:"note"`
	UserGroupIDs []uint `json:"user_group_ids"`
	UserIDs      []uint `json:"user_ids"`
}

// CreateDeviceGroup creates a new device group and configures access control assignments.
func (h *AdminDeviceHandler) CreateDeviceGroup(c *gin.Context) {
	var req deviceGroupReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device group name is required"})
		return
	}

	dg := model.DeviceGroup{Name: strings.TrimSpace(req.Name), Note: req.Note}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&dg).Error; err != nil {
			return err
		}
		return syncDeviceGroupAccess(tx, dg.ID, req.UserGroupIDs, req.UserIDs)
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "create device group failed (name may exist)"})
		return
	}
	c.JSON(http.StatusCreated, dg)
}

// UpdateDeviceGroup updates a device group, syncs its name on Peer rows, and updates ACL assignments.
func (h *AdminDeviceHandler) UpdateDeviceGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device group id"})
		return
	}
	var dg model.DeviceGroup
	if err := h.db.First(&dg, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device group not found"})
		return
	}

	var req deviceGroupReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device group name is required"})
		return
	}
	name := strings.TrimSpace(req.Name)

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&dg).Updates(map[string]interface{}{"name": name, "note": req.Note}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Peer{}).Where("device_group_id = ?", dg.ID).Update("device_group_name", name).Error; err != nil {
			return err
		}
		return syncDeviceGroupAccess(tx, dg.ID, req.UserGroupIDs, req.UserIDs)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update device group failed"})
		return
	}
	c.JSON(http.StatusOK, dg)
}

// DeleteDeviceGroup deletes a device group and unassigns member peers.
func (h *AdminDeviceHandler) DeleteDeviceGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device group id"})
		return
	}
	_ = h.db.Transaction(func(tx *gorm.DB) error {
		tx.Model(&model.Peer{}).Where("device_group_id = ?", id).Updates(map[string]interface{}{
			"device_group_id":   0,
			"device_group_name": "",
		})
		tx.Where("device_group_id = ?", id).Delete(&model.DeviceGroupAccess{})
		return tx.Delete(&model.DeviceGroup{}, id).Error
	})
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func syncDeviceGroupAccess(tx *gorm.DB, dgID uint, userGroupIDs, userIDs []uint) error {
	if err := tx.Where("device_group_id = ?", dgID).Delete(&model.DeviceGroupAccess{}).Error; err != nil {
		return err
	}
	rows := make([]model.DeviceGroupAccess, 0, len(userGroupIDs)+len(userIDs))
	seenUG := make(map[uint]struct{}, len(userGroupIDs))
	for _, ugID := range userGroupIDs {
		if ugID == 0 {
			continue
		}
		if _, ok := seenUG[ugID]; ok {
			continue
		}
		seenUG[ugID] = struct{}{}
		rows = append(rows, model.DeviceGroupAccess{DeviceGroupID: dgID, UserGroupID: ugID})
	}
	seenU := make(map[uint]struct{}, len(userIDs))
	for _, uID := range userIDs {
		if uID == 0 {
			continue
		}
		if _, ok := seenU[uID]; ok {
			continue
		}
		seenU[uID] = struct{}{}
		rows = append(rows, model.DeviceGroupAccess{DeviceGroupID: dgID, UserID: uID})
	}
	if len(rows) > 0 {
		return tx.CreateInBatches(&rows, 100).Error
	}
	return nil
}
