package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rustdesk-api/internal/model"
)

// ClientAuditHandler ingests connection, file-transfer, and alarm audit events from RustDesk clients.
type ClientAuditHandler struct {
	db *gorm.DB
}

// NewClientAuditHandler creates a new ClientAuditHandler.
func NewClientAuditHandler(db *gorm.DB) *ClientAuditHandler {
	return &ClientAuditHandler{db: db}
}

type auditConnRequest struct {
	ConnID    int64            `json:"conn_id"`
	Action    string           `json:"action"`
	ID        model.FlexString `json:"id"`
	PeerID    model.FlexString `json:"peer_id"`
	FromPeer  model.FlexString `json:"from_peer"`
	FromName  string           `json:"from_name"`
	SessionID string           `json:"session_id"`
	UUID      string           `json:"uuid"`
	Type      int              `json:"type"`
	Note      string           `json:"note"`
}

// LogConn records a connection lifecycle audit entry (/api/audit/conn).
func (h *ClientAuditHandler) LogConn(c *gin.Context) {
	var req auditConnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid audit conn payload"})
		return
	}

	peerID := strings.TrimSpace(req.PeerID.String())
	if peerID == "" {
		peerID = strings.TrimSpace(req.ID.String())
	}

	row := model.AuditConn{
		ConnID:    req.ConnID,
		Action:    strings.TrimSpace(req.Action),
		PeerID:    peerID,
		FromPeer:  strings.TrimSpace(req.FromPeer.String()),
		FromName:  strings.TrimSpace(req.FromName),
		SessionID: strings.TrimSpace(req.SessionID),
		UUID:      strings.TrimSpace(req.UUID),
		IP:        c.ClientIP(),
		ConnType:  req.Type,
		Note:      req.Note,
	}
	h.db.Create(&row)
	c.JSON(http.StatusOK, gin.H{})
}

type auditFileRequest struct {
	ID       model.FlexString `json:"id"`
	PeerID   model.FlexString `json:"peer_id"`
	FromPeer model.FlexString `json:"from_peer"`
	FromName string           `json:"from_name"`
	UUID     string           `json:"uuid"`
	Type     int              `json:"type"`
	Path     string           `json:"path"`
	Info     string           `json:"info"`
	IsFile   bool             `json:"is_file"`
	Num      int              `json:"num"`
}

// LogFile records a file transfer audit entry (/api/audit/file).
func (h *ClientAuditHandler) LogFile(c *gin.Context) {
	var req auditFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid audit file payload"})
		return
	}

	peerID := strings.TrimSpace(req.PeerID.String())
	if peerID == "" {
		peerID = strings.TrimSpace(req.ID.String())
	}

	row := model.AuditFile{
		PeerID:   peerID,
		FromPeer: strings.TrimSpace(req.FromPeer.String()),
		FromName: strings.TrimSpace(req.FromName),
		UUID:     strings.TrimSpace(req.UUID),
		IP:       c.ClientIP(),
		FileType: req.Type,
		Path:     req.Path,
		Info:     req.Info,
		IsFile:   req.IsFile,
		Num:      req.Num,
	}
	h.db.Create(&row)
	c.JSON(http.StatusOK, gin.H{})
}

type auditAlarmRequest struct {
	Typ    int              `json:"typ"`
	Type   int              `json:"type"`
	ID     model.FlexString `json:"id"`
	PeerID model.FlexString `json:"peer_id"`
	UUID   string           `json:"uuid"`
	Info   string           `json:"info"`
}

// LogAlarm records a security or policy alarm event (/api/audit/alarm).
func (h *ClientAuditHandler) LogAlarm(c *gin.Context) {
	var req auditAlarmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid audit alarm payload"})
		return
	}

	peerID := strings.TrimSpace(req.PeerID.String())
	if peerID == "" {
		peerID = strings.TrimSpace(req.ID.String())
	}
	typ := req.Typ
	if typ == 0 {
		typ = req.Type
	}

	row := model.AuditAlarm{
		AlarmType: typ,
		PeerID:    peerID,
		UUID:      strings.TrimSpace(req.UUID),
		IP:        c.ClientIP(),
		Info:      req.Info,
	}
	h.db.Create(&row)
	c.JSON(http.StatusOK, gin.H{})
}
