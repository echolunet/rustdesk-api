package model

import "time"

// SMTPConfig stores the singleton SMTP server settings and global email login verification policy.
type SMTPConfig struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	Enabled           bool      `gorm:"default:false" json:"enabled"`
	Host              string    `gorm:"size:255" json:"host"`
	Port              int       `gorm:"default:587" json:"port"`
	Security          string    `gorm:"size:32;default:'starttls'" json:"security"`
	Username          string    `gorm:"size:255" json:"username"`
	Password          string    `gorm:"size:512" json:"-"`
	FromAddress       string    `gorm:"size:255" json:"from_address"`
	FromName          string    `gorm:"size:128;default:'RustDesk API'" json:"from_name"`
	SkipTLSVerify     bool      `gorm:"default:false" json:"skip_tls_verify"`
	EmailLoginPolicy  string    `gorm:"size:32;default:'off'" json:"email_login_policy"`
	CodeExpireMinutes int       `gorm:"default:10" json:"code_expire_minutes"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// VerificationCode persists hashed short-lived email codes and TOTP challenge tokens.
type VerificationCode struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"index;not null" json:"user_id"`
	Purpose        string    `gorm:"size:32;index;not null" json:"purpose"`
	CodeHash       string    `gorm:"size:128;not null" json:"-"`
	ChallengeToken string    `gorm:"size:128;index" json:"challenge_token"`
	DeviceUUID     string    `gorm:"size:128" json:"device_uuid"`
	IP             string    `gorm:"size:64" json:"ip"`
	Attempts       int       `gorm:"default:0" json:"attempts"`
	ExpiresAt      time.Time `gorm:"index;not null" json:"expires_at"`
	Used           bool      `gorm:"default:false;index" json:"used"`
	CreatedAt      time.Time `json:"created_at"`
}

// TrustedDevice records devices that have completed secondary email verification for a user.
type TrustedDevice struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index;not null" json:"user_id"`
	DeviceUUID string    `gorm:"size:128;index;not null" json:"device_uuid"`
	DeviceID   string    `gorm:"size:64" json:"device_id"`
	IP         string    `gorm:"size:64" json:"ip"`
	LastUsedAt time.Time `json:"last_used_at"`
	CreatedAt  time.Time `json:"created_at"`
}
