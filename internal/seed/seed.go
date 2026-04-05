package seed

import (
	"fmt"

	"prototypehub/internal/auth"
	"prototypehub/internal/config"
	"prototypehub/internal/models"

	"gorm.io/gorm"
)

func Run(database *gorm.DB, cfg config.Config) error {
	roles := []models.Role{
		{Code: models.SystemRoleAdmin, Name: "Administrator"},
		{Code: models.SystemRoleProjectAdmin, Name: "Project Administrator"},
		{Code: models.SystemRoleViewer, Name: "Viewer"},
	}
	for _, role := range roles {
		if err := database.FirstOrCreate(&models.Role{}, models.Role{Code: role.Code, Name: role.Name}).Error; err != nil {
			return fmt.Errorf("seed role %s: %w", role.Code, err)
		}
	}

	var legacyUserRole models.Role
	if err := database.Where("code = ?", "user").First(&legacyUserRole).Error; err == nil {
		var viewerRole models.Role
		if err := database.Where("code = ?", models.SystemRoleViewer).First(&viewerRole).Error; err != nil {
			return err
		}
		if err := database.Exec(`
			INSERT INTO user_roles (user_id, role_id)
			SELECT ur.user_id, ?
			FROM user_roles ur
			WHERE ur.role_id = ?
			AND NOT EXISTS (
				SELECT 1 FROM user_roles existing
				WHERE existing.user_id = ur.user_id AND existing.role_id = ?
			)
		`, viewerRole.ID, legacyUserRole.ID, viewerRole.ID).Error; err != nil {
			return fmt.Errorf("migrate legacy user role to viewer: %w", err)
		}
		if err := database.Exec("DELETE FROM user_roles WHERE role_id = ?", legacyUserRole.ID).Error; err != nil {
			return fmt.Errorf("clear legacy user role mappings: %w", err)
		}
		if err := database.Delete(&legacyUserRole).Error; err != nil {
			return fmt.Errorf("delete legacy user role: %w", err)
		}
	}

	var count int64
	if err := database.Model(&models.User{}).Where("username = ?", cfg.DefaultAdminUsername).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	passwordHash, err := auth.HashPassword(cfg.DefaultAdminPassword)
	if err != nil {
		return err
	}
	admin := models.User{
		Username:     cfg.DefaultAdminUsername,
		PasswordHash: passwordHash,
		DisplayName:  cfg.DefaultAdminDisplay,
		Status:       models.UserStatusActive,
	}
	if err := database.Create(&admin).Error; err != nil {
		return err
	}

	var role models.Role
	if err := database.Where("code = ?", models.SystemRoleAdmin).First(&role).Error; err != nil {
		return err
	}
	return database.Model(&admin).Association("Roles").Append(&role)
}
