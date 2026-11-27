package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rustdesk-api/internal/model"
)

// AdminAuditHandler queries and purges connection, file transfer, alarm, and login logs.
type AdminAuditHandler struct {
	db *gorm.DB
}

// NewAdminAuditHandler creates a new AdminAuditHandler.
func NewAdminAuditHandler(db *gorm.DB) *AdminAuditHandler {
	return &AdminAuditHandler{db: db}
}

// ListConnLogs returns paginated connection audit logs.
func (h *AdminAuditHandler) ListConnLogs(c *gin.Context) {
	current, pageSize := ParsePagination(c, 30)
	q := h.db.Model(&model.AuditConn{})
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("peer_id LIKE ? OR from_peer LIKE ? OR from_name LIKE ? OR action LIKE ? OR ip LIKE ?", like, like, like, like, like)
	}
	var total int64
	q.Count(&total)
	var rows []model.AuditConn
	q.Order("id desc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&rows)
	c.JSON(http.StatusOK, gin.H{"total": total, "data": rows})
}

// ListFileLogs returns paginated file transfer audit logs.
func (h *AdminAuditHandler) ListFileLogs(c *gin.Context) {
	current, pageSize := ParsePagination(c, 30)
	q := h.db.Model(&model.AuditFile{})
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("peer_id LIKE ? OR from_peer LIKE ? OR from_name LIKE ? OR path LIKE ? OR info LIKE ?", like, like, like, like, like)
	}
	var total int64
	q.Count(&total)
	var rows []model.AuditFile
	q.Order("id desc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&rows)
	c.JSON(http.StatusOK, gin.H{"total": total, "data": rows})
}

// ListAlarmLogs returns paginated alarm audit logs.
func (h *AdminAuditHandler) ListAlarmLogs(c *gin.Context) {
	current, pageSize := ParsePagination(c, 30)
	q := h.db.Model(&model.AuditAlarm{})
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("peer_id LIKE ? OR info LIKE ? OR ip LIKE ?", like, like, like)
	}
	var total int64
	q.Count(&total)
	var rows []model.AuditAlarm
	q.Order("id desc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&rows)
	c.JSON(http.StatusOK, gin.H{"total": total, "data": rows})
}

// ListLoginLogs returns paginated authentication logs.
func (h *AdminAuditHandler) ListLoginLogs(c *gin.Context) {
	current, pageSize := ParsePagination(c, 30)
	q := h.db.Model(&model.LoginLog{})
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("username LIKE ? OR device_id LIKE ? OR ip LIKE ? OR auth_source LIKE ? OR status LIKE ?", like, like, like, like, like)
	}
	var total int64
	q.Count(&total)
	var rows []model.LoginLog
	q.Order("id desc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&rows)
	c.JSON(http.StatusOK, gin.H{"total": total, "data": rows})
}

// PurgeLogs deletes audit and login log rows older than `days` (default 30, 0 = all).
func (h *AdminAuditHandler) PurgeLogs(c *gin.Context) {
	days := 30
	if raw := strings.TrimSpace(c.Query("days")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			days = parsed
		}
	}

	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	if days == 0 {
		h.db.Where("1 = 1").Delete(&model.AuditConn{})
		h.db.Where("1 = 1").Delete(&model.AuditFile{})
		h.db.Where("1 = 1").Delete(&model.AuditAlarm{})
		h.db.Where("1 = 1").Delete(&model.LoginLog{})
	} else {
		h.db.Where("created_at < ?", cutoff).Delete(&model.AuditConn{})
		h.db.Where("created_at < ?", cutoff).Delete(&model.AuditFile{})
		h.db.Where("created_at < ?", cutoff).Delete(&model.AuditAlarm{})
		h.db.Where("created_at < ?", cutoff).Delete(&model.LoginLog{})
	}
	c.JSON(http.StatusOK, gin.H{"message": "logs purged", "days": days})
}
