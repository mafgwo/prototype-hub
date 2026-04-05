package app

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"prototypehub/internal/auth"
	"prototypehub/internal/config"
	"prototypehub/internal/db"
	"prototypehub/internal/models"
	"prototypehub/internal/seed"
	"prototypehub/internal/storage"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

//go:embed web/templates/* web/static/*
var webAssets embed.FS

type Server struct {
	cfg       config.Config
	db        *gorm.DB
	store     storage.Storage
	templates *template.Template
}

const userContextKey = "currentUser"

func Run() error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := ensureRuntimePaths(cfg); err != nil {
		return err
	}

	database, err := db.Open(cfg)
	if err != nil {
		return err
	}
	if err := seed.Run(database, cfg); err != nil {
		return err
	}

	store, err := buildStorage(cfg)
	if err != nil {
		return err
	}

	templateFS, err := fs.Sub(webAssets, "web/templates")
	if err != nil {
		return err
	}
	templates, err := template.ParseFS(templateFS, "*.html")
	if err != nil {
		return err
	}

	server := &Server{cfg: cfg, db: database, store: store, templates: templates}

	engine := gin.Default()
	engine.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	engine.MaxMultipartMemory = cfg.UploadMaxSizeBytes

	staticFS, err := fs.Sub(webAssets, "web/static")
	if err != nil {
		return err
	}
	engine.StaticFS("/assets", http.FS(staticFS))

	engine.GET("/", server.handleIndex)
	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := engine.Group("/api")
	api.POST("/auth/login", server.handleLogin)
	api.POST("/auth/logout", server.authMiddleware(), server.handleLogout)

	secured := api.Group("/")
	secured.Use(server.authMiddleware())
	secured.GET("/me", server.handleMe)
	secured.POST("/me/password", server.handleChangeMyPassword)
	secured.GET("/users", server.requireRole(models.SystemRoleAdmin, models.SystemRoleProjectAdmin), server.handleProjectVisibleUsers)
	secured.GET("/member-candidates", server.requireRole(models.SystemRoleAdmin, models.SystemRoleProjectAdmin), server.handleMemberCandidates)
	secured.GET("/projects", server.handleProjectList)
	secured.POST("/projects", server.requireRole(models.SystemRoleAdmin, models.SystemRoleProjectAdmin), server.handleCreateProject)
	secured.GET("/projects/:id", server.handleProjectDetail)
	secured.PATCH("/projects/:id", server.requireRole(models.SystemRoleAdmin, models.SystemRoleProjectAdmin), server.handleUpdateProject)
	secured.DELETE("/projects/:id", server.requireRole(models.SystemRoleAdmin), server.handleDeleteProject)
	secured.GET("/projects/:id/versions", server.handleProjectVersions)
	secured.GET("/projects/:id/versions/:versionID", server.handleProjectVersionDetail)
	secured.POST("/projects/:id/versions/upload", server.requireRole(models.SystemRoleAdmin, models.SystemRoleProjectAdmin), server.handleVersionUpload)
	secured.DELETE("/projects/:id/versions/:versionID", server.requireRole(models.SystemRoleAdmin, models.SystemRoleProjectAdmin), server.handleDeleteProjectVersion)
	secured.POST("/projects/:id/versions/:versionID/label", server.requireRole(models.SystemRoleAdmin, models.SystemRoleProjectAdmin), server.handleUpdateVersionLabel)
	secured.POST("/projects/:id/versions/:versionID/entry-file", server.requireRole(models.SystemRoleAdmin, models.SystemRoleProjectAdmin), server.handleUpdateVersionEntryFile)
	secured.POST("/projects/:id/current-version", server.requireRole(models.SystemRoleAdmin, models.SystemRoleProjectAdmin), server.handleSwitchCurrentVersion)

	admin := api.Group("/admin")
	admin.Use(server.authMiddleware())
	admin.GET("/audit-logs", server.requireRole(models.SystemRoleAdmin, models.SystemRoleProjectAdmin), server.handleAdminAuditLogs)
	admin.POST("/projects/:id/members", server.requireRole(models.SystemRoleAdmin, models.SystemRoleProjectAdmin), server.handleAdminReplaceProjectMembers)
	admin.GET("/users", server.requireRole(models.SystemRoleAdmin), server.handleAdminUsers)
	admin.POST("/users", server.requireRole(models.SystemRoleAdmin), server.handleAdminCreateUser)
	admin.PATCH("/users/:id/status", server.requireRole(models.SystemRoleAdmin), server.handleAdminPatchUserStatus)
	admin.POST("/users/:id/reset-password", server.requireRole(models.SystemRoleAdmin), server.handleAdminResetPassword)
	admin.POST("/users/:id/roles", server.requireRole(models.SystemRoleAdmin), server.handleAdminAssignRoles)

	engine.GET("/preview/:versionID", server.authMiddleware(), server.handlePreviewRoot)
	engine.GET("/preview/:versionID/*assetPath", server.authMiddleware(), server.handlePreview)
	engine.GET("/p/:slug", server.authMiddleware(), server.handleCurrentProjectPreview)

	return engine.Run(cfg.HTTPAddr)
}

func ensureRuntimePaths(cfg config.Config) error {
	if strings.EqualFold(cfg.DatabaseDriver, "sqlite") {
		sqlitePath := strings.TrimPrefix(cfg.DatabaseDSN, "file:")
		if index := strings.Index(sqlitePath, "?"); index >= 0 {
			sqlitePath = sqlitePath[:index]
		}
		if sqlitePath != "" && sqlitePath != ":memory:" {
			if err := os.MkdirAll(filepath.Dir(sqlitePath), 0o755); err != nil {
				return err
			}
		}
	}
	if strings.EqualFold(cfg.StorageDriver, "local") {
		if err := os.MkdirAll(cfg.LocalStoragePath, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func buildStorage(cfg config.Config) (storage.Storage, error) {
	if strings.EqualFold(cfg.StorageDriver, "s3") {
		return storage.NewS3(context.Background(), cfg)
	}
	return storage.NewLocal(cfg.LocalStoragePath)
}

func (s *Server) handleIndex(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	_ = s.templates.ExecuteTemplate(c.Writer, "index.html", map[string]any{"AppName": s.cfg.AppName})
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(s.cfg.JWTCookieName)
		if err != nil || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		claims, err := auth.ParseToken(s.cfg, token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			return
		}
		var user models.User
		if err := s.db.Preload("Roles").First(&user, claims.UserID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}
		if user.Status != models.UserStatusActive {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user disabled"})
			return
		}
		c.Set(userContextKey, &user)
		c.Next()
	}
}

func (s *Server) requireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := s.currentUser(c)
		for _, role := range user.Roles {
			for _, required := range roles {
				if role.Code == required {
					c.Next()
					return
				}
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "permission denied"})
	}
}

func (s *Server) currentUser(c *gin.Context) *models.User {
	value, ok := c.Get(userContextKey)
	if !ok {
		return nil
	}
	user, _ := value.(*models.User)
	return user
}

func (s *Server) setAuthCookie(c *gin.Context, token string, expiresAt time.Time) {
	c.SetCookie(s.cfg.JWTCookieName, token, int(time.Until(expiresAt).Seconds()), "/", "", s.cfg.IsProduction(), true)
}

func (s *Server) clearAuthCookie(c *gin.Context) {
	c.SetCookie(s.cfg.JWTCookieName, "", -1, "/", "", s.cfg.IsProduction(), true)
}

func (s *Server) respondError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

func (s *Server) createAuditLog(actorID *uint, action, targetType, targetID, detail string) {
	_ = s.db.Create(&models.AuditLog{
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     detail,
	}).Error
}

func hasSystemRole(user *models.User, roleCode string) bool {
	if user == nil {
		return false
	}
	for _, role := range user.Roles {
		if role.Code == roleCode {
			return true
		}
	}
	return false
}

func canManageProjectsSystem(user *models.User) bool {
	return hasSystemRole(user, models.SystemRoleAdmin) || hasSystemRole(user, models.SystemRoleProjectAdmin)
}

func (s *Server) canAccessProject(user *models.User, project models.Project) bool {
	if user == nil {
		return false
	}
	if canManageProjectsSystem(user) || project.OwnerID == user.ID {
		return true
	}
	for _, member := range project.Members {
		if member.UserID == user.ID {
			return true
		}
	}
	return false
}

func (s *Server) canEditProject(user *models.User, project models.Project) bool {
	if user == nil {
		return false
	}
	if canManageProjectsSystem(user) {
		return true
	}
	return false
}

func (s *Server) loadProject(projectID string) (models.Project, error) {
	var project models.Project
	err := s.db.Preload("Owner").Preload("Members.User").Preload("CurrentVersion").First(&project, projectID).Error
	return project, err
}

func roleCodes(user models.User) []string {
	codes := make([]string, 0, len(user.Roles))
	for _, role := range user.Roles {
		codes = append(codes, role.Code)
	}
	return codes
}

func slugify(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var builder strings.Builder
	prevDash := false
	for _, r := range input {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
			prevDash = false
		case !prevDash:
			builder.WriteRune('-')
			prevDash = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return fmt.Sprintf("project-%d", time.Now().Unix())
	}
	return slug
}
