package webhook

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/VitaliAndrushkevich/pulse/internal/notification"
)

// DeliverFromRaw parses a raw JSON config (as stored in DB) and delivers
// the webhook notification. The secretKey is used to decrypt header values.
func DeliverFromRaw(ctx context.Context, rawConfig json.RawMessage, secretKey []byte, data notification.TemplateData) error {
	var cfg WebhookConfig
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return notification.NewNonRetryableError(
			fmt.Errorf("webhook: invalid config JSON: %w", err),
		)
	}

	client := NewClient(secretKey)
	return client.Deliver(ctx, cfg, data)
}
