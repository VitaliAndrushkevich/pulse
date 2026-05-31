package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	direction := flag.String("direction", "up", "migration direction: up or down")
	steps := flag.Int("steps", 0, "number of steps to apply (0 = all)")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable"
	}

	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "file://migrations"
	}

	m, err := migrate.New(migrationsPath, dbURL)
	if err != nil {
		log.Fatalf("failed to create migrator: %v", err)
	}
	defer m.Close()

	var migrateErr error
	switch *direction {
	case "up":
		if *steps > 0 {
			migrateErr = m.Steps(*steps)
		} else {
			migrateErr = m.Up()
		}
	case "down":
		if *steps > 0 {
			migrateErr = m.Steps(-(*steps))
		} else {
			migrateErr = m.Down()
		}
	default:
		log.Fatalf("unknown direction %q: use up or down", *direction)
	}

	if errors.Is(migrateErr, migrate.ErrNoChange) {
		fmt.Println("no migrations to apply")
		return
	}
	if migrateErr != nil {
		log.Fatalf("migration %s failed: %v", *direction, migrateErr)
	}
	fmt.Printf("migration %s applied successfully\n", *direction)
}
