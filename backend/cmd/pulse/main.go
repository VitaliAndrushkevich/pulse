package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/api"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
)

func main() {
	port := os.Getenv("PULSE_PORT")
	if port == "" {
		port = "8080"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable"
	}

	// Fail-fast dependency initialization: refuse to start when PostgreSQL or
	// TimescaleDB extension is unavailable (TASK-008).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("startup: postgres unavailable: %v", err)
	}
	defer pool.Close()
	log.Printf("startup: postgres connection established")

	timescaleStore := timescale.New(pool)
	if err := timescaleStore.Ping(ctx); err != nil {
		log.Fatalf("startup: timescaledb unavailable: %v", err)
	}
	log.Printf("startup: timescaledb extension available")

	r := api.NewRouter()
	addr := ":" + port
	log.Printf("pulse listening on %s", addr)

	if err := r.Run(addr); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
