package db

import (
	"fmt"

	"prototypehub/internal/config"
	"prototypehub/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.DatabaseDriver {
	case "postgres", "postgresql":
		dialector = postgres.Open(cfg.DatabaseDSN)
	case "mysql":
		dialector = mysql.Open(cfg.DatabaseDSN)
	default:
		dialector = sqlite.Open(cfg.DatabaseDSN)
	}

	database, err := gorm.Open(dialector, &gorm.Config{
		// Project <-> current version is a circular reference.
		// For this PoC we keep integrity in application logic and
		// avoid FK creation during AutoMigrate so PostgreSQL can boot cleanly.
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := database.AutoMigrate(
		&models.User{},
		&models.Role{},
		&models.Project{},
		&models.ProjectVersion{},
		&models.ProjectMember{},
		&models.AuditLog{},
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	return database, nil
}
