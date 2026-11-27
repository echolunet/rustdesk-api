package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rustdesk-api/internal/middleware"
	"rustdesk-api/internal/model"
	"rustdesk-api/internal/service"
)

// AdminUserHandler manages users, user groups, and session token revocations from the admin console.
type AdminUserHandler struct {
	db      *gorm.DB
	authSvc *service.AuthService
	abSvc   *service.ABService
	totpSvc *service.TOTPService
}

// NewAdminUserHandler creates a new AdminUserHandler.
func NewAdminUserHandler(db *gorm.DB, authSvc *service.AuthService, abSvc *service.ABService, totpSvc *service.TOTPService) *AdminUserHandler {
	return &AdminUserHandler{db: db, authSvc: authSvc, abSvc: abSvc, totpSvc: totpSvc}
}

// ListUsers returns paginated users with optional keyword search.
func (h *AdminUserHandler) ListUsers(c *gin.Context) {
	current, pageSize := ParsePagination(c, 20)
	q := h.db.Model(&model.User{})
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("username LIKE ? OR email LIKE ? OR note LIKE ?", like, like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count users failed"})
		return
	}

	var users []model.User
	if err := q.Order("id desc").Offset((current - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query users failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "data": users})
}

type adminUserReq struct {
	Username           string `json:"username"`
	Email              string `json:"email"`
	Password           string `json:"password"`
	Note               string `json:"note"`
	Status             *int   `json:"status"`
	IsAdmin            *bool  `json:"is_admin"`
	EmailVerifyEnabled *bool  `json:"email_verify_enabled"`
	GroupID            uint   `json:"group_id"`
}

// CreateUser creates a new local user and provisions their personal address book.
func (h *AdminUserHandler) CreateUser(c *gin.Context) {
	var req adminUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password (min 6 chars) are required"})
		return
	}

	var cnt int64
	h.db.Model(&model.User{}).Where("username = ?", req.Username).Count(&cnt)
	if cnt > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}

	hash, err := h.authSvc.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash password failed"})
		return
	}

	grp, err := h.resolveGroup(req.GroupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group_id"})
		return
	}

	status := 1
	if req.Status != nil {
		status = *req.Status
	}
	isAdmin := false
	if req.IsAdmin != nil {
		isAdmin = *req.IsAdmin
	}
	emailVerify := false
	if req.EmailVerifyEnabled != nil {
		emailVerify = *req.EmailVerifyEnabled
	}

	user := model.User{
		Username:           req.Username,
		Email:              strings.TrimSpace(req.Email),
		PasswordHash:       hash,
		Note:               req.Note,
		Status:             status,
		IsAdmin:            isAdmin,
		EmailVerifyEnabled: emailVerify,
		GroupID:            grp.ID,
		GroupName:          grp.Name,
		AuthSource:         "local",
	}
	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create user failed"})
		return
	}
	_, _ = h.abSvc.EnsurePersonalCollection(&user)
	c.JSON(http.StatusCreated, user)
}

// UpdateUser updates an existing user's metadata, group, status, admin flag, or optional new password.
func (h *AdminUserHandler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	var user model.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var req adminUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	updates := map[string]interface{}{
		"email": strings.TrimSpace(req.Email),
		"note":  req.Note,
	}
	if req.Status != nil {
		if user.ID == middleware.CurrentUser(c).ID && *req.Status == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot disable your own active admin account"})
			return
		}
		updates["status"] = *req.Status
	}
	if req.IsAdmin != nil {
		if user.ID == middleware.CurrentUser(c).ID && !*req.IsAdmin {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove admin role from yourself"})
			return
		}
		updates["is_admin"] = *req.IsAdmin
	}
	if req.EmailVerifyEnabled != nil {
		updates["email_verify_enabled"] = *req.EmailVerifyEnabled
	}
	if req.GroupID > 0 {
		grp, err := h.resolveGroup(req.GroupID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group_id"})
			return
		}
		updates["group_id"] = grp.ID
		updates["group_name"] = grp.Name
	}
	if strings.TrimSpace(req.Password) != "" {
		if len(req.Password) < 6 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 6 characters"})
			return
		}
		hash, err := h.authSvc.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "hash password failed"})
			return
		}
		updates["password_hash"] = hash
	}

	if err := h.db.Model(&user).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update user failed"})
		return
	}
	if req.Status != nil && *req.Status == 0 {
		_ = h.authSvc.RevokeUserTokens(user.ID)
	} else {
		h.authSvc.InvalidateUserCache(user.ID)
	}
	_ = h.db.First(&user, user.ID).Error
	c.JSON(http.StatusOK, user)
}

// DeleteUser deletes a non-self user and revokes all their tokens.
func (h *AdminUserHandler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if uint(id) == middleware.CurrentUser(c).ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete your own account"})
		return
	}
	_ = h.authSvc.RevokeUserTokens(uint(id))
	h.db.Where("user_id = ?", id).Delete(&model.AddressBookRule{})
	h.db.Where("user_id = ?", id).Delete(&model.DeviceGroupAccess{})
	h.db.Delete(&model.User{}, id)
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// RevokeTokens invalidates all active JWT sessions for the target user.
func (h *AdminUserHandler) RevokeTokens(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if err := h.authSvc.RevokeUserTokens(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke tokens"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "tokens revoked"})
}

// ResetUser2FA clears TOTP 2FA and trusted devices for the target user.
func (h *AdminUserHandler) ResetUser2FA(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if err := h.totpSvc.ResetUser2FA(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset user 2FA"})
		return
	}
	h.authSvc.InvalidateUserCache(uint(id))
	c.JSON(http.StatusOK, gin.H{"message": "user 2FA reset"})
}

// ListGroups returns all user groups.
func (h *AdminUserHandler) ListGroups(c *gin.Context) {
	var groups []model.Group
	if err := h.db.Order("id asc").Find(&groups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query groups failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": groups})
}

// CreateGroup creates a user group.
func (h *AdminUserHandler) CreateGroup(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		Note string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group name is required"})
		return
	}
	grp := model.Group{Name: strings.TrimSpace(req.Name), Note: req.Note, Type: 1}
	if err := h.db.Create(&grp).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "group name already exists"})
		return
	}
	c.JSON(http.StatusCreated, grp)
}

// UpdateGroup updates a user group and syncs GroupName on member users.
func (h *AdminUserHandler) UpdateGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group id"})
		return
	}
	var grp model.Group
	if err := h.db.First(&grp, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	var req struct {
		Name string `json:"name"`
		Note string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group name is required"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if err := h.db.Model(&grp).Updates(map[string]interface{}{"name": name, "note": req.Note}).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "failed to update group"})
		return
	}
	h.db.Model(&model.User{}).Where("group_id = ?", grp.ID).Update("group_name", name)
	h.authSvc.ClearUserCache()
	c.JSON(http.StatusOK, grp)
}

// DeleteGroup deletes a non-default user group.
func (h *AdminUserHandler) DeleteGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group id"})
		return
	}
	var grp model.Group
	if err := h.db.First(&grp, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	if grp.Name == "default" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete default group"})
		return
	}
	defGrp, _ := h.authSvc.EnsureDefaultGroup()
	if defGrp != nil {
		h.db.Model(&model.User{}).Where("group_id = ?", grp.ID).Updates(map[string]interface{}{
			"group_id":   defGrp.ID,
			"group_name": defGrp.Name,
		})
	}
	h.db.Where("group_id = ?", grp.ID).Delete(&model.AddressBookRule{})
	h.db.Where("user_group_id = ?", grp.ID).Delete(&model.DeviceGroupAccess{})
	h.db.Delete(&grp)
	h.authSvc.ClearUserCache()
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *AdminUserHandler) resolveGroup(groupID uint) (*model.Group, error) {
	if groupID == 0 {
		return h.authSvc.EnsureDefaultGroup()
	}
	var grp model.Group
	if err := h.db.First(&grp, groupID).Error; err != nil {
		return nil, err
	}
	return &grp, nil
}
