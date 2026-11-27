package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RequestBaseURL determines the externally visible scheme + host for the current HTTP request.
func RequestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if xfProto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); xfProto != "" {
		scheme = strings.Split(xfProto, ",")[0]
		scheme = strings.TrimSpace(scheme)
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	} else {
		host = strings.TrimSpace(strings.Split(host, ",")[0])
	}
	return scheme + "://" + host
}

// ParsePagination extracts current (1-based) and pageSize parameters with safe defaults.
func ParsePagination(c *gin.Context, defaultSize int) (int, int) {
	if defaultSize <= 0 {
		defaultSize = 20
	}
	current := parseIntQuery(c, "current", 0)
	if current <= 0 {
		current = parseIntQuery(c, "page", 1)
	}
	if current < 1 {
		current = 1
	}

	pageSize := parseIntQuery(c, "pageSize", 0)
	if pageSize <= 0 {
		pageSize = parseIntQuery(c, "page_size", defaultSize)
	}
	if pageSize <= 0 {
		pageSize = defaultSize
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	return current, pageSize
}

func parseIntQuery(c *gin.Context, key string, fallback int) int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

// DecodeOneOrMany decodes a JSON request body that may be either a single object T or an array []T.
func DecodeOneOrMany[T any](r io.Reader) ([]T, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 4<<20))
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var list []T
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	var one T
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return nil, err
	}
	return []T{one}, nil
}

// AccessibleDeviceGroupIDs returns all device group IDs accessible to a given user.
func AccessibleDeviceGroupIDs(db *gorm.DB, userID, userGroupID uint) ([]uint, error) {
	var ids []uint
	q := db.Table("device_group_accesses").Select("DISTINCT device_group_id")
	if userGroupID > 0 {
		q = q.Where("user_id = ? OR user_group_id = ?", userID, userGroupID)
	} else {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Pluck("device_group_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}
