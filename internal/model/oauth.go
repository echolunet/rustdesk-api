package model

import "time"

// OAuthProvider defines an OIDC, GitHub, or Google external identity provider.
type OAuthProvider struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:64;uniqueIndex;not null" json:"name"`
	ProviderType string    `gorm:"size:32;not null" json:"provider_type"`
	DisplayName  string    `gorm:"size:128" json:"display_name"`
	ClientID     string    `gorm:"size:255;not null" json:"client_id"`
	ClientSecret string    `gorm:"size:512" json:"client_secret"`
	Issuer       string    `gorm:"size:512" json:"issuer"`
	AuthURL      string    `gorm:"size:512" json:"auth_url"`
	TokenURL     string    `gorm:"size:512" json:"token_url"`
	UserInfoURL  string    `gorm:"size:512" json:"user_info_url"`
	Scopes       string    `gorm:"size:255" json:"scopes"`
	RedirectURL  string    `gorm:"size:512" json:"redirect_url"`
	Icon         string    `gorm:"type:text" json:"icon"`
	AutoRegister bool      `gorm:"default:true" json:"auto_register"`
	Enabled      bool      `gorm:"default:true;index" json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// OAuthSession tracks a client SSO login handshake from initiation to polling completion.
type OAuthSession struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Code         string    `gorm:"size:128;uniqueIndex;not null" json:"code"`
	ProviderName string    `gorm:"size:64;not null" json:"provider_name"`
	DeviceID     string    `gorm:"size:64" json:"device_id"`
	DeviceUUID   string    `gorm:"size:128" json:"device_uuid"`
	DeviceInfo   string    `gorm:"type:text" json:"device_info"`
	Status       string    `gorm:"size:32;index;not null" json:"status"`
	UserID       uint      `gorm:"index" json:"user_id"`
	AccessToken  string    `gorm:"type:text" json:"access_token"`
	ErrorMsg     string    `gorm:"size:512" json:"error_msg"`
	ExpiresAt    time.Time `gorm:"index" json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// LDAPConfig stores the singleton LDAP/Active Directory authentication settings.
type LDAPConfig struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Enabled       bool      `gorm:"default:false" json:"enabled"`
	URL           string    `gorm:"size:512" json:"url"`
	SkipTLSVerify bool      `gorm:"default:false" json:"skip_tls_verify"`
	BaseDN        string    `gorm:"size:512" json:"base_dn"`
	BindDN        string    `gorm:"size:512" json:"bind_dn"`
	BindPassword  string    `gorm:"size:512" json:"bind_password"`
	UserFilter    string    `gorm:"size:512" json:"user_filter"`
	UsernameAttr  string    `gorm:"size:64" json:"username_attr"`
	EmailAttr     string    `gorm:"size:64" json:"email_attr"`
	AutoRegister  bool      `gorm:"default:true" json:"auto_register"`
	UpdatedAt     time.Time `json:"updated_at"`
}
