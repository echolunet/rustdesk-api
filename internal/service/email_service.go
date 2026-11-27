package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"math/big"
	"net"
	"net/smtp"
	"strings"
	"time"

	"gorm.io/gorm"

	"rustdesk-api/internal/model"
)

// EmailService sends SMTP verification/test emails and evaluates per-user/global email login policies.
type EmailService struct {
	db *gorm.DB
}

// NewEmailService creates a new EmailService.
func NewEmailService(db *gorm.DB) *EmailService {
	return &EmailService{db: db}
}

// GetConfig returns the singleton SMTPConfig row.
func (s *EmailService) GetConfig() (*model.SMTPConfig, error) {
	var cfg model.SMTPConfig
	if err := s.db.First(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

// NeedsEmailVerification determines whether a login requires an email OTP challenge.
func (s *EmailService) NeedsEmailVerification(user *model.User, deviceUUID string) bool {
	if user == nil || strings.TrimSpace(user.Email) == "" {
		return false
	}
	cfg, err := s.GetConfig()
	if err != nil || !cfg.Enabled {
		return false
	}
	if user.EmailVerifyEnabled {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(cfg.EmailLoginPolicy)) {
	case "always":
		return true
	case "new_device":
		if strings.TrimSpace(deviceUUID) == "" {
			return true
		}
		var count int64
		s.db.Model(&model.TrustedDevice{}).
			Where("user_id = ? AND device_uuid = ?", user.ID, strings.TrimSpace(deviceUUID)).
			Count(&count)
		return count == 0
	default:
		return false
	}
}

// SendVerificationCode generates a 6-digit email verification code (with 45s resend cooldown) and emails it.
func (s *EmailService) SendVerificationCode(user *model.User, deviceID, deviceUUID, ip string) error {
	if user == nil || strings.TrimSpace(user.Email) == "" {
		return errors.New("user email address is not configured")
	}
	cfg, err := s.GetConfig()
	if err != nil || !cfg.Enabled {
		return errors.New("SMTP email service is not enabled")
	}

	now := time.Now()
	var recent model.VerificationCode
	if err := s.db.Where("user_id = ? AND purpose = ? AND used = ? AND expires_at > ?", user.ID, "email_login", false, now).
		Order("id desc").First(&recent).Error; err == nil {
		if now.Sub(recent.CreatedAt) < 45*time.Second {
			return nil
		}
	}

	code, err := randomDigits(6)
	if err != nil {
		return err
	}

	expireMin := cfg.CodeExpireMinutes
	if expireMin <= 0 {
		expireMin = 10
	}

	// Mark prior unused email codes as used so only the latest code is valid.
	s.db.Model(&model.VerificationCode{}).
		Where("user_id = ? AND purpose = ? AND used = ?", user.ID, "email_login", false).
		Update("used", true)

	row := model.VerificationCode{
		UserID:     user.ID,
		Purpose:    "email_login",
		CodeHash:   hashNormalizedCode(code),
		DeviceUUID: strings.TrimSpace(deviceUUID),
		IP:         ip,
		ExpiresAt:  now.Add(time.Duration(expireMin) * time.Minute),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return err
	}

	subject := "RustDesk 登录安全验证码"
	body := buildVerificationEmailHTML(user.Username, code, deviceID, ip, expireMin)
	if err := s.SendMail(cfg, user.Email, subject, body); err != nil {
		s.db.Model(&row).Update("used", true)
		return fmt.Errorf("failed to send verification email: %w", err)
	}
	return nil
}

// VerifyEmailCode validates a 6-digit email code for the user and enforces a max of 5 attempts.
func (s *EmailService) VerifyEmailCode(userID uint, code string) error {
	norm := normalizeCode(code)
	if len(norm) != 6 {
		return errors.New("invalid 6-digit verification code")
	}

	var row model.VerificationCode
	if err := s.db.Where("user_id = ? AND purpose = ? AND used = ?", userID, "email_login", false).
		Order("id desc").First(&row).Error; err != nil {
		return errors.New("verification code not found, please request a new one")
	}

	if time.Now().After(row.ExpiresAt) {
		s.db.Model(&row).Update("used", true)
		return errors.New("verification code has expired")
	}
	if row.Attempts >= 5 {
		s.db.Model(&row).Update("used", true)
		return errors.New("too many failed verification attempts")
	}

	candidateHash := hashNormalizedCode(norm)
	if !hmac.Equal([]byte(row.CodeHash), []byte(candidateHash)) {
		s.db.Model(&row).Update("attempts", row.Attempts+1)
		return errors.New("incorrect verification code")
	}

	return s.db.Model(&row).Update("used", true).Error
}

// MarkDeviceTrusted records or updates a TrustedDevice entry after successful email verification.
func (s *EmailService) MarkDeviceTrusted(userID uint, deviceID, deviceUUID, ip string) {
	uuidVal := strings.TrimSpace(deviceUUID)
	if uuidVal == "" {
		return
	}
	now := time.Now()
	var dev model.TrustedDevice
	if err := s.db.Where("user_id = ? AND device_uuid = ?", userID, uuidVal).First(&dev).Error; err == nil {
		s.db.Model(&dev).Updates(map[string]interface{}{
			"device_id":    deviceID,
			"ip":           ip,
			"last_used_at": now,
		})
		return
	}
	s.db.Create(&model.TrustedDevice{
		UserID:     userID,
		DeviceUUID: uuidVal,
		DeviceID:   deviceID,
		IP:         ip,
		LastUsedAt: now,
	})
}

// SendTestEmail sends a diagnostic email using the provided SMTPConfig.
func (s *EmailService) SendTestEmail(cfg *model.SMTPConfig, to string) error {
	if strings.TrimSpace(to) == "" {
		return errors.New("recipient email address is required")
	}
	htmlBody := buildVerificationEmailHTML("Admin Test", "888666", "SMTP-DIAGNOSTIC", "127.0.0.1", 10)
	return s.SendMail(cfg, strings.TrimSpace(to), "RustDesk API SMTP 测试邮件", htmlBody)
}

// SendMail delivers an HTML email using implicit TLS (ssl), STARTTLS, or plain SMTP.
func (s *EmailService) SendMail(cfg *model.SMTPConfig, to, subject, htmlBody string) error {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return errors.New("SMTP host is required")
	}
	port := cfg.Port
	if port <= 0 {
		port = 587
	}
	fromAddr := strings.TrimSpace(cfg.FromAddress)
	if fromAddr == "" {
		fromAddr = strings.TrimSpace(cfg.Username)
	}
	if fromAddr == "" {
		return errors.New("SMTP from_address is required")
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	tlsCfg := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: cfg.SkipTLSVerify,
	}

	var client *smtp.Client
	var err error
	sec := strings.ToLower(strings.TrimSpace(cfg.Security))
	switch sec {
	case "ssl", "tls":
		conn, dialErr := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsCfg)
		if dialErr != nil {
			return dialErr
		}
		client, err = smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return err
		}
	case "none":
		conn, dialErr := net.DialTimeout("tcp", addr, 10*time.Second)
		if dialErr != nil {
			return dialErr
		}
		client, err = smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return err
		}
	default:
		conn, dialErr := net.DialTimeout("tcp", addr, 10*time.Second)
		if dialErr != nil {
			return dialErr
		}
		client, err = smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return err
		}
		if ok, _ := client.Extension("STARTTLS"); ok {
			if tlsErr := client.StartTLS(tlsCfg); tlsErr != nil {
				_ = client.Close()
				return tlsErr
			}
		}
	}
	defer client.Close()

	if strings.TrimSpace(cfg.Username) != "" {
		auth := smtp.PlainAuth("", strings.TrimSpace(cfg.Username), cfg.Password, host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(fromAddr); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}

	fromHeader := fromAddr
	if strings.TrimSpace(cfg.FromName) != "" {
		fromHeader = fmt.Sprintf("=?UTF-8?B?%s?= <%s>",
			base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(cfg.FromName))), fromAddr)
	}
	encodedSubj := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))

	msg := strings.Join([]string{
		"From: " + fromHeader,
		"To: " + to,
		"Subject: " + encodedSubj,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		htmlBody,
	}, "\r\n")

	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// MaskEmail obscures the local part of an email address for safe UI display.
func (s *EmailService) MaskEmail(email string) string {
	parts := strings.SplitN(strings.TrimSpace(email), "@", 2)
	if len(parts) != 2 {
		return "***"
	}
	local := parts[0]
	if len(local) <= 2 {
		return local[:1] + "***@" + parts[1]
	}
	return local[:2] + "***@" + parts[1]
}

func randomDigits(n int) (string, error) {
	maxIdx := big.NewInt(10)
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, maxIdx)
		if err != nil {
			return "", err
		}
		buf[i] = byte('0' + v.Int64())
	}
	return string(buf), nil
}

func buildVerificationEmailHTML(username, code, deviceID, ip string, expireMin int) string {
	return `<!DOCTYPE html>
<html>
<body style="margin:0;padding:24px;background:#f4f7fb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;">
  <div style="max-width:520px;margin:0 auto;background:#ffffff;border-radius:14px;border:1px solid #e2e8f0;padding:28px;">
    <div style="font-size:18px;font-weight:700;color:#0f172a;margin-bottom:8px;">RustDesk 登录安全验证</div>
    <p style="color:#475569;font-size:14px;line-height:1.6;margin:0 0 18px;">
      您好 <strong>` + html.EscapeString(username) + `</strong>，您正在尝试登录 RustDesk 账号。请使用下方 6 位一次性验证码完成身份校验：
    </p>
    <div style="background:#0f172a;color:#ffffff;font-size:28px;font-weight:800;letter-spacing:8px;text-align:center;padding:16px;border-radius:10px;font-family:monospace;margin-bottom:18px;">
      ` + html.EscapeString(code) + `
    </div>
    <div style="font-size:12.5px;color:#64748b;line-height:1.7;background:#f8fafc;border-radius:8px;padding:12px 14px;">
      <div><strong>有效期：</strong> ` + fmt.Sprintf("%d", expireMin) + ` 分钟</div>
      <div><strong>设备标识：</strong> ` + html.EscapeString(deviceID) + `</div>
      <div><strong>来源 IP：</strong> ` + html.EscapeString(ip) + `</div>
    </div>
    <p style="font-size:12px;color:#94a3b8;margin:16px 0 0;">如果这不是您本人的操作，请立即修改账户密码或联系管理员。</p>
  </div>
</body>
</html>`
}
