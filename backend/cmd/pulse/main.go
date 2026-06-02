package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/VitaliAndrushkevich/pulse/internal/api"
	"github.com/VitaliAndrushkevich/pulse/internal/api/handlers"
	"github.com/VitaliAndrushkevich/pulse/internal/config"
	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	"github.com/VitaliAndrushkevich/pulse/internal/monitor"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
)

func main() {
	cfg := config.LoadApp()

	// Load encryption key for secrets (TASK-009/010).
	secretKey, err := crypto.LoadKey("PULSE_SECRET_KEY")
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	log.Printf("startup: secret key loaded")

	// Load JWT secret (TASK-022).
	jwtSecret := []byte(cfg.JWTSecret)
	if len(jwtSecret) == 0 {
		log.Fatalf("startup: PULSE_JWT_SECRET is not set or empty")
	}
	jwtExpiry, err := time.ParseDuration(cfg.JWTExpiry)
	if err != nil {
		log.Fatalf("startup: invalid PULSE_JWT_EXPIRY: %v", err)
	}
	log.Printf("startup: JWT configured (expiry=%s)", jwtExpiry)

	// Fail-fast dependency initialization: refuse to start when PostgreSQL or
	// TimescaleDB extension is unavailable (TASK-008).
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startupCancel()

	pool, err := db.Connect(startupCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("startup: postgres unavailable: %v", err)
	}
	defer pool.Close()
	log.Printf("startup: postgres connection established")

	timescaleStore := timescale.New(pool)
	if err := timescaleStore.Ping(startupCtx); err != nil {
		log.Fatalf("startup: timescaledb unavailable: %v", err)
	}
	log.Printf("startup: timescaledb extension available")

	queries := db.New(pool)

	// Initialize Prometheus metrics (TASK-026).
	promRegistry := prometheus.NewRegistry()
	metrics := handlers.NewMetrics(promRegistry)

	// Initialize monitor engine (TASK-014 through TASK-020).
	registry := monitor.DefaultRegistry()
	scheduler := monitor.NewScheduler(monitor.SchedulerConfig{
		Workers: cfg.SchedulerWorkers,
	}, registry, queries, timescaleStore, metrics)

	// Start scheduler and LISTEN/NOTIFY listener in background.
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	go scheduler.Run(appCtx)
	go monitor.NewListener(pool, scheduler).Run(appCtx)
	log.Printf("startup: monitor scheduler started (%d workers)", cfg.SchedulerWorkers)

	r := api.NewRouter(api.Deps{
		Queries:        queries,
		SecretKey:      secretKey,
		JWTSecret:      jwtSecret,
		JWTExpiry:      jwtExpiry,
		TimescaleStore: timescaleStore,
		Metrics:        metrics,
		PromRegistry:   promRegistry,
	})
	addr := ":" + cfg.Port
	log.Printf("pulse listening on %s", addr)

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received %s, shutting down...", sig)
		appCancel()
	}()

	if err := r.Run(addr); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
