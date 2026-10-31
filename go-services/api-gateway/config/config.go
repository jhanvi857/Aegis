package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Config struct {
	ServerPort           string
	Environment          string // development, staging, production
	DatabaseURL          string
	RedisURL             string
	PythonGRPCAddr       string
	ChaosEngineURL       string
	RecoveryEngineURL    string
	TLSEnabled           bool
	TLSCertFile          string
	TLSKeyFile           string
	CORSAllowedOrigins   []string
	ShutdownTimeout      time.Duration
	LogLevel             string
	DemoMode             bool
	AutonomousRecovery   bool
	ChaosEnabled         bool
	AllowProductionChaos bool
	MetricsEnabled       bool
	OTLPHTTPPort         string
	OTLPGRPCPort         string
	IngestToken          string
	PrometheusURL        string
	ScrapeTargets        []string
	ScrapeFileSD         string
	BootstrapAdminEmail  string
	BootstrapAdminPass   string
}

func LoadConfig() (*Config, error) {
	port := getEnv("API_GATEWAY_PORT", "8000")
	env := strings.ToLower(getEnv("AEGIS_ENV", "development"))
	logLevel := strings.ToLower(getEnv("AEGIS_LOG_LEVEL", "info"))

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		host := getEnv("POSTGRES_HOST", "")
		if host != "" {
			pgPort := getEnv("POSTGRES_PORT", "5432")
			user := getEnv("POSTGRES_USER", "aegis")
			pass := os.Getenv("POSTGRES_PASSWORD")
			db := getEnv("POSTGRES_DB", "aegis_db")
			sslmode := getEnv("POSTGRES_SSLMODE", "disable")
			if pass != "" {
				dbURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, pgPort, db, sslmode)
			}
		}
	}

	redisURL := getEnv("REDIS_URL", "redis://localhost:6379/0")
	pythonGRPC := getEnv("PYTHON_GRPC_ADDR", "localhost:50051")
	chaosURL := getEnv("CHAOS_ENGINE_URL", "http://localhost:8091")
	recoveryURL := getEnv("RECOVERY_ENGINE_URL", "http://localhost:8092")

	tlsEnabled := parseBool(getEnv("AEGIS_TLS_ENABLED", "false"))
	tlsCert := getEnv("AEGIS_TLS_CERT_FILE", "")
	tlsKey := getEnv("AEGIS_TLS_KEY_FILE", "")

	corsRaw := getEnv("AEGIS_CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000,http://127.0.0.1:5173")
	var corsOrigins []string
	for _, o := range strings.Split(corsRaw, ",") {
		trimmed := strings.TrimSpace(o)
		if trimmed != "" {
			corsOrigins = append(corsOrigins, trimmed)
		}
	}

	shutdownSec := parseInt(getEnv("AEGIS_SHUTDOWN_TIMEOUT_SEC", "15"), 15)
	shutdownTimeout := time.Duration(shutdownSec) * time.Second

	demoMode := parseBool(getEnv("DEMO_MODE", "false")) || parseBool(getEnv("AEGIS_DEMO_MODE", "false"))
	autoRecovery := false
	if val := os.Getenv("AEGIS_AUTONOMOUS_RECOVERY"); val != "" {
		autoRecovery = parseBool(val)
	}

	metricsEnabled := true
	if val := os.Getenv("AEGIS_METRICS_ENABLED"); val != "" {
		metricsEnabled = parseBool(val)
	}

	scrapeTargets := make([]string, 0)
	for _, target := range strings.Split(os.Getenv("AEGIS_SCRAPE_TARGETS"), ",") {
		if target = strings.TrimSpace(target); target != "" {
			scrapeTargets = append(scrapeTargets, target)
		}
	}

	chaosDefault := "true"
	if env == "production" {
		chaosDefault = "false"
	}
	chaosEnabled := parseBool(getEnv("CHAOS_ENABLED", chaosDefault))
	allowProdChaos := parseBool(getEnv("CHAOS_ALLOW_PRODUCTION", "false"))

	adminEmail := getEnv("AEGIS_BOOTSTRAP_ADMIN_EMAIL", "admin@aegis.local")
	adminPass := os.Getenv("AEGIS_BOOTSTRAP_ADMIN_PASSWORD")
	if adminPass == "" && env == "production" {
		return nil, fmt.Errorf("AEGIS_BOOTSTRAP_ADMIN_PASSWORD environment variable is required in production")
	}

	cfg := &Config{
		ServerPort:           port,
		Environment:          env,
		DatabaseURL:          dbURL,
		RedisURL:             redisURL,
		PythonGRPCAddr:       pythonGRPC,
		ChaosEngineURL:       chaosURL,
		RecoveryEngineURL:    recoveryURL,
		TLSEnabled:           tlsEnabled,
		TLSCertFile:          tlsCert,
		TLSKeyFile:           tlsKey,
		CORSAllowedOrigins:   corsOrigins,
		ShutdownTimeout:      shutdownTimeout,
		LogLevel:             logLevel,
		DemoMode:             demoMode,
		AutonomousRecovery:   autoRecovery,
		ChaosEnabled:         chaosEnabled,
		AllowProductionChaos: allowProdChaos,
		MetricsEnabled:       metricsEnabled,
		OTLPHTTPPort:         getEnv("AEGIS_OTLP_HTTP_PORT", "4318"),
		OTLPGRPCPort:         getEnv("AEGIS_OTLP_GRPC_PORT", ""),
		IngestToken:          os.Getenv("AEGIS_INGEST_TOKEN"),
		PrometheusURL:        getEnv("AEGIS_PROM_URL", ""),
		ScrapeTargets:        scrapeTargets,
		ScrapeFileSD:         getEnv("AEGIS_SCRAPE_FILE_SD", ""),
		BootstrapAdminEmail:  adminEmail,
		BootstrapAdminPass:   adminPass,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ValidatePasswordComplexity validates password length and character composition
func ValidatePasswordComplexity(pass string) error {
	if len(pass) < 12 {
		return fmt.Errorf("password must be at least 12 characters long")
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range pass {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		return fmt.Errorf("password must contain at least one uppercase letter, one lowercase letter, one digit, and one special character")
	}
	return nil
}

func (c *Config) Validate() error {
	if c.BootstrapAdminPass != "" {
		if err := ValidatePasswordComplexity(c.BootstrapAdminPass); err != nil {
			return fmt.Errorf("invalid AEGIS_BOOTSTRAP_ADMIN_PASSWORD: %w", err)
		}
	}

	if c.Environment == "production" {
		if c.TLSEnabled {
			if c.TLSCertFile == "" || c.TLSKeyFile == "" {
				return fmt.Errorf("production mode with TLS requires AEGIS_TLS_CERT_FILE and AEGIS_TLS_KEY_FILE")
			}
		}

		for _, o := range c.CORSAllowedOrigins {
			if o == "*" {
				return fmt.Errorf("production environment strictly forbids wildcard '*' in AEGIS_CORS_ALLOWED_ORIGINS")
			}
			if _, err := url.ParseRequestURI(o); err != nil {
				return fmt.Errorf("invalid CORS origin in production '%s': %w", o, err)
			}
		}

		if c.BootstrapAdminPass == "" {
			return fmt.Errorf("production mode requires AEGIS_BOOTSTRAP_ADMIN_PASSWORD to be set")
		}

		if c.DatabaseURL == "" {
			return fmt.Errorf("production mode strictly requires explicit DATABASE_URL or POSTGRES_PASSWORD; default credentials are forbidden")
		}
		if strings.Contains(c.DatabaseURL, "insecure_default_pass") || strings.Contains(c.DatabaseURL, ":password@") || strings.Contains(c.DatabaseURL, "://aegis:@") {
			return fmt.Errorf("production mode strictly forbids using insecure or default database credentials")
		}

		if c.ChaosEnabled && !c.AllowProductionChaos {
			return fmt.Errorf("chaos injection is disabled in production by default. Set CHAOS_ALLOW_PRODUCTION=true if authorized for live disaster testing")
		}
	}

	return nil
}

func (c *Config) IsOriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	for _, allowed := range c.CORSAllowedOrigins {
		if allowed == "*" || strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func parseBool(val string) bool {
	return strings.EqualFold(val, "true") || val == "1" || strings.EqualFold(val, "yes")
}

func parseInt(val string, fallback int) int {
	if i, err := strconv.Atoi(val); err == nil {
		return i
	}
	return fallback
}
