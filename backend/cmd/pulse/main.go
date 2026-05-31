package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/api"
	"github.com/VitaliAndrushkevich/pulse/internal/store/influx"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
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
	// InfluxDB is unreachable (TASK-008).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("startup: postgres unavailable: %v", err)
	}
	defer pool.Close()
	log.Printf("startup: postgres connection established")

	influxStore := influx.NewFromEnv()
	defer influxStore.Close()
	if err := influxStore.Ping(ctx); err != nil {
		log.Fatalf("startup: influxdb unavailable: %v", err)
	}
	log.Printf("startup: influxdb connection established")

	r := api.NewRouter()
	addr := ":" + port
	log.Printf("pulse listening on %s", addr)

	if err := r.Run(addr); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
