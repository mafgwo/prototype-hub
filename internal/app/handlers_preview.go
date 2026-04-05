package app

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"prototypehub/internal/models"

	"github.com/gin-gonic/gin"
)

func (s *Server) handlePreviewRoot(c *gin.Context) {
	user := s.currentUser(c)
	var version models.ProjectVersion
	if err := s.db.First(&version, c.Param("versionID")).Error; err != nil {
		s.respondError(c, http.StatusNotFound, "version not found")
		return
	}
	if version.Status != models.VersionStatusReady {
		s.respondError(c, http.StatusBadRequest, "version is not ready")
		return
	}

	project, err := s.loadProject(fmt.Sprintf("%d", version.ProjectID))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canAccessProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("/preview/%d/%s", version.ID, version.EntryFile))
}

func (s *Server) handlePreview(c *gin.Context) {
	user := s.currentUser(c)
	var version models.ProjectVersion
	if err := s.db.First(&version, c.Param("versionID")).Error; err != nil {
		s.respondError(c, http.StatusNotFound, "version not found")
		return
	}
	if version.Status != models.VersionStatusReady {
		s.respondError(c, http.StatusBadRequest, "version is not ready")
		return
	}

	project, err := s.loadProject(fmt.Sprintf("%d", version.ProjectID))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canAccessProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}

	requested := strings.TrimPrefix(c.Param("assetPath"), "/")
	if requested == "" {
		requested = version.EntryFile
	}
	object, err := s.store.Get(c.Request.Context(), path.Join(version.ExtractPrefix, requested))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "preview asset not found")
		return
	}
	defer object.Body.Close()

	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; img-src 'self' data: blob:; media-src 'self' data: blob:; frame-ancestors 'none';")
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Type", object.ContentType)
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, object.Body)
}

func (s *Server) handleCurrentProjectPreview(c *gin.Context) {
	user := s.currentUser(c)
	var project models.Project
	if err := s.db.Preload("Members").Where("slug = ?", c.Param("slug")).First(&project).Error; err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canAccessProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}
	if project.CurrentVersionID == nil {
		s.respondError(c, http.StatusNotFound, "project has no current version")
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("/preview/%d", *project.CurrentVersionID))
}
