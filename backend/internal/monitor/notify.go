package monitor

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// NotifyChannel is the PostgreSQL LISTEN/NOTIFY channel name for monitor changes.
	NotifyChannel = "monitor_changes"
)

// Listener subscribes to PostgreSQL LISTEN/NOTIFY on the monitor_changes channel
// and wakes up the scheduler when a monitor is created or updated.
type Listener struct {
	pool      *pgxpool.Pool
	scheduler *Scheduler
}

// NewListener creates a new LISTEN/NOTIFY listener.
func NewListener(pool *pgxpool.Pool, scheduler *Scheduler) *Listener {
	return &Listener{
		pool:      pool,
		scheduler: scheduler,
	}
}

// Run starts listening for notifications. It blocks until ctx is cancelled.
// On connection loss, it attempts to re-acquire a connection and re-subscribe.
func (l *Listener) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		l.listen(ctx)
	}
}

func (l *Listener) listen(ctx context.Context) {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Printf("notify: acquire connection: %v", err)
		return
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, "LISTEN "+NotifyChannel)
	if err != nil {
		log.Printf("notify: LISTEN: %v", err)
		return
	}
	log.Printf("notify: subscribed to %s", NotifyChannel)

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("notify: wait: %v (will reconnect)", err)
			return
		}

		log.Printf("notify: received on channel=%s payload=%s",
			notification.Channel, notification.Payload)
		l.scheduler.Wakeup()
	}
}
