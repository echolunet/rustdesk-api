package database

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"rustdesk-api/internal/config"
	"rustdesk-api/internal/model"
)

// Init opens the configured database, runs schema migrations, and seeds initial data.
func Init(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.DBType {
	case "sqlite":
		dialector = sqlite.Open(cfg.DBDSN)
	case "mysql":
		dialector = mysql.Open(cfg.DBDSN)
	case "postgres", "postgresql":
		dialector = postgres.Open(cfg.DBDSN)
	default:
		return nil, fmt.Errorf("unsupported DB_TYPE %q (must be sqlite, mysql, or postgres)", cfg.DBType)
	}

	gormLogLevel := logger.Warn
	if cfg.AppMode == "debug" {
		gormLogLevel = logger.Info
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(gormLogLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if cfg.DBType == "sqlite" {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.SetMaxOpenConns(1)
		}
		if !strings.Contains(cfg.DBDSN, ":memory:") {
			db.Exec("PRAGMA journal_mode=WAL;")
		}
		db.Exec("PRAGMA busy_timeout=5000;")
		db.Exec("PRAGMA foreign_keys=ON;")
	}

	if err := db.AutoMigrate(
		&model.Group{},
		&model.User{},
		&model.UserToken{},
		&model.DeviceGroup{},
		&model.DeviceGroupAccess{},
		&model.Peer{},
		&model.AddressBookCollection{},
		&model.AddressBookRule{},
		&model.AddressBookPeer{},
		&model.AddressBookTag{},
		&model.AuditConn{},
		&model.AuditFile{},
		&model.AuditAlarm{},
		&model.LoginLog{},
		&model.OAuthProvider{},
		&model.OAuthSession{},
		&model.LDAPConfig{},
		&model.SMTPConfig{},
		&model.VerificationCode{},
		&model.TrustedDevice{},
		&model.SystemSetting{},
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	if err := Seed(db, cfg); err != nil {
		return nil, fmt.Errorf("seed database: %w", err)
	}

	return db, nil
}

// Seed creates the default user group, default LDAP config row, and initial admin account if absent.
func Seed(db *gorm.DB, cfg *config.Config) error {
	var defaultGroup model.Group
	if err := db.Where("name = ?", "default").First(&defaultGroup).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		defaultGroup = model.Group{
			Name: "default",
			Note: "Default user group",
			Type: 1,
		}
		if err := db.Create(&defaultGroup).Error; err != nil {
			return err
		}
	}

	var ldapCount int64
	if err := db.Model(&model.LDAPConfig{}).Count(&ldapCount).Error; err != nil {
		return err
	}
	if ldapCount == 0 {
		ldapDefault := model.LDAPConfig{
			Enabled:      false,
			UserFilter:   "(&(objectClass=person)(|(uid=%s)(sAMAccountName=%s)(mail=%s)))",
			UsernameAttr: "uid",
			EmailAttr:    "mail",
			AutoRegister: true,
		}
		if err := db.Create(&ldapDefault).Error; err != nil {
			return err
		}
	}

	var smtpCount int64
	if err := db.Model(&model.SMTPConfig{}).Count(&smtpCount).Error; err != nil {
		return err
	}
	if smtpCount == 0 {
		smtpDefault := model.SMTPConfig{
			Enabled:           false,
			Port:              587,
			Security:          "starttls",
			FromName:          "RustDesk API",
			EmailLoginPolicy:  "off",
			CodeExpireMinutes: 10,
		}
		if err := db.Create(&smtpDefault).Error; err != nil {
			return err
		}
	}

	var adminCount int64
	if err := db.Model(&model.User{}).Where("is_admin = ?", true).Count(&adminCount).Error; err != nil {
		return err
	}
	if adminCount == 0 {
		initPass := cfg.AdminInitPassword
		generated := false
		if initPass == "" {
			var err error
			initPass, err = randomPassword(16)
			if err != nil {
				return err
			}
			generated = true
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(initPass), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		adminUser := model.User{
			Username:     cfg.AdminInitUser,
			Email:        "",
			PasswordHash: string(hash),
			Note:         "Initial Administrator",
			Status:       1,
			IsAdmin:      true,
			GroupID:      defaultGroup.ID,
			GroupName:    defaultGroup.Name,
			AuthSource:   "local",
		}
		if err := db.Create(&adminUser).Error; err != nil {
			return err
		}

		personalAB := model.AddressBookCollection{
			GUID:       uuid.NewString(),
			UserID:     adminUser.ID,
			OwnerName:  adminUser.Username,
			Name:       "Personal",
			Note:       "Personal Address Book",
			IsPersonal: true,
		}
		if err := db.Create(&personalAB).Error; err != nil {
			return err
		}

		if generated {
			log.Printf("================================================================")
			log.Printf(" INITIAL ADMIN ACCOUNT CREATED")
			log.Printf(" Username : %s", adminUser.Username)
			log.Printf(" Password : %s", initPass)
			log.Printf(" Please sign in at /_admin/ and change this password promptly.")
			log.Printf("================================================================")
		} else {
			log.Printf("Initial admin account %q created from ADMIN_INIT_PASSWORD.", adminUser.Username)
		}
	}

	return nil
}

func randomPassword(length int) (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%"
	buf := make([]byte, length)
	maxIdx := big.NewInt(int64(len(charset)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, maxIdx)
		if err != nil {
			return "", err
		}
		buf[i] = charset[n.Int64()]
	}
	return string(buf), nil
}
