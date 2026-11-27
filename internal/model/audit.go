package model

import "time"

// AuditConn records remote control session lifecycle events reported by clients.
type AuditConn struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ConnID    int64     `gorm:"index" json:"conn_id"`
	Action    string    `gorm:"size:64;index" json:"action"`
	PeerID    string    `gorm:"size:64;index" json:"peer_id"`
	FromPeer  string    `gorm:"size:64;index" json:"from_peer"`
	FromName  string    `gorm:"size:128" json:"from_name"`
	SessionID string    `gorm:"size:128" json:"session_id"`
	UUID      string    `gorm:"size:128" json:"uuid"`
	IP        string    `gorm:"size:64" json:"ip"`
	ConnType  int       `gorm:"default:0" json:"type"`
	Note      string    `gorm:"size:512" json:"note"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// AuditFile records file transfer events reported during remote sessions.
type AuditFile struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PeerID    string    `gorm:"size:64;index" json:"peer_id"`
	FromPeer  string    `gorm:"size:64;index" json:"from_peer"`
	FromName  string    `gorm:"size:128" json:"from_name"`
	UUID      string    `gorm:"size:128" json:"uuid"`
	IP        string    `gorm:"size:64" json:"ip"`
	FileType  int       `gorm:"default:0" json:"type"`
	Path      string    `gorm:"size:1024" json:"path"`
	Info      string    `gorm:"type:text" json:"info"`
	IsFile    bool      `gorm:"default:true" json:"is_file"`
	Num       int       `gorm:"default:1" json:"num"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// AuditAlarm records security or threshold alerts reported by clients.
type AuditAlarm struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AlarmType int       `gorm:"default:0;index" json:"typ"`
	PeerID    string    `gorm:"size:64;index" json:"peer_id"`
	UUID      string    `gorm:"size:128" json:"uuid"`
	IP        string    `gorm:"size:64" json:"ip"`
	Info      string    `gorm:"type:text" json:"info"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// LoginLog records authentication attempts across local, LDAP, OAuth, and Admin console logins.
type LoginLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index" json:"user_id"`
	Username   string    `gorm:"size:128;index" json:"username"`
	AuthSource string    `gorm:"size:64" json:"auth_source"`
	DeviceID   string    `gorm:"size:64" json:"device_id"`
	DeviceUUID string    `gorm:"size:128" json:"device_uuid"`
	Platform   string    `gorm:"size:64" json:"platform"`
	IP         string    `gorm:"size:64" json:"ip"`
	Status     string    `gorm:"size:32;index" json:"status"`
	Message    string    `gorm:"size:512" json:"message"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}
