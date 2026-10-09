// Package config loads runtime configuration from environment variables,
// mirroring the keys used by the original Laravel .env file where they map
// to a real equivalent here. See docs/analysis/ARCHITECTURE.md.
package config

import (
	"os"
	"strconv"
	"time"
)

// The original app's clock is UTC (config/app.php 'timezone' => 'UTC'):
// "today", "this month" and "last month" in reports and jobs are UTC
// dates. Pin the process to UTC so a server set to Asia/Colombo can't
// shift them. Every binary loads config before doing anything else.
func init() {
	time.Local = time.UTC
}

type Config struct {
	AppName string
	AppURL  string
	AppEnv  string
	Port    string

	DBHost     string
	DBPort     string
	DBDatabase string
	DBUsername string
	DBPassword string

	SessionSecret   string
	SessionLifetime time.Duration

	MailHost string
	MailPort string
	MailUser string
	MailPass string
	MailFrom string

	MiningWebhookEnabled bool
	MiningWebhookURL     string
	MiningWebhookSecret  string

	// FrontendOrigin is the Vue dev-server origin allowed for CORS in dev.
	FrontendOrigin string

	// EnableScheduler controls whether this process runs the in-process
	// recurring jobs (mining:update, mining:send-webhook, packages:weekly-sum,
	// share:calculate — see internal/jobs/scheduler.go), replacing the
	// original app's OS-level cron. Defaults to on so a single-instance
	// deployment behaves like the original out of the box; set
	// ENABLE_SCHEDULER=false on every instance but one when running more
	// than one API process against the same database, to avoid duplicate
	// job runs.
	EnableScheduler bool

	// StorageDriver selects where uploaded files (KYC images) go: "local"
	// (default, under StorageLocalRoot) or "s3" (AWSBucket; reads fall back
	// to StorageLocalRoot for files uploaded before the switch). See
	// internal/storage. AWS credentials are read by the AWS SDK itself from
	// AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY.
	StorageDriver    string
	StorageLocalRoot string
	AWSBucket        string
	AWSRegion        string
	AWSEndpoint      string // only for S3-compatible stores; empty for AWS
	AWSUsePathStyle  bool
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func Load() *Config {
	lifetimeMinutes, err := strconv.Atoi(env("SESSION_LIFETIME", "120"))
	if err != nil {
		lifetimeMinutes = 120
	}

	return &Config{
		AppName: env("APP_NAME", "Signetint.net"),
		AppURL:  env("APP_URL", "http://127.0.0.1:8080"),
		AppEnv:  env("APP_ENV", "local"),
		Port:    env("PORT", "8080"),

		DBHost:     env("DB_HOST", "127.0.0.1"),
		DBPort:     env("DB_PORT", "3306"),
		DBDatabase: env("DB_DATABASE", "signet_last"),
		DBUsername: env("DB_USERNAME", "root"),
		DBPassword: env("DB_PASSWORD", ""),

		SessionSecret:   env("SESSION_SECRET", "insecure-dev-secret-change-me"),
		SessionLifetime: time.Duration(lifetimeMinutes) * time.Minute,

		MailHost: env("MAIL_HOST", "127.0.0.1"),
		MailPort: env("MAIL_PORT", "1025"),
		MailUser: env("MAIL_USERNAME", ""),
		MailPass: env("MAIL_PASSWORD", ""),
		MailFrom: env("MAIL_FROM_ADDRESS", "hello@example.com"),

		MiningWebhookEnabled: envBool("MINING_WEBHOOK_ENABLED", false),
		MiningWebhookURL:     env("MINING_WEBHOOK_URL", ""),
		MiningWebhookSecret:  env("MINING_WEBHOOK_SECRET", ""),

		FrontendOrigin: env("FRONTEND_ORIGIN", "http://localhost:5173"),

		EnableScheduler: envBool("ENABLE_SCHEDULER", true),

		StorageDriver:    env("STORAGE_DRIVER", "local"),
		StorageLocalRoot: env("STORAGE_LOCAL_ROOT", "storage"),
		AWSBucket:        env("AWS_BUCKET", ""),
		AWSRegion:        env("AWS_DEFAULT_REGION", ""),
		AWSEndpoint:      env("AWS_ENDPOINT", ""),
		AWSUsePathStyle:  envBool("AWS_USE_PATH_STYLE_ENDPOINT", false),
	}
}

func (c *Config) MySQLDSN() string {
	// The original app runs in UTC (config/app.php 'timezone' => 'UTC'), so
	// the session is pinned to UTC regardless of the server's own timezone:
	// TIMESTAMP columns are read, compared and DATE()-ed in the session
	// zone, and a server set to Asia/Colombo would otherwise shift every
	// date filter by 5:30. loc=UTC makes Go read/write times the same way.
	return c.DBUsername + ":" + c.DBPassword + "@tcp(" + c.DBHost + ":" + c.DBPort + ")/" + c.DBDatabase +
		"?parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27&charset=utf8mb4"
}
