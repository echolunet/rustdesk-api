package service

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"rustdesk-api/internal/model"
)

// ABService manages personal and shared RustDesk Address Books, peers, tags, and legacy v1 sync.
type ABService struct {
	db *gorm.DB
}

// NewABService creates a new ABService.
func NewABService(db *gorm.DB) *ABService {
	return &ABService{db: db}
}

// SharedProfileDTO matches the /api/ab/shared/profiles item format expected by RustDesk clients.
type SharedProfileDTO struct {
	GUID      string `json:"guid"`
	Name      string `json:"name"`
	Note      string `json:"note"`
	Rule      int    `json:"rule"`
	Owner     string `json:"owner"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// LegacyABPeer represents one peer item inside legacy /api/ab serialized JSON.
type LegacyABPeer struct {
	ID               string   `json:"id"`
	Username         string   `json:"username"`
	Hostname         string   `json:"hostname"`
	Alias            string   `json:"alias"`
	Platform         string   `json:"platform"`
	Tags             []string `json:"tags"`
	Hash             string   `json:"hash,omitempty"`
	Password         string   `json:"password,omitempty"`
	ForceAlwaysRelay bool     `json:"forceAlwaysRelay,omitempty"`
	RDPPort          string   `json:"rdpPort,omitempty"`
	RDPUsername      string   `json:"rdpUsername,omitempty"`
}

// EnsurePersonalCollection finds or creates the user's personal AddressBookCollection.
func (s *ABService) EnsurePersonalCollection(user *model.User) (*model.AddressBookCollection, error) {
	var col model.AddressBookCollection
	err := s.db.Where("user_id = ? AND is_personal = ?", user.ID, true).First(&col).Error
	if err == nil {
		return &col, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	col = model.AddressBookCollection{
		GUID:       uuid.NewString(),
		UserID:     user.ID,
		OwnerName:  user.Username,
		Name:       "Personal",
		Note:       "Personal Address Book",
		IsPersonal: true,
	}
	if err := s.db.Create(&col).Error; err != nil {
		return nil, err
	}
	return &col, nil
}

// GetAccessRule returns the user's effective permission level (0=none, 1=read, 2=read/write, 3=full) for a collection.
func (s *ABService) GetAccessRule(user *model.User, guid string) (int, *model.AddressBookCollection, error) {
	var col model.AddressBookCollection
	if err := s.db.Where("guid = ?", guid).First(&col).Error; err != nil {
		return 0, nil, err
	}
	if col.UserID == user.ID || user.IsAdmin {
		return model.ABRuleFullControl, &col, nil
	}
	if col.IsPersonal {
		return 0, &col, nil
	}

	var rules []model.AddressBookRule
	q := s.db.Where("collection_guid = ?", guid)
	if user.GroupID > 0 {
		q = q.Where("user_id = ? OR group_id = ?", user.ID, user.GroupID)
	} else {
		q = q.Where("user_id = ?", user.ID)
	}
	if err := q.Find(&rules).Error; err != nil {
		return 0, &col, err
	}

	best := 0
	for _, r := range rules {
		if r.Rule > best {
			best = r.Rule
		}
	}
	return best, &col, nil
}

// ListAccessibleSharedCollections returns paginated shared address book profiles accessible to the user.
func (s *ABService) ListAccessibleSharedCollections(user *model.User, current, pageSize int) ([]SharedProfileDTO, int64, error) {
	if current < 1 {
		current = 1
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}

	var collections []model.AddressBookCollection
	var total int64

	if user.IsAdmin {
		base := s.db.Model(&model.AddressBookCollection{}).Where("is_personal = ?", false)
		if err := base.Count(&total).Error; err != nil {
			return nil, 0, err
		}
		if err := base.Order("id desc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&collections).Error; err != nil {
			return nil, 0, err
		}
		out := make([]SharedProfileDTO, 0, len(collections))
		for _, c := range collections {
			out = append(out, SharedProfileDTO{
				GUID:      c.GUID,
				Name:      c.Name,
				Note:      c.Note,
				Rule:      model.ABRuleFullControl,
				Owner:     c.OwnerName,
				CreatedAt: c.CreatedAt.Unix(),
				UpdatedAt: c.UpdatedAt.Unix(),
			})
		}
		return out, total, nil
	}

	var rules []model.AddressBookRule
	rq := s.db.Model(&model.AddressBookRule{})
	if user.GroupID > 0 {
		rq = rq.Where("user_id = ? OR group_id = ?", user.ID, user.GroupID)
	} else {
		rq = rq.Where("user_id = ?", user.ID)
	}
	if err := rq.Find(&rules).Error; err != nil {
		return nil, 0, err
	}

	ruleMap := make(map[string]int, len(rules))
	guids := make([]string, 0, len(rules))
	for _, r := range rules {
		if prev, ok := ruleMap[r.CollectionGUID]; !ok || r.Rule > prev {
			ruleMap[r.CollectionGUID] = r.Rule
			guids = append(guids, r.CollectionGUID)
		}
	}

	base := s.db.Model(&model.AddressBookCollection{}).Where("is_personal = ?", false)
	if len(guids) > 0 {
		base = base.Where("user_id = ? OR guid IN ?", user.ID, guids)
	} else {
		base = base.Where("user_id = ?", user.ID)
	}

	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := base.Order("id desc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&collections).Error; err != nil {
		return nil, 0, err
	}

	out := make([]SharedProfileDTO, 0, len(collections))
	for _, c := range collections {
		rule := ruleMap[c.GUID]
		if c.UserID == user.ID {
			rule = model.ABRuleFullControl
		}
		out = append(out, SharedProfileDTO{
			GUID:      c.GUID,
			Name:      c.Name,
			Note:      c.Note,
			Rule:      rule,
			Owner:     c.OwnerName,
			CreatedAt: c.CreatedAt.Unix(),
			UpdatedAt: c.UpdatedAt.Unix(),
		})
	}
	return out, total, nil
}

// ExportLegacyAB serializes the user's personal address book into legacy /api/ab JSON format.
func (s *ABService) ExportLegacyAB(user *model.User) (string, int64, error) {
	col, err := s.EnsurePersonalCollection(user)
	if err != nil {
		return "", 0, err
	}

	var peers []model.AddressBookPeer
	if err := s.db.Where("collection_guid = ?", col.GUID).Order("id asc").Find(&peers).Error; err != nil {
		return "", 0, err
	}

	var tags []model.AddressBookTag
	if err := s.db.Where("collection_guid = ?", col.GUID).Order("id asc").Find(&tags).Error; err != nil {
		return "", 0, err
	}

	tagNames := make([]string, 0, len(tags))
	tagColors := make(map[string]uint32, len(tags))
	for _, t := range tags {
		tagNames = append(tagNames, t.Name)
		tagColors[t.Name] = t.Color
	}

	legacyPeers := make([]LegacyABPeer, 0, len(peers))
	for _, p := range peers {
		peerTags := []string(p.Tags)
		if peerTags == nil {
			peerTags = []string{}
		}
		legacyPeers = append(legacyPeers, LegacyABPeer{
			ID:               p.PeerID,
			Username:         p.Username,
			Hostname:         p.Hostname,
			Alias:            p.Alias,
			Platform:         p.Platform,
			Tags:             peerTags,
			Hash:             p.Hash,
			Password:         p.Password,
			ForceAlwaysRelay: p.ForceAlwaysRelay,
			RDPPort:          p.RDPPort,
			RDPUsername:      p.RDPUsername,
		})
	}

	colorBytes, _ := json.Marshal(tagColors)
	payload := map[string]interface{}{
		"tags":       tagNames,
		"peers":      legacyPeers,
		"tag_colors": string(colorBytes),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", 0, err
	}
	return string(raw), col.UpdatedAt.Unix(), nil
}

// ImportLegacyAB synchronizes a legacy /api/ab JSON string payload into the user's personal AddressBookCollection.
func (s *ABService) ImportLegacyAB(user *model.User, rawData string) error {
	col, err := s.EnsurePersonalCollection(user)
	if err != nil {
		return err
	}

	var body struct {
		Tags      []string        `json:"tags"`
		Peers     []LegacyABPeer  `json:"peers"`
		TagColors json.RawMessage `json:"tag_colors"`
	}
	if err := json.Unmarshal([]byte(rawData), &body); err != nil {
		return err
	}

	colorMap := make(map[string]uint32)
	trimmedColors := strings.TrimSpace(string(body.TagColors))
	if trimmedColors != "" && trimmedColors != "null" {
		if strings.HasPrefix(trimmedColors, "\"") {
			var inner string
			if err := json.Unmarshal(body.TagColors, &inner); err == nil && strings.TrimSpace(inner) != "" {
				var parsed map[string]model.FlexColor
				if err := json.Unmarshal([]byte(inner), &parsed); err == nil {
					for k, v := range parsed {
						colorMap[k] = uint32(v)
					}
				}
			}
		} else {
			var parsed map[string]model.FlexColor
			if err := json.Unmarshal(body.TagColors, &parsed); err == nil {
				for k, v := range parsed {
					colorMap[k] = uint32(v)
				}
			}
		}
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("collection_guid = ?", col.GUID).Delete(&model.AddressBookPeer{}).Error; err != nil {
			return err
		}
		if err := tx.Where("collection_guid = ?", col.GUID).Delete(&model.AddressBookTag{}).Error; err != nil {
			return err
		}

		for _, tagName := range body.Tags {
			tagName = strings.TrimSpace(tagName)
			if tagName == "" {
				continue
			}
			colorVal := colorMap[tagName]
			if colorVal == 0 {
				colorVal = 4280391411
			}
			if err := tx.Create(&model.AddressBookTag{
				CollectionGUID: col.GUID,
				Name:           tagName,
				Color:          colorVal,
			}).Error; err != nil {
				return err
			}
		}

		for _, p := range body.Peers {
			if strings.TrimSpace(p.ID) == "" {
				continue
			}
			if err := tx.Create(&model.AddressBookPeer{
				CollectionGUID:   col.GUID,
				PeerID:           strings.TrimSpace(p.ID),
				Username:         p.Username,
				Hostname:         p.Hostname,
				Alias:            p.Alias,
				Platform:         p.Platform,
				Tags:             model.StringSlice(p.Tags),
				Hash:             p.Hash,
				Password:         p.Password,
				ForceAlwaysRelay: p.ForceAlwaysRelay,
				RDPPort:          p.RDPPort,
				RDPUsername:      p.RDPUsername,
			}).Error; err != nil {
				return err
			}
		}

		return tx.Model(col).Update("note", col.Note).Error
	})
}
