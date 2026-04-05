package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                 string
	AppName                string
	HTTPAddr               string
	JWTSecret              string
	JWTCookieName          string
	JWTTTL                 time.Duration
	DatabaseDriver         string
	DatabaseDSN            string
	StorageDriver          string
	LocalStoragePath       string
	S3Endpoint             string
	S3Region               string
	S3Bucket               string
	S3AccessKey            string
	S3SecretKey            string
	S3UsePathStyle         bool
	UploadMaxSizeBytes     int64
	UploadMaxFileCount     int
	UploadMaxExtractedSize int64
	DefaultAdminUsername   string
	DefaultAdminPassword   string
	DefaultAdminDisplay    string
}

func Load() Config {
	jwtHours := getInt("JWT_TTL_HOURS", 24)
	return Config{
		AppEnv:                 getEnv("APP_ENV", "development"),
		AppName:                getEnv("APP_NAME", "Prototype Hub"),
		HTTPAddr:               getEnv("HTTP_ADDR", ":8080"),
		JWTSecret:              getEnv("JWT_SECRET", "change-me-in-production"),
		JWTCookieName:          getEnv("JWT_COOKIE_NAME", "prototypehub_token"),
		JWTTTL:                 time.Duration(jwtHours) * time.Hour,
		DatabaseDriver:         strings.ToLower(getEnv("DB_DRIVER", "sqlite")),
		DatabaseDSN:            getEnv("DB_DSN", "file:data/prototypehub.db?_foreign_keys=on"),
		StorageDriver:          strings.ToLower(getEnv("STORAGE_DRIVER", "local")),
		LocalStoragePath:       getEnv("LOCAL_STORAGE_PATH", "data/storage"),
		S3Endpoint:             getEnv("S3_ENDPOINT", "http://minio:9000"),
		S3Region:               getEnv("S3_REGION", "us-east-1"),
		S3Bucket:               getEnv("S3_BUCKET", "prototype-hub"),
		S3AccessKey:            getEnv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:            getEnv("S3_SECRET_KEY", "minioadmin"),
		S3UsePathStyle:         getBool("S3_USE_PATH_STYLE", true),
		UploadMaxSizeBytes:     getInt64("UPLOAD_MAX_SIZE_BYTES", 100<<20),
		UploadMaxFileCount:     getInt("UPLOAD_MAX_FILE_COUNT", 10000),
		UploadMaxExtractedSize: getInt64("UPLOAD_MAX_EXTRACTED_SIZE_BYTES", 300<<20),
		DefaultAdminUsername:   getEnv("DEFAULT_ADMIN_USERNAME", "admin"),
		DefaultAdminPassword:   getEnv("DEFAULT_ADMIN_PASSWORD", "ChangeMe123!"),
		DefaultAdminDisplay:    getEnv("DEFAULT_ADMIN_DISPLAY_NAME", "System Admin"),
	}
}

func (c Config) IsProduction() bool {
	return strings.EqualFold(c.AppEnv, "production")
}

func (c Config) Validate() error {
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if c.DatabaseDSN == "" {
		return fmt.Errorf("DB_DSN is required")
	}
	switch c.DatabaseDriver {
	case "sqlite", "postgres", "postgresql", "mysql":
	default:
		return fmt.Errorf("DB_DRIVER must be one of sqlite, postgres, mysql")
	}
	if c.StorageDriver == "s3" {
		switch {
		case c.S3Bucket == "":
			return fmt.Errorf("S3_BUCKET is required")
		case c.S3AccessKey == "":
			return fmt.Errorf("S3_ACCESS_KEY is required")
		case c.S3SecretKey == "":
			return fmt.Errorf("S3_SECRET_KEY is required")
		}
	}
	return nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func getInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getInt64(key string, fallback int64) int64 {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
