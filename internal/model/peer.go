package model

import "time"

// DeviceGroup organizes managed RustDesk peers into logical ACL groups.
type DeviceGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Note      string    `gorm:"size:512" json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DeviceGroupAccess grants a user group or specific user access to a DeviceGroup.
type DeviceGroupAccess struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	DeviceGroupID uint      `gorm:"index;not null" json:"device_group_id"`
	UserGroupID   uint      `gorm:"index" json:"user_group_id"`
	UserID        uint      `gorm:"index" json:"user_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// Peer stores telemetry and ownership metadata for a registered RustDesk device.
type Peer struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	PeerID          string     `gorm:"size:64;uniqueIndex;not null" json:"peer_id"`
	UUID            string     `gorm:"size:128;index" json:"uuid"`
	UserID          uint       `gorm:"index" json:"user_id"`
	UserName        string     `gorm:"size:128;index" json:"user_name"`
	DeviceGroupID   uint       `gorm:"index" json:"device_group_id"`
	DeviceGroupName string     `gorm:"size:128" json:"device_group_name"`
	Hostname        string     `gorm:"size:255" json:"hostname"`
	Username        string     `gorm:"size:128" json:"username"`
	OS              string     `gorm:"size:128" json:"os"`
	CPU             string     `gorm:"size:255" json:"cpu"`
	Memory          string     `gorm:"size:64" json:"memory"`
	Version         string     `gorm:"size:64" json:"version"`
	Alias           string     `gorm:"size:255" json:"alias"`
	Note            string     `gorm:"size:512" json:"note"`
	LastIP          string     `gorm:"size:64" json:"last_ip"`
	LastOnlineAt    *time.Time `gorm:"index" json:"last_online_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
