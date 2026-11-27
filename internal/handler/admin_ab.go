package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"rustdesk-api/internal/middleware"
	"rustdesk-api/internal/model"
)

// AdminABHandler provides full management of personal/shared address books, rules, peers, and tags.
type AdminABHandler struct {
	db *gorm.DB
}

// NewAdminABHandler creates a new AdminABHandler.
func NewAdminABHandler(db *gorm.DB) *AdminABHandler {
	return &AdminABHandler{db: db}
}

// ListCollections lists all address book collections.
func (h *AdminABHandler) ListCollections(c *gin.Context) {
	var cols []model.AddressBookCollection
	if err := h.db.Order("is_personal asc, id desc").Find(&cols).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query collections failed"})
		return
	}

	type countRow struct {
		CollectionGUID string
		Cnt            int64
	}
	var peerCounts []countRow
	h.db.Model(&model.AddressBookPeer{}).Select("collection_guid, count(*) as cnt").Group("collection_guid").Scan(&peerCounts)
	countMap := make(map[string]int64, len(peerCounts))
	for _, r := range peerCounts {
		countMap[r.CollectionGUID] = r.Cnt
	}

	out := make([]map[string]interface{}, 0, len(cols))
	for _, col := range cols {
		out = append(out, map[string]interface{}{
			"id":          col.ID,
			"guid":        col.GUID,
			"user_id":     col.UserID,
			"owner":       col.OwnerName,
			"name":        col.Name,
			"note":        col.Note,
			"is_personal": col.IsPersonal,
			"peer_count":  countMap[col.GUID],
			"created_at":  col.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// CreateCollection creates a shared address book collection.
func (h *AdminABHandler) CreateCollection(c *gin.Context) {
	admin := middleware.CurrentUser(c)
	var req struct {
		Name   string `json:"name"`
		Note   string `json:"note"`
		UserID uint   `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection name is required"})
		return
	}

	owner := admin
	if req.UserID > 0 && req.UserID != admin.ID {
		var u model.User
		if err := h.db.First(&u, req.UserID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "owner user not found"})
			return
		}
		owner = &u
	}

	col := model.AddressBookCollection{
		GUID:       uuid.NewString(),
		UserID:     owner.ID,
		OwnerName:  owner.Username,
		Name:       strings.TrimSpace(req.Name),
		Note:       req.Note,
		IsPersonal: false,
	}
	if err := h.db.Create(&col).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create collection failed"})
		return
	}
	c.JSON(http.StatusCreated, col)
}

// UpdateCollection updates a collection's name or note.
func (h *AdminABHandler) UpdateCollection(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	var col model.AddressBookCollection
	if err := h.db.Where("guid = ?", guid).First(&col).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	var req struct {
		Name string `json:"name"`
		Note string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection name is required"})
		return
	}
	h.db.Model(&col).Updates(map[string]interface{}{"name": strings.TrimSpace(req.Name), "note": req.Note})
	c.JSON(http.StatusOK, col)
}

// DeleteCollection deletes a non-personal address book collection and all its peers, tags, and sharing rules.
func (h *AdminABHandler) DeleteCollection(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	var col model.AddressBookCollection
	if err := h.db.Where("guid = ?", guid).First(&col).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	if col.IsPersonal {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete a personal address book"})
		return
	}
	_ = h.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("collection_guid = ?", guid).Delete(&model.AddressBookPeer{})
		tx.Where("collection_guid = ?", guid).Delete(&model.AddressBookTag{})
		tx.Where("collection_guid = ?", guid).Delete(&model.AddressBookRule{})
		return tx.Delete(&col).Error
	})
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ListRules lists sharing rules for a collection.
func (h *AdminABHandler) ListRules(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	var rules []model.AddressBookRule
	h.db.Where("collection_guid = ?", guid).Order("id asc").Find(&rules)
	c.JSON(http.StatusOK, gin.H{"data": rules})
}

// CreateRule adds a user or group sharing rule to a collection.
func (h *AdminABHandler) CreateRule(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	var req struct {
		UserID  uint `json:"user_id"`
		GroupID uint `json:"group_id"`
		Rule    int  `json:"rule"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.UserID == 0 && req.GroupID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id or group_id is required"})
		return
	}
	if req.Rule < model.ABRuleRead || req.Rule > model.ABRuleFullControl {
		req.Rule = model.ABRuleRead
	}
	r := model.AddressBookRule{
		CollectionGUID: guid,
		UserID:         req.UserID,
		GroupID:        req.GroupID,
		Rule:           req.Rule,
	}
	if err := h.db.Create(&r).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create sharing rule failed"})
		return
	}
	c.JSON(http.StatusCreated, r)
}

// DeleteRule removes a sharing rule by ID.
func (h *AdminABHandler) DeleteRule(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	ruleID, err := strconv.ParseUint(c.Param("ruleId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule id"})
		return
	}
	h.db.Where("collection_guid = ? AND id = ?", guid, ruleID).Delete(&model.AddressBookRule{})
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ListPeers lists peers inside a collection.
func (h *AdminABHandler) ListPeers(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	var peers []model.AddressBookPeer
	h.db.Where("collection_guid = ?", guid).Order("id asc").Find(&peers)
	c.JSON(http.StatusOK, gin.H{"data": peers})
}

// UpsertPeer adds or updates a peer entry inside a collection.
func (h *AdminABHandler) UpsertPeer(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	var req abPeerPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	pid := req.resolvedID()
	if pid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "peer_id is required"})
		return
	}
	var existing model.AddressBookPeer
	if err := h.db.Where("collection_guid = ? AND peer_id = ?", guid, pid).First(&existing).Error; err == nil {
		h.db.Model(&existing).Updates(map[string]interface{}{
			"username": req.Username,
			"hostname": req.Hostname,
			"alias":    req.Alias,
			"platform": req.Platform,
			"tags":     model.StringSlice(req.Tags),
			"note":     req.Note,
		})
		_ = h.db.First(&existing, existing.ID).Error
		c.JSON(http.StatusOK, existing)
		return
	}
	peer := model.AddressBookPeer{
		CollectionGUID: guid,
		PeerID:         pid,
		Username:       req.Username,
		Hostname:       req.Hostname,
		Alias:          req.Alias,
		Platform:       req.Platform,
		Tags:           model.StringSlice(req.Tags),
		Note:           req.Note,
	}
	h.db.Create(&peer)
	c.JSON(http.StatusCreated, peer)
}

// DeletePeer removes a peer from a collection.
func (h *AdminABHandler) DeletePeer(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	peerID := strings.TrimSpace(c.Param("peerId"))
	h.db.Where("collection_guid = ? AND peer_id = ?", guid, peerID).Delete(&model.AddressBookPeer{})
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ListTags lists tags inside a collection.
func (h *AdminABHandler) ListTags(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	var tags []model.AddressBookTag
	h.db.Where("collection_guid = ?", guid).Order("id asc").Find(&tags)
	c.JSON(http.StatusOK, gin.H{"data": tags})
}

// UpsertTag creates or updates a tag color inside a collection.
func (h *AdminABHandler) UpsertTag(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	var req abTagPayload
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag name is required"})
		return
	}
	name := strings.TrimSpace(req.Name)
	colorVal := uint32(req.Color)
	if colorVal == 0 {
		colorVal = 4280391411
	}
	var existing model.AddressBookTag
	if err := h.db.Where("collection_guid = ? AND name = ?", guid, name).First(&existing).Error; err == nil {
		h.db.Model(&existing).Update("color", colorVal)
		c.JSON(http.StatusOK, existing)
		return
	}
	tag := model.AddressBookTag{CollectionGUID: guid, Name: name, Color: colorVal}
	h.db.Create(&tag)
	c.JSON(http.StatusCreated, tag)
}

// DeleteTag removes a tag from a collection.
func (h *AdminABHandler) DeleteTag(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	name := strings.TrimSpace(c.Param("name"))
	h.db.Where("collection_guid = ? AND name = ?", guid, name).Delete(&model.AddressBookTag{})
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
