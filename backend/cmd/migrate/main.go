package main

import (
	"errors"
	"flag"
	"fmt"
	"log"

	"github.com/VitaliAndrushkevich/pulse/internal/config"
	"github.com/VitaliAndrushkevich/pulse/migrations"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func main() {
	direction := flag.String("direction", "up", "migration direction: up or down")
	steps := flag.Int("steps", 0, "number of steps to apply (0 = all)")
	flag.Parse()

	cfg := config.LoadMigrate()

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		log.Fatalf("failed to open embedded migrations: %v", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, cfg.DatabaseURL)
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
