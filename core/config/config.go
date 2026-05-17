// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package config provides a unified YAML-based configuration loader for
// the SovereignStack gateway, management, and visibility services.
//
// Precedence (highest wins): CLI flags > environment variables > YAML file > defaults.
// CLI flag overrides are the caller's responsibility (cobra / flag); this package
// returns the result of merging the lower three layers.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration shared across all services.
// Each service reads its own section plus the shared Log/CORS sections.
type Config struct {
	Log        LogConfig        `yaml:"log"`
	Gateway    GatewayConfig    `yaml:"gateway"`
	Policy     PolicyConfig     `yaml:"policy"`
	Visibility VisibilityConfig `yaml:"visibility"`
	CORS       CORSConfig       `yaml:"cors"`
	TLS        TLSConfig        `yaml:"tls"`
}

// LogConfig controls structured logging behaviour for all services.
type LogConfig struct {
	Format string `yaml:"format" env:"SOVSTACK_LOG_FORMAT"` // text | json
	Level  string `yaml:"level"  env:"SOVSTACK_LOG_LEVEL"`  // debug | info | warn | error
}

// GatewayConfig holds settings specific to the gateway service.
type GatewayConfig struct {
	Port          int         `yaml:"port"           env:"SOVSTACK_GATEWAY_PORT"`
	RateLimit     float64     `yaml:"rate_limit"     env:"SOVSTACK_GATEWAY_RATE_LIMIT"`
	APIKeyHeader  string      `yaml:"api_key_header" env:"SOVSTACK_GATEWAY_API_KEY_HEADER"`
	Backend       string      `yaml:"backend"        env:"SOVSTACK_GATEWAY_BACKEND"`
	// DiscoveryURL is the URL of the discovery service that lists running
	// model containers. The gateway polls /api/v1/models/running on this
	// URL to keep its routing table fresh. Default: http://localhost:8889
	// (the discovery subcommand's default port).
	DiscoveryURL  string      `yaml:"discovery_url"  env:"SOVSTACK_GATEWAY_DISCOVERY_URL"`
	KeysFile      string      `yaml:"keys_file"      env:"SOVSTACK_KEYS_FILE"`
	AuditDB       string      `yaml:"audit_db"       env:"SOVSTACK_AUDIT_DB"`
	Audit         AuditConfig `yaml:"audit"`
	Quota         QuotaConfig `yaml:"quota"`
	Admission     AdmissionConfig `yaml:"admission"`
}

// AdmissionConfig tunes the host-aware adaptive admission controller that
// the gateway runs in front of every reverse-proxy call. When Enabled is
// true, the controller polls each running model's vLLM /metrics via the
// management service, tracks an EMA of GPU kv-cache pressure, and rejects
// requests with 503 + Retry-After when the host approaches saturation.
//
// All thresholds are percent (0..100) on the gpu_cache_usage_perc gauge.
// Defaults derive from the host's VRAM (see Defaults below).
type AdmissionConfig struct {
	// Enabled is the master switch. When false, the gateway admits all
	// requests regardless of host state (preserves prior behavior).
	Enabled bool `yaml:"enabled" env:"SOVSTACK_ADMISSION_ENABLED"`

	// HardCachePct is the upper bound on dynamic kv-cache cap (default 95).
	HardCachePct float64 `yaml:"hard_cache_pct" env:"SOVSTACK_ADMISSION_HARD_CACHE_PCT"`

	// SoftCachePct is the warn-only threshold below the hard cap (default 80).
	SoftCachePct float64 `yaml:"soft_cache_pct" env:"SOVSTACK_ADMISSION_SOFT_CACHE_PCT"`

	// MinHardCachePct floors the dynamic cap so it can't drift below
	// this value under sustained stress (default 60).
	MinHardCachePct float64 `yaml:"min_hard_cache_pct" env:"SOVSTACK_ADMISSION_MIN_HARD_CACHE_PCT"`

	// StressMargin is how far the dynamic cap drops on a stress event
	// and steps back up by half on each calm window (default 10).
	StressMargin float64 `yaml:"stress_margin" env:"SOVSTACK_ADMISSION_STRESS_MARGIN"`

	// MaxQueuePerGB caps queued vLLM requests per GB of host VRAM,
	// fair-shared across running models (default 4).
	MaxQueuePerGB int `yaml:"max_queue_per_gb" env:"SOVSTACK_ADMISSION_MAX_QUEUE_PER_GB"`

	// PollSeconds is how often the controller refreshes its view of
	// per-model metrics (default 5).
	PollSeconds int `yaml:"poll_seconds" env:"SOVSTACK_ADMISSION_POLL_SECONDS"`

	// EMAAlpha is the smoothing factor in (0,1] for the cache-usage EMA
	// (default 0.2).
	EMAAlpha float64 `yaml:"ema_alpha" env:"SOVSTACK_ADMISSION_EMA_ALPHA"`

	// CalmSeconds is how long the EMA must stay below SoftCachePct
	// before the dynamic cap relaxes back toward HardCachePct
	// (default 300 = 5 min).
	CalmSeconds int `yaml:"calm_seconds" env:"SOVSTACK_ADMISSION_CALM_SECONDS"`
}

// AuditConfig selects audit log destinations. The gateway can fan out to
// any combination of sinks: SQLite (default, encrypted), MySQL hot path,
// and JSONL cold path with daily rotation.
type AuditConfig struct {
	// JSONLDir, if non-empty, writes audit records to daily-rotated
	// JSONL files in this directory (e.g. "/var/log/sovereignstack").
	// Yesterday's file is gzipped automatically.
	JSONLDir string `yaml:"jsonl_dir" env:"SOVSTACK_AUDIT_JSONL_DIR"`

	// MySQLDSN, if non-empty, also writes to a MySQL audit_logs table.
	// Format: "user:pass@tcp(host:3306)/dbname?parseTime=true"
	MySQLDSN string `yaml:"mysql_dsn" env:"SOVSTACK_AUDIT_MYSQL_DSN"`

	// RetentionDays for the MySQL hot path; 0 disables auto-pruning.
	// JSONL files are not auto-pruned (use logrotate or a cron job).
	RetentionDays int `yaml:"retention_days" env:"SOVSTACK_AUDIT_RETENTION_DAYS"`

	// ShipperURL forwards each record after writing locally. Schemes:
	// s3://, syslog://host:port, http(s)://endpoint. Empty disables.
	ShipperURL string `yaml:"shipper_url" env:"SOVSTACK_AUDIT_SHIPPER_URL"`
}

// QuotaConfig selects and configures the quota backend.
//
//	backend: memory | sqlite | redis
//	  memory — single-instance, lost on restart (default)
//	  sqlite — single-instance, persists across restarts
//	  redis  — multi-instance, shared state
type QuotaConfig struct {
	Backend  string `yaml:"backend"   env:"SOVSTACK_QUOTA_BACKEND"`   // memory | sqlite | redis
	SQLiteDB string `yaml:"sqlite_db" env:"SOVSTACK_QUOTA_SQLITE_DB"` // path when backend=sqlite
	Redis    QuotaRedisConfig `yaml:"redis"`
}

// QuotaRedisConfig configures the Redis quota backend.
type QuotaRedisConfig struct {
	Addr      string `yaml:"addr"       env:"SOVSTACK_QUOTA_REDIS_ADDR"`
	Password  string `yaml:"password"   env:"SOVSTACK_QUOTA_REDIS_PASSWORD"`
	DB        int    `yaml:"db"         env:"SOVSTACK_QUOTA_REDIS_DB"`
	KeyPrefix string `yaml:"key_prefix" env:"SOVSTACK_QUOTA_REDIS_KEY_PREFIX"`
}

// PolicyConfig holds settings specific to the policy service (the
// user/quota/access REST API on port 8888). Renamed from
// ManagementConfig when the unified `sovstack management` shim was
// removed; the YAML key is now `policy:` to match the subcommand name.
type PolicyConfig struct {
	Port     int    `yaml:"port"      env:"SOVSTACK_POLICY_PORT"`
	KeysFile string `yaml:"keys_file" env:"SOVSTACK_KEYS_FILE"`
	AdminKey string `yaml:"admin_key" env:"SOVSTACK_ADMIN_KEY"`

	// AdminKeys is the Phase C4 named-actor admin map. Keys are actor
	// names (logged in audit trails); values are the Bearer tokens those
	// actors present. AdminKey above remains as a single-actor fallback
	// labelled "admin".
	AdminKeys map[string]string `yaml:"admin_keys"`

	// OIDC, when present, enables /api/v1/auth/{login,callback,logout}
	// on the policy service. Phase F1.
	OIDC OIDCConfig `yaml:"oidc"`
}

// OIDCConfig configures sign-in via any OpenID Connect provider
// (Keycloak, Authentik, Auth0, Okta, …). All four required fields must
// be set to activate; otherwise OIDC endpoints reply 503 and admin auth
// falls back to the AdminKey Bearer.
type OIDCConfig struct {
	IssuerURL    string `yaml:"issuer_url"     env:"SOVSTACK_OIDC_ISSUER_URL"`
	ClientID     string `yaml:"client_id"      env:"SOVSTACK_OIDC_CLIENT_ID"`
	ClientSecret string `yaml:"client_secret"  env:"SOVSTACK_OIDC_CLIENT_SECRET"`
	RedirectURL  string `yaml:"redirect_url"   env:"SOVSTACK_OIDC_REDIRECT_URL"`
	AdminClaim   string `yaml:"admin_claim"    env:"SOVSTACK_OIDC_ADMIN_CLAIM"` // defaults "role"
}

// VisibilityConfig holds settings specific to the visibility backend.
type VisibilityConfig struct {
	Port       int            `yaml:"port"        env:"SOVSTACK_VISIBILITY_PORT"`
	GatewayURL string         `yaml:"gateway_url" env:"SOVSTACK_VISIBILITY_GATEWAY_URL"`
	KeysFile   string         `yaml:"keys_file"   env:"SOVSTACK_KEYS_FILE"`
	DBType     string         `yaml:"db_type"     env:"SOVSTACK_VISIBILITY_DB_TYPE"` // sqlite | mysql
	DB         DatabaseConfig `yaml:"db"`
}

// DatabaseConfig is the shared shape for SQL database connections.
type DatabaseConfig struct {
	Host     string `yaml:"host"     env:"SOVSTACK_DB_HOST"`
	Port     int    `yaml:"port"     env:"SOVSTACK_DB_PORT"`
	Name     string `yaml:"name"     env:"SOVSTACK_DB_NAME"`
	User     string `yaml:"user"     env:"SOVSTACK_DB_USER"`
	Password string `yaml:"password" env:"SOVSTACK_DB_PASSWORD"`
}

// CORSConfig is shared by all services that expose HTTP endpoints to browsers.
type CORSConfig struct {
	Origins          []string `yaml:"origins"`
	AllowDevWildcard bool     `yaml:"allow_dev_wildcard"`
}

// TLSConfig controls how each service serves HTTPS. Defaults amount to
// "TLS on, self-signed cert generated under ~/.sovereignstack/tls/" — set
// InsecureHTTP=true (or pass --insecure-http) only for dev.
type TLSConfig struct {
	CertFile     string `yaml:"cert_file"     env:"SOVSTACK_TLS_CERT_FILE"`
	KeyFile      string `yaml:"key_file"      env:"SOVSTACK_TLS_KEY_FILE"`
	Dir          string `yaml:"dir"           env:"SOVSTACK_TLS_DIR"`
	InsecureHTTP bool   `yaml:"insecure_http" env:"SOVSTACK_INSECURE_HTTP"`
}

// Defaults returns a Config populated with sensible production defaults.
// Callers should not mutate the returned struct's nested slices/maps in place
// across goroutines without copying.
func Defaults() *Config {
	return &Config{
		Log: LogConfig{Format: "text", Level: "info"},
		Gateway: GatewayConfig{
			Port:          8001,
			RateLimit:     100,
			APIKeyHeader:  "X-API-Key",
			Backend:       "http://localhost:8000",
			DiscoveryURL:  "http://localhost:8889",
			KeysFile:      "",
			AuditDB:       "./sovstack-audit.db",
			Quota: QuotaConfig{
				Backend:  "memory",
				SQLiteDB: "./sovstack-quota.db",
				Redis: QuotaRedisConfig{
					Addr:      "localhost:6379",
					KeyPrefix: "sovstack:quota:",
				},
			},
			Admission: AdmissionConfig{
				Enabled:         true,
				HardCachePct:    95,
				SoftCachePct:    80,
				MinHardCachePct: 60,
				StressMargin:    10,
				MaxQueuePerGB:   4,
				PollSeconds:     5,
				EMAAlpha:        0.2,
				CalmSeconds:     300,
			},
		},
		Policy: PolicyConfig{
			Port: 8888,
		},
		Visibility: VisibilityConfig{
			Port:       9000,
			GatewayURL: "http://localhost:8001",
			DBType:     "sqlite",
			DB: DatabaseConfig{
				Host: "localhost",
				Port: 3306,
				Name: "visibility",
				User: "visibility",
			},
		},
		CORS: CORSConfig{
			Origins:          []string{},
			AllowDevWildcard: false,
		},
	}
}

// Load reads YAML from path (if non-empty and the file exists), then
// applies environment-variable overrides for any field with an `env:"..."` tag.
//
// A non-existent file at the given path is not an error; defaults plus env
// overrides are returned. A malformed file is an error.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	if path != "" {
		expanded := expandPath(path)
		data, err := os.ReadFile(expanded)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config %q: %w", expanded, err)
		}
		if err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parse config %q: %w", expanded, err)
			}
		}
	}

	if err := applyEnv(reflect.ValueOf(cfg).Elem()); err != nil {
		return nil, err
	}
	return cfg, nil
}

// expandPath resolves a leading ~/ to the user's home directory.
func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// applyEnv walks a struct value and overrides any field whose `env:"NAME"`
// tag matches a set environment variable. Supported field kinds: string, int
// family, float family, bool. Nested structs are recursed into; pointers,
// maps, and slices are not handled by env (declare them in YAML instead).
func applyEnv(v reflect.Value) error {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		structField := t.Field(i)

		if field.Kind() == reflect.Struct {
			if err := applyEnv(field); err != nil {
				return err
			}
			continue
		}

		envName := structField.Tag.Get("env")
		if envName == "" {
			continue
		}
		envVal, ok := os.LookupEnv(envName)
		if !ok || envVal == "" {
			continue
		}

		switch field.Kind() {
		case reflect.String:
			field.SetString(envVal)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(envVal, 10, 64)
			if err != nil {
				return fmt.Errorf("env %s: %w", envName, err)
			}
			field.SetInt(n)
		case reflect.Float32, reflect.Float64:
			f, err := strconv.ParseFloat(envVal, 64)
			if err != nil {
				return fmt.Errorf("env %s: %w", envName, err)
			}
			field.SetFloat(f)
		case reflect.Bool:
			b, err := strconv.ParseBool(envVal)
			if err != nil {
				return fmt.Errorf("env %s: %w", envName, err)
			}
			field.SetBool(b)
		default:
			return fmt.Errorf("env %s: unsupported field kind %s", envName, field.Kind())
		}
	}
	return nil
}
