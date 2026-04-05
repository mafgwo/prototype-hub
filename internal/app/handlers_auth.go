package app

import (
	"net/http"

	"prototypehub/internal/auth"
	"prototypehub/internal/models"

	"github.com/gin-gonic/gin"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changeMyPasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (s *Server) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.respondError(c, http.StatusBadRequest, "invalid login payload")
		return
	}

	var user models.User
	if err := s.db.Preload("Roles").Where("username = ?", req.Username).First(&user).Error; err != nil {
		s.respondError(c, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if user.Status != models.UserStatusActive || !auth.CheckPassword(user.PasswordHash, req.Password) {
		s.respondError(c, http.StatusUnauthorized, "invalid username or password")
		return
	}

	token, expiresAt, err := auth.IssueToken(s.cfg, user)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to create session")
		return
	}
	s.setAuthCookie(c, token, expiresAt)
	s.createAuditLog(&user.ID, "auth.login", "user", "self", "user logged in")

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":          user.ID,
			"username":    user.Username,
			"displayName": user.DisplayName,
			"roles":       roleCodes(user),
		},
	})
}

func (s *Server) handleLogout(c *gin.Context) {
	user := s.currentUser(c)
	s.clearAuthCookie(c)
	if user != nil {
		s.createAuditLog(&user.ID, "auth.logout", "user", "self", "user logged out")
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleMe(c *gin.Context) {
	user := s.currentUser(c)
	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":          user.ID,
			"username":    user.Username,
			"displayName": user.DisplayName,
			"status":      user.Status,
			"roles":       roleCodes(*user),
		},
	})
}

func (s *Server) handleChangeMyPassword(c *gin.Context) {
	user := s.currentUser(c)

	var req changeMyPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.respondError(c, http.StatusBadRequest, "invalid password payload")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		s.respondError(c, http.StatusBadRequest, "currentPassword and newPassword are required")
		return
	}
	if len(req.NewPassword) < 8 {
		s.respondError(c, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		s.respondError(c, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	if req.CurrentPassword == req.NewPassword {
		s.respondError(c, http.StatusBadRequest, "new password must be different from current password")
		return
	}

	passwordHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := s.db.Model(&models.User{}).Where("id = ?", user.ID).Update("password_hash", passwordHash).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to update password")
		return
	}

	user.PasswordHash = passwordHash
	s.createAuditLog(&user.ID, "auth.password.change", "user", "self", "user changed own password")
	c.JSON(http.StatusOK, gin.H{"success": true})
}
