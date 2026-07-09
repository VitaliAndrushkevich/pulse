package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/VitaliAndrushkevich/pulse/internal/api"
	"github.com/VitaliAndrushkevich/pulse/internal/config"
	"github.com/VitaliAndrushkevich/pulse/internal/crypto"
	"github.com/VitaliAndrushkevich/pulse/internal/hub"
	"github.com/VitaliAndrushkevich/pulse/internal/monitor"
	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	smtpclient "github.com/VitaliAndrushkevich/pulse/internal/notification/smtp"
	"github.com/VitaliAndrushkevich/pulse/internal/notification/webhook"
	"github.com/VitaliAndrushkevich/pulse/internal/retention"
	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/VitaliAndrushkevich/pulse/internal/store/timescale"
	"github.com/VitaliAndrushkevich/pulse/internal/version"
	"github.com/VitaliAndrushkevich/pulse/migrations"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func main() {
	log.Printf("Pulse %s starting", version.Version)

	cfg := config.LoadApp()

	if cfg.ResetAdmin {
		log.Printf("WARNING: Admin reset mode is active. Remove PULSE_RESET_ADMIN after completing setup.")
	}

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

	// Auto-apply pending migrations using embedded SQL files.
	migSrc, err := iofs.New(migrations.FS, ".")
	if err != nil {
		log.Fatalf("startup: migration source: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", migSrc, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("startup: migration init: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("startup: migration failed: %v", err)
	}
	srcErr, dbErr := m.Close()
	if srcErr != nil {
		log.Fatalf("startup: migration source close: %v", srcErr)
	}
	if dbErr != nil {
		log.Fatalf("startup: migration db close: %v", dbErr)
	}
	log.Printf("startup: migrations applied")

	timescaleStore := timescale.New(pool)
	if err := timescaleStore.Ping(startupCtx); err != nil {
		log.Fatalf("startup: timescaledb unavailable: %v", err)
	}
	log.Printf("startup: timescaledb extension available")

	queries := db.New(pool)

	// Initialize retention service (monitor-history-explorer).
	retentionInterval, err := retention.ParseRetentionInterval(os.Getenv("PULSE_RETENTION_CHECK_INTERVAL"))
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	retentionSvc, err := retention.New(retention.Config{
		Pool:     pool,
		Queries:  queries,
		Interval: retentionInterval,
	})
	if err != nil {
		log.Fatalf("startup: retention service: %v", err)
	}
	log.Printf("startup: retention service configured (interval=%s)", retentionInterval)

	// Initialize Prometheus metrics (TASK-026).
	promRegistry := prometheus.NewRegistry()

	// Initialize DynamicMetrics for tag-aware scheduler metrics.
	// DynamicMetrics registers itself as an unchecked collector with the registry
	// and handles pulse_monitor_up, pulse_monitor_response_time_seconds, and pulse_monitors_total.
	dynMetrics := monitor.NewDynamicMetrics(promRegistry)

	// Initialize notification dispatcher (notification-channels).
	notifWorkers := getEnvInt("PULSE_NOTIFICATION_WORKERS", 10)
	notifDrainTimeout := getEnvDuration("PULSE_NOTIFICATION_DRAIN_TIMEOUT", 30*time.Second)
	notifCfg := notification.DispatcherConfig{
		Workers:      notifWorkers,
		BufferSize:   256,
		DrainTimeout: notifDrainTimeout,
		BaseURL:      cfg.BaseURL,
	}
	notifMetrics := notification.NewMetrics(promRegistry)
	stateTracker := notification.NewStateTracker()
	dispatcher := notification.NewDispatcher(notifCfg, queries, pool, notifMetrics, stateTracker, secretKey)

	// SMTP startup validation: load settings from DB, validate connectivity.
	// Returns nil if SMTP is not configured or validation fails — app continues either way.
	smtpClient := smtpclient.ValidateOnStartup(startupCtx, queries, secretKey)

	// Wire delivery clients into the dispatcher.
	if smtpClient != nil {
		dispatcher.SetSMTPClient(smtpClient)
		log.Printf("startup: SMTP client configured for notification dispatcher")
	}
	dispatcher.SetWebhookDeliverFn(webhook.DeliverFromRaw)

	dispatcher.Start()
	log.Printf("startup: notification dispatcher started (%d workers, drain timeout %s)", notifWorkers, notifDrainTimeout)

	// Start reminder scheduler for recurring notifications.
	reminderScheduler := notification.NewReminderScheduler(dispatcher, stateTracker, time.Minute)
	reminderScheduler.Start()

	// Initialize monitor engine (TASK-014 through TASK-020).
	registry := monitor.DefaultRegistry(queries)

	// WebSocket hub (TASK-028).
	wsHub := hub.New()
	go wsHub.Run()
	log.Printf("startup: websocket hub started")

	scheduler := monitor.NewScheduler(monitor.SchedulerConfig{
		Workers: cfg.SchedulerWorkers,
	}, registry, queries, timescaleStore, dynMetrics, wsHub, secretKey, dispatcher)

	// Start scheduler and LISTEN/NOTIFY listener in background.
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	go scheduler.Run(appCtx)
	go monitor.NewListener(pool, scheduler).Run(appCtx)
	log.Printf("startup: monitor scheduler started (%d workers)", cfg.SchedulerWorkers)

	go retentionSvc.Start(appCtx)

	r := api.NewRouter(api.Deps{
		Queries:         queries,
		Pool:            pool,
		SecretKey:       secretKey,
		JWTSecret:       jwtSecret,
		JWTExpiry:       jwtExpiry,
		TimescaleStore:  timescaleStore,
		Metrics:         nil,
		PromRegistry:    promRegistry,
		Hub:             wsHub,
		SMTPClient:      smtpClient,
		DevMode:         cfg.DevMode,
		OpenAPIDir:      cfg.OpenAPIDir,
		BaseURL:         cfg.BaseURL,
		MetricsUser:     cfg.MetricsUser,
		MetricsPassword: cfg.MetricsPassword,
		ResetAdmin:      cfg.ResetAdmin,
	})
	addr := ":" + cfg.Port
	if cfg.DevMode {
		log.Printf("startup: dev mode enabled — Swagger UI at http://localhost%s/swagger", addr)
	}
	if cfg.MetricsUser != "" && cfg.MetricsPassword != "" {
		log.Printf("startup: /metrics endpoint protected by Basic Auth")
	} else {
		log.Printf("startup: /metrics endpoint open (no auth — set PULSE_METRICS_USER and PULSE_METRICS_PASSWORD to protect)")
	}
	log.Printf("pulse listening on %s", addr)

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received %s, shutting down...", sig)

		// Stop reminder scheduler first (no new reminders enqueued).
		reminderScheduler.Stop()

		// Drain the notification dispatcher within its configured timeout.
		drainCtx, drainCancel := context.WithTimeout(context.Background(), notifDrainTimeout)
		dispatcher.Shutdown(drainCtx)
		drainCancel()

		wsHub.Stop()
		appCancel()
	}()

	if err := r.Run(addr); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}

// getEnvInt reads an environment variable as a positive integer, returning
// the fallback value if the variable is unset, empty, or invalid.
func getEnvInt(name string, fallback int) int {
	if value := os.Getenv(name); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// getEnvDuration reads an environment variable as a time.Duration, returning
// the fallback value if the variable is unset, empty, or invalid.
func getEnvDuration(name string, fallback time.Duration) time.Duration {
	if value := os.Getenv(name); value != "" {
		if d, err := time.ParseDuration(value); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}
