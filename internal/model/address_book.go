package model

import "time"

const (
	ABRuleRead        = 1
	ABRuleReadWrite   = 2
	ABRuleFullControl = 3
)

// AddressBookCollection represents either a user's personal address book or a shared team address book.
type AddressBookCollection struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	GUID       string    `gorm:"size:64;uniqueIndex;not null" json:"guid"`
	UserID     uint      `gorm:"index;not null" json:"user_id"`
	OwnerName  string    `gorm:"size:128" json:"owner"`
	Name       string    `gorm:"size:128;not null" json:"name"`
	Note       string    `gorm:"size:512" json:"note"`
	IsPersonal bool      `gorm:"default:false;index" json:"is_personal"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// AddressBookRule shares an AddressBookCollection with a specific user or user group.
type AddressBookRule struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	CollectionGUID string    `gorm:"size:64;index;not null" json:"collection_guid"`
	UserID         uint      `gorm:"index" json:"user_id"`
	GroupID        uint      `gorm:"index" json:"group_id"`
	Rule           int       `gorm:"default:1;not null" json:"rule"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AddressBookPeer stores a contact/device entry inside an AddressBookCollection.
type AddressBookPeer struct {
	ID               uint        `gorm:"primaryKey" json:"id"`
	CollectionGUID   string      `gorm:"size:64;index;not null" json:"collection_guid"`
	PeerID           string      `gorm:"size:64;index;not null" json:"peer_id"`
	Username         string      `gorm:"size:128" json:"username"`
	Hostname         string      `gorm:"size:255" json:"hostname"`
	Alias            string      `gorm:"size:255" json:"alias"`
	Platform         string      `gorm:"size:64" json:"platform"`
	Tags             StringSlice `gorm:"type:text" json:"tags"`
	Hash             string      `gorm:"size:255" json:"hash"`
	Password         string      `gorm:"size:255" json:"password"`
	Note             string      `gorm:"size:512" json:"note"`
	ForceAlwaysRelay bool        `gorm:"default:false" json:"force_always_relay"`
	RDPPort          string      `gorm:"size:16" json:"rdp_port"`
	RDPUsername      string      `gorm:"size:128" json:"rdp_username"`
	SameServer       bool        `gorm:"default:false" json:"same_server"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

// AddressBookTag stores a tag and ARGB color inside an AddressBookCollection.
type AddressBookTag struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	CollectionGUID string    `gorm:"size:64;index;not null" json:"collection_guid"`
	Name           string    `gorm:"size:128;index;not null" json:"name"`
	Color          uint32    `gorm:"default:4280391411" json:"color"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
