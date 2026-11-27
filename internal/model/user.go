package model

import "time"

// Group represents a user organization group.
type Group struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128;uniqueIndex;not null" json:"name"`
	Note      string    `gorm:"size:512" json:"note"`
	Type      int       `gorm:"default:1" json:"type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User represents a system user capable of logging into RustDesk clients and optionally the Web Admin console.
type User struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Username     string     `gorm:"size:128;uniqueIndex;not null" json:"username"`
	Email        string     `gorm:"size:255;index" json:"email"`
	PasswordHash string     `gorm:"size:255" json:"-"`
	Note         string     `gorm:"size:512" json:"note"`
	Status       int        `gorm:"default:1;index" json:"status"`
	IsAdmin      bool       `gorm:"default:false;index" json:"is_admin"`
	GroupID      uint       `gorm:"index" json:"group_id"`
	GroupName    string     `gorm:"size:128" json:"group_name"`
	AuthSource         string      `gorm:"size:64;default:'local'" json:"auth_source"`
	ExternalID         string      `gorm:"size:255;index" json:"external_id"`
	TOTPEnabled        bool        `gorm:"default:false" json:"totp_enabled"`
	TOTPSecret         string      `gorm:"size:128" json:"-"`
	TOTPRecoveryCodes  StringSlice `gorm:"type:text" json:"-"`
	EmailVerifyEnabled bool        `gorm:"default:false" json:"email_verify_enabled"`
	LastLoginAt        *time.Time  `json:"last_login_at"`
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`
}

// UserToken tracks issued JWT session tokens for revocation and device session auditing.
type UserToken struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	TokenID    string    `gorm:"size:64;uniqueIndex;not null" json:"token_id"`
	UserID     uint      `gorm:"index;not null" json:"user_id"`
	Username   string    `gorm:"size:128" json:"username"`
	DeviceID   string    `gorm:"size:64;index" json:"device_id"`
	DeviceUUID string    `gorm:"size:128;index" json:"device_uuid"`
	IP         string    `gorm:"size:64" json:"ip"`
	ExpiresAt  time.Time `gorm:"index" json:"expires_at"`
	Revoked    bool      `gorm:"default:false;index" json:"revoked"`
	CreatedAt  time.Time `json:"created_at"`
}
