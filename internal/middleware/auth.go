package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"rustdesk-api/internal/model"
	"rustdesk-api/internal/service"
)

const (
	contextUserKey = "auth_user"
	contextJTIKey  = "auth_jti"
)

// AuthRequired validates the Bearer JWT token and injects the authenticated user into the Gin context.
func AuthRequired(authSvc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization scheme"})
			return
		}

		user, jti, err := authSvc.ValidateToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		c.Set(contextUserKey, user)
		c.Set(contextJTIKey, jti)
		c.Next()
	}
}

// AdminRequired verifies that the authenticated user has administrator privileges.
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user == nil || !user.IsAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "administrator privileges required"})
			return
		}
		c.Next()
	}
}

// CurrentUser extracts the authenticated User from the Gin context.
func CurrentUser(c *gin.Context) *model.User {
	val, exists := c.Get(contextUserKey)
	if !exists {
		return nil
	}
	user, _ := val.(*model.User)
	return user
}

// CurrentJTI extracts the current JWT token ID from the Gin context.
func CurrentJTI(c *gin.Context) string {
	val, exists := c.Get(contextJTIKey)
	if !exists {
		return ""
	}
	jti, _ := val.(string)
	return jti
}
