package notification

import (
	"context"
	"log"

	db "github.com/VitaliAndrushkevich/pulse/internal/store/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// maxErrorDetailLength is the maximum length for error_detail in delivery_logs.
const maxErrorDetailLength = 1024

// TruncateErrorDetail truncates an error detail string to maxErrorDetailLength characters.
// Returns nil if the input is empty.
func TruncateErrorDetail(detail string) *string {
	if detail == "" {
		return nil
	}
	if len(detail) > maxErrorDetailLength {
		detail = detail[:maxErrorDetailLength]
	}
	return &detail
}

// LogDelivery records a delivery attempt in the delivery_logs table.
// Parameters:
//   - ctx: context for the database operation
//   - channelID: the notification channel used
//   - monitorID: the monitor that triggered the notification
//   - bindingID: the binding that matched (uuid.Nil for no binding)
//   - triggerType: the trigger type (e.g. "monitor_down", "monitor_up")
//   - attempt: the attempt number (1-based)
//   - status: "success" or "failure"
//   - errorDetail: error description (truncated to 1024 chars); empty string means no error
func (d *Dispatcher) LogDelivery(ctx context.Context, channelID, monitorID, bindingID uuid.UUID, triggerType string, attempt int, status string, errorDetail string) {
	if d.queries == nil {
		log.Printf("notification: cannot record delivery log (no database queries configured)")
		return
	}

	bindingPgUUID := pgtype.UUID{}
	if bindingID != uuid.Nil {
		bindingPgUUID = pgtype.UUID{Bytes: bindingID, Valid: true}
	}

	params := db.InsertDeliveryLogParams{
		ChannelID:   channelID,
		MonitorID:   monitorID,
		BindingID:   bindingPgUUID,
		TriggerType: triggerType,
		Attempt:     int32(attempt),
		Status:      status,
		ErrorDetail: TruncateErrorDetail(errorDetail),
	}

	_, err := d.queries.InsertDeliveryLog(ctx, params)
	if err != nil {
		log.Printf("notification: failed to record delivery log: %v (channel=%s monitor=%s trigger=%s attempt=%d)",
			err, channelID, monitorID, triggerType, attempt)
	}
}
