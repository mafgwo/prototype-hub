package app

import (
	"fmt"
	"net/http"
	"strconv"

	"prototypehub/internal/auth"
	"prototypehub/internal/models"

	"github.com/gin-gonic/gin"
)

type createUserRequest struct {
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	DisplayName string   `json:"displayName"`
	Roles       []string `json:"roles"`
}

type patchUserStatusRequest struct {
	Status string `json:"status"`
}

type assignRolesRequest struct {
	Roles []string `json:"roles"`
}

type resetPasswordRequest struct {
	Password string `json:"password"`
}

type replaceProjectMembersRequest struct {
	UserIDs []uint `json:"userIds"`
}

func (s *Server) handleAdminUsers(c *gin.Context) {
	var users []models.User
	if err := s.db.Preload("Roles").Order("created_at desc").Find(&users).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to load users")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": users})
}

func (s *Server) handleAdminCreateUser(c *gin.Context) {
	actor := s.currentUser(c)
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" || req.DisplayName == "" {
		s.respondError(c, http.StatusBadRequest, "username, password and displayName are required")
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user := models.User{
		Username:     req.Username,
		PasswordHash: passwordHash,
		DisplayName:  req.DisplayName,
		Status:       models.UserStatusActive,
	}
	if err := s.db.Create(&user).Error; err != nil {
		s.respondError(c, http.StatusConflict, "failed to create user")
		return
	}
	if len(req.Roles) == 0 {
		req.Roles = []string{models.SystemRoleViewer}
	}
	if err := s.assignRoles(user.ID, req.Roles); err != nil {
		s.respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	s.createAuditLog(&actor.ID, "admin.user.create", "user", fmt.Sprintf("%d", user.ID), req.Username)
	c.JSON(http.StatusCreated, gin.H{"user": user})
}

func (s *Server) handleAdminPatchUserStatus(c *gin.Context) {
	actor := s.currentUser(c)
	userID, _ := strconv.Atoi(c.Param("id"))
	var req patchUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.respondError(c, http.StatusBadRequest, "invalid status payload")
		return
	}
	if req.Status != models.UserStatusActive && req.Status != models.UserStatusDisabled {
		s.respondError(c, http.StatusBadRequest, "invalid status")
		return
	}
	if actor != nil && actor.ID == uint(userID) && req.Status == models.UserStatusDisabled {
		s.respondError(c, http.StatusBadRequest, "you cannot disable your own account")
		return
	}
	if err := s.db.Model(&models.User{}).Where("id = ?", userID).Update("status", req.Status).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to update user")
		return
	}
	s.createAuditLog(&actor.ID, "admin.user.status", "user", c.Param("id"), req.Status)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleAdminAssignRoles(c *gin.Context) {
	actor := s.currentUser(c)
	userID, _ := strconv.Atoi(c.Param("id"))
	var req assignRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Roles) == 0 {
		s.respondError(c, http.StatusBadRequest, "roles are required")
		return
	}
	if actor != nil && actor.ID == uint(userID) {
		keepsAdmin := false
		for _, role := range req.Roles {
			if role == models.SystemRoleAdmin {
				keepsAdmin = true
				break
			}
		}
		if !keepsAdmin {
			s.respondError(c, http.StatusBadRequest, "you cannot remove your own admin role")
			return
		}
	}
	if err := s.assignRoles(uint(userID), req.Roles); err != nil {
		s.respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	s.createAuditLog(&actor.ID, "admin.user.roles", "user", c.Param("id"), fmt.Sprintf("%v", req.Roles))
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleAdminResetPassword(c *gin.Context) {
	actor := s.currentUser(c)
	userID, _ := strconv.Atoi(c.Param("id"))
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Password == "" {
		s.respondError(c, http.StatusBadRequest, "password is required")
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := s.db.Model(&models.User{}).Where("id = ?", userID).Update("password_hash", passwordHash).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to reset password")
		return
	}
	s.createAuditLog(&actor.ID, "admin.user.reset_password", "user", c.Param("id"), "password reset")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleAdminReplaceProjectMembers(c *gin.Context) {
	actor := s.currentUser(c)
	project, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}

	var req replaceProjectMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.respondError(c, http.StatusBadRequest, "invalid project members payload")
		return
	}

	tx := s.db.Begin()
	if tx.Error != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to start transaction")
		return
	}

	if err := tx.Where("project_id = ? AND user_id <> ?", project.ID, project.OwnerID).Delete(&models.ProjectMember{}).Error; err != nil {
		tx.Rollback()
		s.respondError(c, http.StatusInternalServerError, "failed to clear project members")
		return
	}

	for _, userID := range req.UserIDs {
		if userID == 0 || userID == project.OwnerID {
			continue
		}
		member := models.ProjectMember{
			ProjectID: project.ID,
			UserID:    userID,
			Role:      models.ProjectRoleViewer,
		}
		if err := tx.Create(&member).Error; err != nil {
			tx.Rollback()
			s.respondError(c, http.StatusInternalServerError, "failed to save project members")
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to commit project members")
		return
	}

	s.createAuditLog(&actor.ID, "admin.project.members", "project", c.Param("id"), fmt.Sprintf("userIds=%v", req.UserIDs))
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleAdminAuditLogs(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if err != nil || pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var total int64
	if err := s.db.Model(&models.AuditLog{}).Count(&total).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to count audit logs")
		return
	}

	var logs []models.AuditLog
	if err := s.db.Preload("Actor").
		Order("created_at desc").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&logs).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to load audit logs")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":    logs,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func (s *Server) assignRoles(userID uint, codes []string) error {
	var user models.User
	if err := s.db.Preload("Roles").First(&user, userID).Error; err != nil {
		return fmt.Errorf("user not found")
	}
	var roles []models.Role
	if err := s.db.Where("code IN ?", codes).Find(&roles).Error; err != nil {
		return fmt.Errorf("load roles: %w", err)
	}
	if len(roles) != len(codes) {
		return fmt.Errorf("one or more roles are invalid")
	}
	return s.db.Model(&user).Association("Roles").Replace(&roles)
}
