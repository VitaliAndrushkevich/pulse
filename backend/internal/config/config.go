package config

import (
	"os"
	"strconv"
)

const (
	defaultPulsePort         = "8080"
	defaultDatabaseURL       = "postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable"
	defaultMigrationsURI     = "file://migrations"
	defaultSchedulerWorkers  = 50 // dev-friendly default; production deploys should set PULSE_SCHEDULER_WORKERS=200
	defaultJWTExpiry         = "24h"
)

// App stores runtime configuration for the Pulse API process.
type App struct {
	Port             string
	DatabaseURL      string
	SchedulerWorkers int
	JWTSecret        string
	JWTExpiry        string
	DevMode          bool
	OpenAPIDir       string
}

// Migrate stores runtime configuration for the migration command.
type Migrate struct {
	DatabaseURL    string
	MigrationsPath string
}

// LoadApp reads all environment variables used by cmd/pulse.
func LoadApp() App {
	return App{
		Port:             getEnv("PULSE_PORT", defaultPulsePort),
		DatabaseURL:      getEnv("DATABASE_URL", defaultDatabaseURL),
		SchedulerWorkers: getEnvInt("PULSE_SCHEDULER_WORKERS", defaultSchedulerWorkers),
		JWTSecret:        getEnv("PULSE_JWT_SECRET", ""),
		JWTExpiry:        getEnv("PULSE_JWT_EXPIRY", defaultJWTExpiry),
		DevMode:          getEnv("PULSE_DEV", "") == "true",
		OpenAPIDir:       getEnv("PULSE_OPENAPI_DIR", "api"),
	}
}

// LoadMigrate reads all environment variables used by cmd/migrate.
func LoadMigrate() Migrate {
	return Migrate{
		DatabaseURL:    getEnv("DATABASE_URL", defaultDatabaseURL),
		MigrationsPath: getEnv("MIGRATIONS_PATH", defaultMigrationsURI),
	}
}

func getEnv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(name string, fallback int) int {
	if value := os.Getenv(name); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
