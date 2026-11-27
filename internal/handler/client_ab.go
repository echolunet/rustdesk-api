package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rustdesk-api/internal/config"
	"rustdesk-api/internal/middleware"
	"rustdesk-api/internal/model"
	"rustdesk-api/internal/service"
)

// ClientABHandler implements both Legacy (v1) and Shared/Personal (v2) RustDesk Address Book APIs.
type ClientABHandler struct {
	db    *gorm.DB
	cfg   *config.Config
	abSvc *service.ABService
}

// NewClientABHandler creates a new ClientABHandler.
func NewClientABHandler(db *gorm.DB, cfg *config.Config, abSvc *service.ABService) *ClientABHandler {
	return &ClientABHandler{db: db, cfg: cfg, abSvc: abSvc}
}

// LegacyGet handles GET /api/ab and POST /api/ab/get.
func (h *ClientABHandler) LegacyGet(c *gin.Context) {
	user := middleware.CurrentUser(c)
	dataStr, updatedAt, err := h.abSvc.ExportLegacyAB(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load address book"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"updated_at": updatedAt,
		"data":       dataStr,
	})
}

// LegacySave handles POST /api/ab.
func (h *ClientABHandler) LegacySave(c *gin.Context) {
	user := middleware.CurrentUser(c)
	var req struct {
		Data string `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := h.abSvc.ImportLegacyAB(user, req.Data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse address book data"})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// PersonalProfile returns the user's personal address book metadata (/api/ab/personal).
func (h *ClientABHandler) PersonalProfile(c *gin.Context) {
	user := middleware.CurrentUser(c)
	col, err := h.abSvc.EnsurePersonalCollection(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load personal address book"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"guid": col.GUID,
		"name": col.Name,
		"note": col.Note,
		"rule": model.ABRuleFullControl,
	})
}

// SharedProfiles lists shared address book profiles accessible to the user (/api/ab/shared/profiles).
func (h *ClientABHandler) SharedProfiles(c *gin.Context) {
	user := middleware.CurrentUser(c)
	current, pageSize := ParsePagination(c, 50)
	items, total, err := h.abSvc.ListAccessibleSharedCollections(user, current, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load shared profiles"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"data":  items,
	})
}

// ListPeers returns peers inside a specified address book collection (/api/ab/peers).
func (h *ClientABHandler) ListPeers(c *gin.Context) {
	user := middleware.CurrentUser(c)
	guid := strings.TrimSpace(c.Query("ab"))
	if guid == "" && c.Request.Method == http.MethodPost {
		var body struct {
			AB string `json:"ab"`
		}
		_ = c.ShouldBindJSON(&body)
		guid = strings.TrimSpace(body.AB)
	}
	if guid == "" {
		col, err := h.abSvc.EnsurePersonalCollection(user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve personal address book"})
			return
		}
		guid = col.GUID
	}

	rule, _, err := h.abSvc.GetAccessRule(user, guid)
	if err != nil || rule < model.ABRuleRead {
		c.JSON(http.StatusForbidden, gin.H{"error": "address book access denied"})
		return
	}

	current, pageSize := ParsePagination(c, 200)
	q := h.db.Model(&model.AddressBookPeer{}).Where("collection_guid = ?", guid)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count peers failed"})
		return
	}

	var peers []model.AddressBookPeer
	if err := q.Order("id asc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&peers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load peers failed"})
		return
	}

	data := make([]map[string]interface{}, 0, len(peers))
	for _, p := range peers {
		tags := []string(p.Tags)
		if tags == nil {
			tags = []string{}
		}
		data = append(data, map[string]interface{}{
			"id":               p.PeerID,
			"username":         p.Username,
			"hostname":         p.Hostname,
			"alias":            p.Alias,
			"platform":         p.Platform,
			"tags":             tags,
			"hash":             p.Hash,
			"password":         p.Password,
			"note":             p.Note,
			"forceAlwaysRelay": p.ForceAlwaysRelay,
			"rdpPort":          p.RDPPort,
			"rdpUsername":      p.RDPUsername,
			"same_server":      p.SameServer,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"data":  data,
	})
}

// ListTags returns tags inside an address book collection (/api/ab/tags/:guid).
func (h *ClientABHandler) ListTags(c *gin.Context) {
	user := middleware.CurrentUser(c)
	guid := strings.TrimSpace(c.Param("guid"))
	rule, _, err := h.abSvc.GetAccessRule(user, guid)
	if err != nil || rule < model.ABRuleRead {
		c.JSON(http.StatusForbidden, gin.H{"error": "address book access denied"})
		return
	}

	var tags []model.AddressBookTag
	if err := h.db.Where("collection_guid = ?", guid).Order("id asc").Find(&tags).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load tags failed"})
		return
	}

	out := make([]map[string]interface{}, 0, len(tags))
	for _, t := range tags {
		out = append(out, map[string]interface{}{
			"name":  t.Name,
			"color": t.Color,
		})
	}
	c.JSON(http.StatusOK, out)
}

type abPeerPayload struct {
	ID               string   `json:"id"`
	PeerID           string   `json:"peer_id"`
	Username         string   `json:"username"`
	Hostname         string   `json:"hostname"`
	Alias            string   `json:"alias"`
	Platform         string   `json:"platform"`
	Tags             []string `json:"tags"`
	Hash             string   `json:"hash"`
	Password         string   `json:"password"`
	Note             string   `json:"note"`
	ForceAlwaysRelay bool     `json:"forceAlwaysRelay"`
	RDPPort          string   `json:"rdpPort"`
	RDPUsername      string   `json:"rdpUsername"`
	SameServer       bool     `json:"same_server"`
}

func (p abPeerPayload) resolvedID() string {
	if strings.TrimSpace(p.ID) != "" {
		return strings.TrimSpace(p.ID)
	}
	return strings.TrimSpace(p.PeerID)
}

// AddPeers adds or updates one or more peers in an address book collection (/api/ab/peer/add/:guid).
func (h *ClientABHandler) AddPeers(c *gin.Context) {
	user := middleware.CurrentUser(c)
	guid := strings.TrimSpace(c.Param("guid"))
	rule, _, err := h.abSvc.GetAccessRule(user, guid)
	if err != nil || rule < model.ABRuleReadWrite {
		c.JSON(http.StatusForbidden, gin.H{"error": "write permission required"})
		return
	}

	items, err := DecodeOneOrMany[abPeerPayload](c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid peer payload"})
		return
	}

	if h.cfg.MaxPeerOneAB > 0 {
		var currentCount int64
		h.db.Model(&model.AddressBookPeer{}).Where("collection_guid = ?", guid).Count(&currentCount)
		if int(currentCount)+len(items) > h.cfg.MaxPeerOneAB {
			c.JSON(http.StatusBadRequest, gin.H{"error": "address book peer limit reached"})
			return
		}
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		peerIDs := make([]string, 0, len(items))
		seenReqIDs := make(map[string]struct{}, len(items))
		tagNameSet := make(map[string]struct{})
		for _, item := range items {
			pid := item.resolvedID()
			if pid != "" {
				if _, ok := seenReqIDs[pid]; !ok {
					seenReqIDs[pid] = struct{}{}
					peerIDs = append(peerIDs, pid)
				}
			}
			for _, t := range item.Tags {
				tn := strings.TrimSpace(t)
				if tn != "" {
					tagNameSet[tn] = struct{}{}
				}
			}
		}
		if len(peerIDs) == 0 {
			return nil
		}

		var existingPeers []model.AddressBookPeer
		if err := tx.Where("collection_guid = ? AND peer_id IN ?", guid, peerIDs).Find(&existingPeers).Error; err != nil {
			return err
		}
		existingPeerMap := make(map[string]model.AddressBookPeer, len(existingPeers))
		for _, ep := range existingPeers {
			existingPeerMap[ep.PeerID] = ep
		}

		existingTagSet := make(map[string]struct{}, len(tagNameSet))
		if len(tagNameSet) > 0 {
			tagList := make([]string, 0, len(tagNameSet))
			for tn := range tagNameSet {
				tagList = append(tagList, tn)
			}
			var existingTags []model.AddressBookTag
			if err := tx.Where("collection_guid = ? AND name IN ?", guid, tagList).Find(&existingTags).Error; err != nil {
				return err
			}
			for _, et := range existingTags {
				existingTagSet[et.Name] = struct{}{}
			}
		}

		toCreatePeers := make([]model.AddressBookPeer, 0, len(peerIDs))
		pendingNewIdx := make(map[string]int, len(peerIDs))
		for _, item := range items {
			pid := item.resolvedID()
			if pid == "" {
				continue
			}
			if existing, ok := existingPeerMap[pid]; ok {
				if err := tx.Model(&existing).Updates(map[string]interface{}{
					"username":           item.Username,
					"hostname":           item.Hostname,
					"alias":              item.Alias,
					"platform":           item.Platform,
					"tags":               model.StringSlice(item.Tags),
					"hash":               item.Hash,
					"password":           item.Password,
					"note":               item.Note,
					"force_always_relay": item.ForceAlwaysRelay,
					"rdp_port":           item.RDPPort,
					"rdp_username":       item.RDPUsername,
					"same_server":        item.SameServer,
				}).Error; err != nil {
					return err
				}
			} else {
				row := model.AddressBookPeer{
					CollectionGUID:   guid,
					PeerID:           pid,
					Username:         item.Username,
					Hostname:         item.Hostname,
					Alias:            item.Alias,
					Platform:         item.Platform,
					Tags:             model.StringSlice(item.Tags),
					Hash:             item.Hash,
					Password:         item.Password,
					Note:             item.Note,
					ForceAlwaysRelay: item.ForceAlwaysRelay,
					RDPPort:          item.RDPPort,
					RDPUsername:      item.RDPUsername,
					SameServer:       item.SameServer,
				}
				if idx, dup := pendingNewIdx[pid]; dup {
					toCreatePeers[idx] = row
				} else {
					pendingNewIdx[pid] = len(toCreatePeers)
					toCreatePeers = append(toCreatePeers, row)
				}
			}
		}

		if len(toCreatePeers) > 0 {
			if err := tx.CreateInBatches(&toCreatePeers, 100).Error; err != nil {
				return err
			}
		}

		if len(tagNameSet) > len(existingTagSet) {
			toCreateTags := make([]model.AddressBookTag, 0, len(tagNameSet)-len(existingTagSet))
			for tn := range tagNameSet {
				if _, ok := existingTagSet[tn]; ok {
					continue
				}
				toCreateTags = append(toCreateTags, model.AddressBookTag{
					CollectionGUID: guid,
					Name:           tn,
					Color:          4280391411,
				})
			}
			if len(toCreateTags) > 0 {
				if err := tx.CreateInBatches(&toCreateTags, 100).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add peers"})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// UpdatePeers updates existing peers inside an address book collection (/api/ab/peer/update/:guid).
func (h *ClientABHandler) UpdatePeers(c *gin.Context) {
	h.AddPeers(c)
}

// DeletePeers removes peer IDs from an address book collection (/api/ab/peer/:guid).
func (h *ClientABHandler) DeletePeers(c *gin.Context) {
	user := middleware.CurrentUser(c)
	guid := strings.TrimSpace(c.Param("guid"))
	rule, _, err := h.abSvc.GetAccessRule(user, guid)
	if err != nil || rule < model.ABRuleReadWrite {
		c.JSON(http.StatusForbidden, gin.H{"error": "write permission required"})
		return
	}

	ids, err := decodeStringListOrField(c.Request.Body, "ids")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid peer ids payload"})
		return
	}
	if len(ids) > 0 {
		h.db.Where("collection_guid = ? AND peer_id IN ?", guid, ids).Delete(&model.AddressBookPeer{})
	}
	c.JSON(http.StatusOK, gin.H{})
}

type abTagPayload struct {
	Name  string          `json:"name"`
	Color model.FlexColor `json:"color"`
}

// AddTags creates or updates tags in an address book collection (/api/ab/tag/add/:guid).
func (h *ClientABHandler) AddTags(c *gin.Context) {
	user := middleware.CurrentUser(c)
	guid := strings.TrimSpace(c.Param("guid"))
	rule, _, err := h.abSvc.GetAccessRule(user, guid)
	if err != nil || rule < model.ABRuleReadWrite {
		c.JSON(http.StatusForbidden, gin.H{"error": "write permission required"})
		return
	}

	items, err := DecodeOneOrMany[abTagPayload](c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tag payload"})
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				continue
			}
			colorVal := uint32(item.Color)
			if colorVal == 0 {
				colorVal = 4280391411
			}
			var existing model.AddressBookTag
			if err := tx.Where("collection_guid = ? AND name = ?", guid, name).First(&existing).Error; err == nil {
				if err := tx.Model(&existing).Update("color", colorVal).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Create(&model.AddressBookTag{
					CollectionGUID: guid,
					Name:           name,
					Color:          colorVal,
				}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save tags"})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// UpdateTags updates tag properties inside an address book collection (/api/ab/tag/update/:guid).
func (h *ClientABHandler) UpdateTags(c *gin.Context) {
	h.AddTags(c)
}

// RenameTag renames a tag across AddressBookTag and AddressBookPeer entries (/api/ab/tag/rename/:guid).
func (h *ClientABHandler) RenameTag(c *gin.Context) {
	user := middleware.CurrentUser(c)
	guid := strings.TrimSpace(c.Param("guid"))
	rule, _, err := h.abSvc.GetAccessRule(user, guid)
	if err != nil || rule < model.ABRuleReadWrite {
		c.JSON(http.StatusForbidden, gin.H{"error": "write permission required"})
		return
	}

	var req struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rename payload"})
		return
	}
	oldName := strings.TrimSpace(req.Old)
	newName := strings.TrimSpace(req.New)
	if oldName == "" || newName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "old and new tag names are required"})
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.AddressBookTag{}).
			Where("collection_guid = ? AND name = ?", guid, oldName).
			Update("name", newName).Error; err != nil {
			return err
		}

		var peers []model.AddressBookPeer
		if err := tx.Where("collection_guid = ?", guid).Find(&peers).Error; err != nil {
			return err
		}
		for _, p := range peers {
			changed := false
			updatedTags := make([]string, 0, len(p.Tags))
			for _, t := range p.Tags {
				if t == oldName {
					updatedTags = append(updatedTags, newName)
					changed = true
				} else {
					updatedTags = append(updatedTags, t)
				}
			}
			if changed {
				if err := tx.Model(&p).Update("tags", model.StringSlice(updatedTags)).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rename tag"})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// DeleteTags deletes tag definitions and strips removed tags from peers (/api/ab/tag/:guid).
func (h *ClientABHandler) DeleteTags(c *gin.Context) {
	user := middleware.CurrentUser(c)
	guid := strings.TrimSpace(c.Param("guid"))
	rule, _, err := h.abSvc.GetAccessRule(user, guid)
	if err != nil || rule < model.ABRuleReadWrite {
		c.JSON(http.StatusForbidden, gin.H{"error": "write permission required"})
		return
	}

	names, err := decodeStringListOrField(c.Request.Body, "names")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tag names payload"})
		return
	}
	if len(names) == 0 {
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	removeSet := make(map[string]struct{}, len(names))
	for _, n := range names {
		removeSet[n] = struct{}{}
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("collection_guid = ? AND name IN ?", guid, names).Delete(&model.AddressBookTag{}).Error; err != nil {
			return err
		}
		var peers []model.AddressBookPeer
		if err := tx.Where("collection_guid = ?", guid).Find(&peers).Error; err != nil {
			return err
		}
		for _, p := range peers {
			kept := make([]string, 0, len(p.Tags))
			changed := false
			for _, t := range p.Tags {
				if _, rm := removeSet[t]; rm {
					changed = true
				} else {
					kept = append(kept, t)
				}
			}
			if changed {
				if err := tx.Model(&p).Update("tags", model.StringSlice(kept)).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete tags"})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// Settings returns address book limits (/api/ab/settings).
func (h *ClientABHandler) Settings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"max_peer_one_ab": h.cfg.MaxPeerOneAB,
	})
}

func decodeStringListOrField(bodyReader interface{ Read([]byte) (int, error) }, fieldName string) ([]string, error) {
	var raw json.RawMessage
	if err := json.NewDecoder(bodyReader).Decode(&raw); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var list []string
		if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	var obj map[string][]string
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return nil, err
	}
	return obj[fieldName], nil
}
