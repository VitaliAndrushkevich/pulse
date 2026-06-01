package main

import (
	"context"
	"log"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/api"
	"github.com/VitaliAndrushkevich/pulse/internal/config"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
)

func main() {
	cfg := config.LoadApp()

	// Fail-fast dependency initialization: refuse to start when PostgreSQL or
	// TimescaleDB extension is unavailable (TASK-008).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
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
	addr := ":" + cfg.Port
	log.Printf("pulse listening on %s", addr)

	if err := r.Run(addr); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
