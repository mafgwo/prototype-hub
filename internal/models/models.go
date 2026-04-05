package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"

	ProjectStatusActive   = "active"
	ProjectStatusArchived = "archived"

	VersionStatusProcessing = "processing"
	VersionStatusReady      = "ready"
	VersionStatusFailed     = "failed"

	SystemRoleAdmin        = "admin"
	SystemRoleProjectAdmin = "project_admin"
	SystemRoleViewer       = "viewer"

	ProjectRoleViewer = "viewer"
	ProjectRoleEditor = "editor"
)

type User struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Username     string    `json:"username" gorm:"uniqueIndex;size:64;not null"`
	PasswordHash string    `json:"-" gorm:"size:255;not null"`
	DisplayName  string    `json:"displayName" gorm:"size:120;not null"`
	Status       string    `json:"status" gorm:"size:20;not null;default:active"`
	Roles        []Role    `json:"roles,omitempty" gorm:"many2many:user_roles;constraint:OnDelete:CASCADE;"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Role struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Code      string    `json:"code" gorm:"uniqueIndex;size:32;not null"`
	Name      string    `json:"name" gorm:"size:64;not null"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Project struct {
	ID               uint             `json:"id" gorm:"primaryKey"`
	Name             string           `json:"name" gorm:"size:160;not null"`
	Slug             string           `json:"slug" gorm:"uniqueIndex;size:180;not null"`
	Description      string           `json:"description" gorm:"type:text"`
	OwnerID          uint             `json:"ownerId"`
	Owner            User             `json:"owner" gorm:"foreignKey:OwnerID"`
	Status           string           `json:"status" gorm:"size:20;not null;default:active"`
	CurrentVersionID *uint            `json:"currentVersionId"`
	CurrentVersion   *ProjectVersion  `json:"currentVersion,omitempty" gorm:"foreignKey:CurrentVersionID"`
	Members          []ProjectMember  `json:"members,omitempty" gorm:"constraint:OnDelete:CASCADE;"`
	Versions         []ProjectVersion `json:"versions,omitempty" gorm:"constraint:OnDelete:CASCADE;"`
	CreatedAt        time.Time        `json:"createdAt"`
	UpdatedAt        time.Time        `json:"updatedAt"`
}

type ProjectVersion struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	ProjectID     uint       `json:"projectId" gorm:"index;not null"`
	VersionNo     int        `json:"versionNo" gorm:"not null"`
	Label         string     `json:"label" gorm:"size:120"`
	ZipObjectKey  string     `json:"zipObjectKey" gorm:"size:255;not null"`
	ExtractPrefix string     `json:"extractPrefix" gorm:"size:255;not null"`
	EntryFile     string     `json:"entryFile" gorm:"size:255;not null"`
	HTMLFiles     StringList `json:"htmlFiles" gorm:"type:text"`
	Status        string     `json:"status" gorm:"size:20;not null;default:processing"`
	ErrorMessage  string     `json:"errorMessage" gorm:"type:text"`
	UploadUserID  uint       `json:"uploadUserId"`
	UploadUser    User       `json:"uploadUser" gorm:"foreignKey:UploadUserID"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type ProjectMember struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	ProjectID uint      `json:"projectId" gorm:"uniqueIndex:idx_project_member;not null"`
	UserID    uint      `json:"userId" gorm:"uniqueIndex:idx_project_member;not null"`
	Role      string    `json:"role" gorm:"size:20;not null"`
	User      User      `json:"user" gorm:"foreignKey:UserID"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AuditLog struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	ActorID    *uint     `json:"actorId"`
	Actor      *User     `json:"actor,omitempty"`
	Action     string    `json:"action" gorm:"size:80;not null"`
	TargetType string    `json:"targetType" gorm:"size:80;not null"`
	TargetID   string    `json:"targetId" gorm:"size:120;not null"`
	Detail     string    `json:"detail" gorm:"type:text"`
	CreatedAt  time.Time `json:"createdAt"`
}

type StringList []string

func (s StringList) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal([]string(s))
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (s *StringList) Scan(value any) error {
	if value == nil {
		*s = StringList{}
		return nil
	}

	var raw string
	switch typed := value.(type) {
	case []byte:
		raw = string(typed)
	case string:
		raw = typed
	default:
		return fmt.Errorf("unsupported StringList scan type %T", value)
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		*s = StringList{}
		return nil
	}

	if strings.HasPrefix(raw, "[") {
		var items []string
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			return err
		}
		*s = StringList(items)
		return nil
	}

	*s = StringList{raw}
	return nil
}
