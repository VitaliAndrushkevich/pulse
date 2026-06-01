package config

import "os"

const (
	defaultPulsePort     = "8080"
	defaultDatabaseURL   = "postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable"
	defaultMigrationsURI = "file://migrations"
)

// App stores runtime configuration for the Pulse API process.
type App struct {
	Port        string
	DatabaseURL string
}

// Migrate stores runtime configuration for the migration command.
type Migrate struct {
	DatabaseURL    string
	MigrationsPath string
}

// LoadApp reads all environment variables used by cmd/pulse.
func LoadApp() App {
	return App{
		Port:        getEnv("PULSE_PORT", defaultPulsePort),
		DatabaseURL: getEnv("DATABASE_URL", defaultDatabaseURL),
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
