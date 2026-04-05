package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"time"

	"prototypehub/internal/models"
	"prototypehub/internal/prototype"

	"github.com/gin-gonic/gin"
)

type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type switchVersionRequest struct {
	VersionID uint `json:"versionId"`
}

type updateEntryFileRequest struct {
	EntryFile string `json:"entryFile"`
}

type updateVersionLabelRequest struct {
	Label string `json:"label"`
}

func versionSummary(version models.ProjectVersion) models.ProjectVersion {
	version.HTMLFiles = nil
	return version
}

func versionSummaries(versions []models.ProjectVersion) []models.ProjectVersion {
	items := make([]models.ProjectVersion, 0, len(versions))
	for _, version := range versions {
		items = append(items, versionSummary(version))
	}
	return items
}

func (s *Server) handleProjectList(c *gin.Context) {
	user := s.currentUser(c)
	var projects []models.Project
	query := s.db.Preload("Owner").Preload("CurrentVersion")
	if !canManageProjectsSystem(user) {
		query = query.
			Joins("LEFT JOIN project_members ON project_members.project_id = projects.id").
			Where("projects.owner_id = ? OR project_members.user_id = ?", user.ID, user.ID).
			Group("projects.id")
	}
	if err := query.Order("projects.created_at desc").Find(&projects).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to load projects")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": projects})
}

func (s *Server) handleCreateProject(c *gin.Context) {
	user := s.currentUser(c)
	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		s.respondError(c, http.StatusBadRequest, "project name is required")
		return
	}

	project := models.Project{
		Name:        req.Name,
		Slug:        slugify(req.Name),
		Description: req.Description,
		OwnerID:     user.ID,
		Status:      models.ProjectStatusActive,
	}
	if err := s.db.Create(&project).Error; err != nil {
		s.respondError(c, http.StatusConflict, "failed to create project")
		return
	}
	member := models.ProjectMember{ProjectID: project.ID, UserID: user.ID, Role: models.ProjectRoleEditor}
	_ = s.db.FirstOrCreate(&member, member).Error
	s.createAuditLog(&user.ID, "project.create", "project", fmt.Sprintf("%d", project.ID), req.Name)
	c.JSON(http.StatusCreated, gin.H{"project": project})
}

func (s *Server) handleProjectDetail(c *gin.Context) {
	user := s.currentUser(c)
	project, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canAccessProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}

	var versions []models.ProjectVersion
	if err := s.db.Where("project_id = ?", project.ID).Order("version_no desc").Find(&versions).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to load project versions")
		return
	}
	project.Versions = versionSummaries(versions)
	if project.CurrentVersion != nil {
		summary := versionSummary(*project.CurrentVersion)
		project.CurrentVersion = &summary
	}

	c.JSON(http.StatusOK, gin.H{"project": project})
}

func (s *Server) handleDeleteProject(c *gin.Context) {
	user := s.currentUser(c)
	project, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canEditProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}

	var versions []models.ProjectVersion
	if err := s.db.Where("project_id = ?", project.ID).Find(&versions).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to load project versions")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	for _, version := range versions {
		if version.ZipObjectKey != "" {
			if err := s.store.Delete(ctx, version.ZipObjectKey); err != nil {
				s.respondError(c, http.StatusInternalServerError, "failed to delete version zip from storage")
				return
			}
		}
		if version.ExtractPrefix != "" {
			if err := s.store.DeletePrefix(ctx, version.ExtractPrefix); err != nil {
				s.respondError(c, http.StatusInternalServerError, "failed to delete version files from storage")
				return
			}
		}
	}

	tx := s.db.Begin()
	if tx.Error != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to start transaction")
		return
	}

	if err := tx.Where("project_id = ?", project.ID).Delete(&models.ProjectMember{}).Error; err != nil {
		tx.Rollback()
		s.respondError(c, http.StatusInternalServerError, "failed to delete project members")
		return
	}
	if err := tx.Where("project_id = ?", project.ID).Delete(&models.ProjectVersion{}).Error; err != nil {
		tx.Rollback()
		s.respondError(c, http.StatusInternalServerError, "failed to delete project versions")
		return
	}
	if err := tx.Delete(&models.Project{}, project.ID).Error; err != nil {
		tx.Rollback()
		s.respondError(c, http.StatusInternalServerError, "failed to delete project")
		return
	}
	if err := tx.Commit().Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to commit project deletion")
		return
	}

	s.createAuditLog(&user.ID, "project.delete", "project", fmt.Sprintf("%d", project.ID), project.Name)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleProjectVersions(c *gin.Context) {
	user := s.currentUser(c)
	project, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canAccessProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}
	var versions []models.ProjectVersion
	if err := s.db.Where("project_id = ?", project.ID).Order("version_no desc").Find(&versions).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to load versions")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": versionSummaries(versions)})
}

func (s *Server) handleDeleteProjectVersion(c *gin.Context) {
	user := s.currentUser(c)
	project, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canEditProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}

	var version models.ProjectVersion
	if err := s.db.Where("id = ? AND project_id = ?", c.Param("versionID"), project.ID).First(&version).Error; err != nil {
		s.respondError(c, http.StatusNotFound, "version not found")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	if version.ZipObjectKey != "" {
		if err := s.store.Delete(ctx, version.ZipObjectKey); err != nil {
			s.respondError(c, http.StatusInternalServerError, "failed to delete version zip from storage")
			return
		}
	}
	if version.ExtractPrefix != "" {
		if err := s.store.DeletePrefix(ctx, version.ExtractPrefix); err != nil {
			s.respondError(c, http.StatusInternalServerError, "failed to delete version files from storage")
			return
		}
	}

	tx := s.db.Begin()
	if tx.Error != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to start transaction")
		return
	}

	if err := tx.Delete(&models.ProjectVersion{}, version.ID).Error; err != nil {
		tx.Rollback()
		s.respondError(c, http.StatusInternalServerError, "failed to delete project version")
		return
	}

	wasCurrent := project.CurrentVersionID != nil && *project.CurrentVersionID == version.ID
	if wasCurrent {
		var nextVersion models.ProjectVersion
		var nextVersionID *uint
		if err := tx.
			Where("project_id = ? AND status = ?", project.ID, models.VersionStatusReady).
			Order("version_no desc").
			First(&nextVersion).Error; err == nil {
			nextVersionID = &nextVersion.ID
		}
		if err := tx.Model(&models.Project{}).Where("id = ?", project.ID).Update("current_version_id", nextVersionID).Error; err != nil {
			tx.Rollback()
			s.respondError(c, http.StatusInternalServerError, "failed to update current project version")
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to commit version deletion")
		return
	}

	s.createAuditLog(&user.ID, "version.delete", "project_version", fmt.Sprintf("%d", version.ID), fmt.Sprintf("project=%d version=%d", project.ID, version.VersionNo))

	updatedProject, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to reload project after deletion")
		return
	}
	var versions []models.ProjectVersion
	if err := s.db.Where("project_id = ?", updatedProject.ID).Order("version_no desc").Find(&versions).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to reload project versions")
		return
	}
	updatedProject.Versions = versionSummaries(versions)
	if updatedProject.CurrentVersion != nil {
		summary := versionSummary(*updatedProject.CurrentVersion)
		updatedProject.CurrentVersion = &summary
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"project": updatedProject,
	})
}

func (s *Server) handleMemberCandidates(c *gin.Context) {
	items, err := s.loadProjectAssignableUsers()
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to load member candidates")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) handleProjectVisibleUsers(c *gin.Context) {
	items, err := s.loadProjectAssignableUsers()
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to load users")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) loadProjectAssignableUsers() ([]gin.H, error) {
	var users []models.User
	if err := s.db.Preload("Roles").Order("created_at desc").Find(&users).Error; err != nil {
		return nil, err
	}

	items := make([]gin.H, 0, len(users))
	for _, user := range users {
		if hasSystemRole(&user, models.SystemRoleAdmin) || hasSystemRole(&user, models.SystemRoleProjectAdmin) {
			continue
		}
		items = append(items, gin.H{
			"id":          user.ID,
			"username":    user.Username,
			"displayName": user.DisplayName,
			"status":      user.Status,
			"roles":       roleCodes(user),
		})
	}
	return items, nil
}

func (s *Server) handleProjectVersionDetail(c *gin.Context) {
	user := s.currentUser(c)
	project, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canAccessProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}

	var version models.ProjectVersion
	if err := s.db.Where("id = ? AND project_id = ?", c.Param("versionID"), project.ID).First(&version).Error; err != nil {
		s.respondError(c, http.StatusNotFound, "version not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{"version": version})
}

func (s *Server) handleVersionUpload(c *gin.Context) {
	user := s.currentUser(c)
	project, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canEditProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		s.respondError(c, http.StatusBadRequest, "zip file is required")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		s.respondError(c, http.StatusBadRequest, "failed to open upload")
		return
	}
	defer file.Close()

	zipData, err := io.ReadAll(io.LimitReader(file, s.cfg.UploadMaxSizeBytes+1))
	if err != nil {
		s.respondError(c, http.StatusBadRequest, "failed to read upload")
		return
	}
	if int64(len(zipData)) > s.cfg.UploadMaxSizeBytes {
		s.respondError(c, http.StatusBadRequest, "zip file exceeds max size")
		return
	}

	var latest models.ProjectVersion
	versionNo := 1
	if err := s.db.Where("project_id = ?", project.ID).Order("version_no desc").Limit(1).Find(&latest).Error; err == nil && latest.ID != 0 {
		versionNo = latest.VersionNo + 1
	}

	version := models.ProjectVersion{
		ProjectID:     project.ID,
		VersionNo:     versionNo,
		Status:        models.VersionStatusProcessing,
		ZipObjectKey:  path.Join("projects", strconv.Itoa(int(project.ID)), "versions", strconv.Itoa(versionNo), "upload.zip"),
		ExtractPrefix: path.Join("projects", strconv.Itoa(int(project.ID)), "versions", strconv.Itoa(versionNo), "public"),
		EntryFile:     "index.html",
		UploadUserID:  user.ID,
	}
	if err := s.db.Create(&version).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to create version")
		return
	}

	s.createAuditLog(&user.ID, "version.upload.requested", "project_version", fmt.Sprintf("%d", version.ID), fileHeader.Filename)
	go s.processVersionUpload(project.ID, version.ID, zipData, user.ID)

	c.JSON(http.StatusAccepted, gin.H{
		"versionId": version.ID,
		"status":    version.Status,
	})
}

func (s *Server) processVersionUpload(projectID, versionID uint, zipData []byte, actorID uint) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var version models.ProjectVersion
	if err := s.db.First(&version, versionID).Error; err != nil {
		return
	}

	result, err := prototype.ProcessZip(ctx, s.cfg, s.store, zipData, version.ZipObjectKey, version.ExtractPrefix)
	if err != nil {
		_ = s.db.Model(&models.ProjectVersion{}).Where("id = ?", version.ID).Updates(map[string]any{
			"status":        models.VersionStatusFailed,
			"error_message": err.Error(),
		}).Error
		s.createAuditLog(&actorID, "version.upload.failed", "project_version", fmt.Sprintf("%d", version.ID), err.Error())
		return
	}

	_ = s.db.Model(&models.ProjectVersion{}).Where("id = ?", version.ID).Updates(map[string]any{
		"status":         models.VersionStatusReady,
		"entry_file":     result.EntryFile,
		"html_files":     models.StringList(result.HTMLFiles),
		"zip_object_key": result.ZipKey,
		"extract_prefix": result.PublicKey,
		"error_message":  "",
	}).Error

	var project models.Project
	if err := s.db.First(&project, projectID).Error; err == nil && project.CurrentVersionID == nil {
		_ = s.db.Model(&models.Project{}).Where("id = ?", project.ID).Update("current_version_id", version.ID).Error
	}
	s.createAuditLog(&actorID, "version.upload.ready", "project_version", fmt.Sprintf("%d", version.ID), "version published")
}

func (s *Server) handleSwitchCurrentVersion(c *gin.Context) {
	user := s.currentUser(c)
	project, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canEditProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}

	var req switchVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.VersionID == 0 {
		s.respondError(c, http.StatusBadRequest, "versionId is required")
		return
	}

	var version models.ProjectVersion
	if err := s.db.Where("id = ? AND project_id = ?", req.VersionID, project.ID).First(&version).Error; err != nil {
		s.respondError(c, http.StatusNotFound, "version not found")
		return
	}
	if version.Status != models.VersionStatusReady {
		s.respondError(c, http.StatusBadRequest, "only ready versions can be set as current")
		return
	}

	result := s.db.Model(&models.Project{}).Where("id = ?", project.ID).Update("current_version_id", version.ID)
	if result.Error != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to switch version")
		return
	}
	if result.RowsAffected == 0 {
		s.respondError(c, http.StatusInternalServerError, "project current version was not updated")
		return
	}
	s.createAuditLog(&user.ID, "project.switch_current_version", "project", fmt.Sprintf("%d", project.ID), fmt.Sprintf("version=%d", version.ID))

	updatedProject, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to reload project after switch")
		return
	}
	var versions []models.ProjectVersion
	if err := s.db.Where("project_id = ?", updatedProject.ID).Order("version_no desc").Find(&versions).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to reload project versions")
		return
	}
	updatedProject.Versions = versionSummaries(versions)
	if updatedProject.CurrentVersion != nil {
		summary := versionSummary(*updatedProject.CurrentVersion)
		updatedProject.CurrentVersion = &summary
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"project": updatedProject,
	})
}

func (s *Server) handleUpdateVersionEntryFile(c *gin.Context) {
	user := s.currentUser(c)
	project, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canEditProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}

	var version models.ProjectVersion
	if err := s.db.Where("id = ? AND project_id = ?", c.Param("versionID"), project.ID).First(&version).Error; err != nil {
		s.respondError(c, http.StatusNotFound, "version not found")
		return
	}
	if version.Status != models.VersionStatusReady {
		s.respondError(c, http.StatusBadRequest, "only ready versions can update entry file")
		return
	}

	var req updateEntryFileRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.EntryFile == "" {
		s.respondError(c, http.StatusBadRequest, "entryFile is required")
		return
	}

	allowed := false
	for _, htmlFile := range version.HTMLFiles {
		if htmlFile == req.EntryFile {
			allowed = true
			break
		}
	}
	if !allowed {
		s.respondError(c, http.StatusBadRequest, "entry file is not in this version")
		return
	}

	if err := s.db.Model(&version).Update("entry_file", req.EntryFile).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to update entry file")
		return
	}
	s.createAuditLog(&user.ID, "version.entry_file.update", "project_version", fmt.Sprintf("%d", version.ID), req.EntryFile)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleUpdateVersionLabel(c *gin.Context) {
	user := s.currentUser(c)
	project, err := s.loadProject(c.Param("id"))
	if err != nil {
		s.respondError(c, http.StatusNotFound, "project not found")
		return
	}
	if !s.canEditProject(user, project) {
		s.respondError(c, http.StatusForbidden, "permission denied")
		return
	}

	var version models.ProjectVersion
	if err := s.db.Where("id = ? AND project_id = ?", c.Param("versionID"), project.ID).First(&version).Error; err != nil {
		s.respondError(c, http.StatusNotFound, "version not found")
		return
	}

	var req updateVersionLabelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.respondError(c, http.StatusBadRequest, "invalid version label payload")
		return
	}
	if len(req.Label) > 120 {
		s.respondError(c, http.StatusBadRequest, "label must be 120 characters or fewer")
		return
	}

	if err := s.db.Model(&version).Update("label", req.Label).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "failed to update version label")
		return
	}

	s.createAuditLog(&user.ID, "version.label.update", "project_version", fmt.Sprintf("%d", version.ID), req.Label)
	c.JSON(http.StatusOK, gin.H{"success": true})
}
