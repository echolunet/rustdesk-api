package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
	"gorm.io/gorm"

	"rustdesk-api/internal/model"
)

// TOTPService provides pure-Go RFC 6238 TOTP generation/verification, QR PNG encoding, and recovery codes.
type TOTPService struct {
	db *gorm.DB
}

// NewTOTPService creates a new TOTPService.
func NewTOTPService(db *gorm.DB) *TOTPService {
	return &TOTPService{db: db}
}

// GenerateSecret creates a 160-bit random Base32-encoded TOTP secret without padding.
func (s *TOTPService) GenerateSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

// BuildOTPAuthURI returns an otpauth://totp/ URI compatible with Google/Microsoft Authenticator and 1Password.
func (s *TOTPService) BuildOTPAuthURI(issuer, username, secret string) string {
	label := url.PathEscape(issuer + ":" + username)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	return fmt.Sprintf("otpauth://totp/%s?%s", label, q.Encode())
}

// GenerateQRDataURI encodes arbitrary text into a PNG QR code wrapped as a data:image/png;base64 URI.
func (s *TOTPService) GenerateQRDataURI(content string, size int) (string, error) {
	if size <= 0 {
		size = 240
	}
	png, err := qrcode.Encode(content, qrcode.Medium, size)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// GenerateCode computes the 6-digit RFC 6238 TOTP code for a given Base32 secret and timestamp.
func (s *TOTPService) GenerateCode(secret string, at time.Time) (string, error) {
	norm := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(norm)
	if err != nil {
		key, err = base32.StdEncoding.DecodeString(norm)
		if err != nil {
			return "", err
		}
	}

	counter := uint64(at.Unix() / 30)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)

	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	binCode := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])

	return fmt.Sprintf("%06d", binCode%1000000), nil
}

// VerifyCode validates a 6-digit TOTP code allowing +-30s clock skew.
func (s *TOTPService) VerifyCode(secret, code string, now time.Time) bool {
	clean := normalizeCode(code)
	if len(clean) != 6 || secret == "" {
		return false
	}

	matched := false
	for _, delta := range []time.Duration{-30 * time.Second, 0, 30 * time.Second} {
		expected, err := s.GenerateCode(secret, now.Add(delta))
		if err != nil {
			continue
		}
		if hmac.Equal([]byte(expected), []byte(clean)) {
			matched = true
		}
	}
	return matched
}

// BeginSetup generates and saves a pending TOTP secret on the user and returns secret, URI, and QR image data URI.
func (s *TOTPService) BeginSetup(user *model.User, issuer string) (string, string, string, error) {
	if user == nil {
		return "", "", "", errors.New("user is required")
	}
	secret, err := s.GenerateSecret()
	if err != nil {
		return "", "", "", err
	}
	if err := s.db.Model(user).Updates(map[string]interface{}{
		"totp_secret":  secret,
		"totp_enabled": false,
	}).Error; err != nil {
		return "", "", "", err
	}
	user.TOTPSecret = secret
	user.TOTPEnabled = false

	uri := s.BuildOTPAuthURI(issuer, user.Username, secret)
	qr, err := s.GenerateQRDataURI(uri, 240)
	if err != nil {
		return "", "", "", err
	}
	return secret, uri, qr, nil
}

// ConfirmSetup verifies the user's first authenticator code, enables 2FA, and returns 6 single-use recovery codes.
func (s *TOTPService) ConfirmSetup(user *model.User, code string) ([]string, error) {
	if user == nil || user.TOTPSecret == "" {
		return nil, errors.New("2FA setup has not been initiated")
	}
	if !s.VerifyCode(user.TOTPSecret, code, time.Now()) {
		return nil, errors.New("invalid 6-digit authenticator code")
	}

	plainCodes := make([]string, 0, 6)
	hashedCodes := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		rc, err := generateRecoveryCode()
		if err != nil {
			return nil, err
		}
		plainCodes = append(plainCodes, rc)
		hashedCodes = append(hashedCodes, hashNormalizedCode(rc))
	}

	if err := s.db.Model(user).Updates(map[string]interface{}{
		"totp_enabled":        true,
		"totp_recovery_codes": model.StringSlice(hashedCodes),
	}).Error; err != nil {
		return nil, err
	}
	user.TOTPEnabled = true
	user.TOTPRecoveryCodes = model.StringSlice(hashedCodes)
	return plainCodes, nil
}

// VerifyUserTFA checks a TOTP code or single-use recovery code (consuming the recovery code on match).
func (s *TOTPService) VerifyUserTFA(user *model.User, code string) bool {
	if user == nil || !user.TOTPEnabled {
		return false
	}
	if s.VerifyCode(user.TOTPSecret, code, time.Now()) {
		return true
	}

	candidateHash := hashNormalizedCode(code)
	if candidateHash == "" || len(user.TOTPRecoveryCodes) == 0 {
		return false
	}

	remaining := make([]string, 0, len(user.TOTPRecoveryCodes))
	consumed := false
	for _, h := range user.TOTPRecoveryCodes {
		if !consumed && hmac.Equal([]byte(h), []byte(candidateHash)) {
			consumed = true
			continue
		}
		remaining = append(remaining, h)
	}
	if !consumed {
		return false
	}

	_ = s.db.Model(user).Update("totp_recovery_codes", model.StringSlice(remaining)).Error
	user.TOTPRecoveryCodes = model.StringSlice(remaining)
	return true
}

// CreateChallenge creates an opaque short-lived challenge token returned in tfa_check responses.
func (s *TOTPService) CreateChallenge(userID uint, deviceUUID, ip string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(buf)
	row := model.VerificationCode{
		UserID:         userID,
		Purpose:        "tfa_challenge",
		CodeHash:       hashNormalizedCode(tok),
		ChallengeToken: tok,
		DeviceUUID:     deviceUUID,
		IP:             ip,
		ExpiresAt:      time.Now().Add(5 * time.Minute),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return "", err
	}
	return tok, nil
}

// DisableTOTP turns off 2FA and clears secret, recovery codes, and trusted devices for a user.
func (s *TOTPService) DisableTOTP(userID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"totp_enabled":        false,
			"totp_secret":         "",
			"totp_recovery_codes": model.StringSlice{},
		}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", userID).Delete(&model.TrustedDevice{}).Error
	})
}

// ResetUser2FA resets a user's 2FA configuration and pending verification codes.
func (s *TOTPService) ResetUser2FA(userID uint) error {
	if err := s.DisableTOTP(userID); err != nil {
		return err
	}
	return s.db.Where("user_id = ?", userID).Delete(&model.VerificationCode{}).Error
}

func normalizeCode(code string) string {
	s := strings.TrimSpace(strings.ToUpper(code))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func hashNormalizedCode(code string) string {
	norm := normalizeCode(code)
	if norm == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

func generateRecoveryCode() (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	maxIdx := big.NewInt(int64(len(charset)))
	part := func(n int) (string, error) {
		b := make([]byte, n)
		for i := 0; i < n; i++ {
			v, err := rand.Int(rand.Reader, maxIdx)
			if err != nil {
				return "", err
			}
			b[i] = charset[v.Int64()]
		}
		return string(b), nil
	}
	a, err := part(4)
	if err != nil {
		return "", err
	}
	b, err := part(4)
	if err != nil {
		return "", err
	}
	return a + "-" + b, nil
}
