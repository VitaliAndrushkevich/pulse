package smtp

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/VitaliAndrushkevich/pulse/internal/notification"
	"github.com/google/uuid"
)

// FormatSubject constructs the email subject line in the format:
// "[Pulse] {Monitor Name} - {Event Type}"
func FormatSubject(data notification.TemplateData) string {
	eventType := statusToEventType(data.Status)
	return fmt.Sprintf("[Pulse] %s - %s", data.Monitor.Name, eventType)
}

// statusToEventType maps the raw status string to a human-readable event type
// suitable for email subjects.
func statusToEventType(status string) string {
	switch strings.ToLower(status) {
	case "down":
		return "Down"
	case "up":
		return "Recovered"
	case "degraded":
		return "Degraded"
	case "ssl_expiring":
		return "SSL Expiring"
	case "n_failures_in_row":
		return "Consecutive Failures"
	default:
		return "Status Change"
	}
}

// emailTemplateData extends TemplateData with computed fields for rendering.
type emailTemplateData struct {
	MonitorName      string
	MonitorTarget    string
	MonitorURL       string // link to monitor in Pulse UI
	Status           string
	PreviousStatus   string
	ResponseTime     int32
	ShowResponseTime bool   // false when status is "down" (response time is meaningless)
	HasIncident      bool   // true when incident data is present (non-zero UUID)
	IncidentID       string
	StartedAt        string
	Duration         string
	Timestamp        string
	StatusColor      string
	StatusIcon       string
}

// RenderEmail renders the Pulse-branded HTML email template with the given data.
func RenderEmail(data notification.TemplateData) (string, error) {
	tmpl, err := template.New("email").Parse(emailHTMLTemplate)
	if err != nil {
		return "", fmt.Errorf("parse email template: %w", err)
	}

	var monitorURL string
	if data.BaseURL != "" {
		monitorURL = data.BaseURL + "/monitors/" + data.Monitor.ID.String()
	}

	hasIncident := data.Incident.ID != uuid.Nil

	td := emailTemplateData{
		MonitorName:      data.Monitor.Name,
		MonitorTarget:    data.Monitor.Target,
		MonitorURL:       monitorURL,
		Status:           data.Status,
		PreviousStatus:   data.PreviousStatus,
		ResponseTime:     data.ResponseTime,
		ShowResponseTime: strings.ToLower(data.Status) != "down",
		HasIncident:      hasIncident,
		IncidentID:       data.Incident.ID.String(),
		StartedAt:        data.Incident.StartedAt.Format(time.RFC3339),
		Duration:         formatDuration(data.Incident.Duration),
		Timestamp:        data.Timestamp.Format(time.RFC3339),
		StatusColor:      statusColor(data.Status),
		StatusIcon:       statusIcon(data.Status),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, td); err != nil {
		return "", fmt.Errorf("execute email template: %w", err)
	}

	return buf.String(), nil
}

// statusColor returns the hex color associated with a given status.
func statusColor(status string) string {
	switch strings.ToLower(status) {
	case "down":
		return "#ef4444" // red-500
	case "up":
		return "#22c55e" // green-500
	case "degraded":
		return "#f59e0b" // amber-500
	case "ssl_expiring":
		return "#f97316" // orange-500
	default:
		return "#0ea5e9" // brand primary (sky-500)
	}
}

// statusIcon returns a simple text-based status indicator.
func statusIcon(status string) string {
	switch strings.ToLower(status) {
	case "down":
		return "⚠"
	case "up":
		return "✓"
	case "degraded":
		return "⚡"
	case "ssl_expiring":
		return "🔒"
	default:
		return "●"
	}
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "N/A"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// emailHTMLTemplate is the Pulse-branded HTML email template with ECG motif,
// brand colors, and clean layout. All styles are inlined for email client
// compatibility.
const emailHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Pulse Notification</title>
</head>
<body style="margin:0;padding:0;background-color:#f8fafc;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#f8fafc;padding:24px 0;">
<tr>
<td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,0.1);">
<!-- ECG Header Motif -->
<tr>
<td style="background: linear-gradient(135deg, #0ea5e9, #0284c7); padding: 0; height: 6px;">
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 600 6" preserveAspectRatio="none" style="display:block;width:100%;height:6px;">
<path d="M0,3 L150,3 L170,0 L180,6 L190,1 L200,5 L210,3 L600,3" fill="none" stroke="rgba(255,255,255,0.6)" stroke-width="1.5"/>
</svg>
</td>
</tr>
<!-- Logo and Brand -->
<tr>
<td style="padding:28px 32px 12px;text-align:center;background:linear-gradient(135deg, #0ea5e9, #0284c7);">
<p style="margin:0;font-size:24px;font-weight:700;color:#ffffff;letter-spacing:0.5px;">Pulse</p>
</td>
</tr>
<!-- Status Banner -->
<tr>
<td style="padding:20px 32px;text-align:center;background:linear-gradient(135deg, #0ea5e9, #0284c7);">
<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 auto;background-color:{{.StatusColor}};border-radius:6px;padding:12px 24px;">
<tr>
<td style="font-size:16px;font-weight:600;color:#ffffff;">
{{.StatusIcon}} {{.MonitorName}} is {{.Status}}
</td>
</tr>
</table>
</td>
</tr>
<!-- Content Body -->
<tr>
<td style="padding:32px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0">
<!-- Monitor Details Section -->
<tr>
<td style="padding-bottom:24px;">
<h2 style="margin:0 0 16px;font-size:16px;font-weight:600;color:#1e293b;">Monitor Details</h2>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid #e2e8f0;border-radius:6px;overflow:hidden;">
<tr>
<td style="padding:12px 16px;background-color:#f8fafc;border-bottom:1px solid #e2e8f0;width:140px;">
<span style="font-size:13px;font-weight:500;color:#64748b;">Monitor</span>
</td>
<td style="padding:12px 16px;border-bottom:1px solid #e2e8f0;">
<span style="font-size:14px;font-weight:600;color:#1e293b;">{{.MonitorName}}</span>
</td>
</tr>
<tr>
<td style="padding:12px 16px;background-color:#f8fafc;border-bottom:1px solid #e2e8f0;width:140px;">
<span style="font-size:13px;font-weight:500;color:#64748b;">Target URL</span>
</td>
<td style="padding:12px 16px;border-bottom:1px solid #e2e8f0;">
<span style="font-size:14px;color:#1e293b;">{{.MonitorTarget}}</span>
</td>
</tr>
<tr>
<td style="padding:12px 16px;background-color:#f8fafc;border-bottom:1px solid #e2e8f0;width:140px;">
<span style="font-size:13px;font-weight:500;color:#64748b;">Status</span>
</td>
<td style="padding:12px 16px;border-bottom:1px solid #e2e8f0;">
<span style="font-size:14px;font-weight:600;color:{{.StatusColor}};">{{.Status}}</span>
{{if .PreviousStatus}}<span style="font-size:13px;color:#64748b;"> (was {{.PreviousStatus}})</span>{{end}}
</td>
</tr>
{{if .ShowResponseTime}}
<tr>
<td style="padding:12px 16px;background-color:#f8fafc;width:140px;">
<span style="font-size:13px;font-weight:500;color:#64748b;">Response Time</span>
</td>
<td style="padding:12px 16px;">
<span style="font-size:14px;color:#1e293b;">{{.ResponseTime}}ms</span>
</td>
</tr>
{{end}}
</table>
</td>
</tr>
<!-- Incident Details Section -->
{{if .HasIncident}}
<tr>
<td style="padding-bottom:24px;">
<h2 style="margin:0 0 16px;font-size:16px;font-weight:600;color:#1e293b;">Incident Details</h2>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid #e2e8f0;border-radius:6px;overflow:hidden;">
<tr>
<td style="padding:12px 16px;background-color:#f8fafc;border-bottom:1px solid #e2e8f0;width:140px;">
<span style="font-size:13px;font-weight:500;color:#64748b;">Incident ID</span>
</td>
<td style="padding:12px 16px;border-bottom:1px solid #e2e8f0;">
<code style="font-size:13px;color:#1e293b;background-color:#f1f5f9;padding:2px 6px;border-radius:3px;">{{.IncidentID}}</code>
</td>
</tr>
<tr>
<td style="padding:12px 16px;background-color:#f8fafc;border-bottom:1px solid #e2e8f0;width:140px;">
<span style="font-size:13px;font-weight:500;color:#64748b;">Started At</span>
</td>
<td style="padding:12px 16px;border-bottom:1px solid #e2e8f0;">
<span style="font-size:14px;color:#1e293b;">{{.StartedAt}}</span>
</td>
</tr>
<tr>
<td style="padding:12px 16px;background-color:#f8fafc;width:140px;">
<span style="font-size:13px;font-weight:500;color:#64748b;">Duration</span>
</td>
<td style="padding:12px 16px;">
<span style="font-size:14px;color:#1e293b;">{{.Duration}}</span>
</td>
</tr>
</table>
</td>
</tr>
{{end}}
<!-- View Monitor Button (only when BaseURL is configured) -->
{{if .MonitorURL}}
<tr>
<td style="padding-bottom:24px;text-align:center;">
<a href="{{.MonitorURL}}" target="_blank" style="display:inline-block;background-color:#0ea5e9;color:#ffffff;font-size:14px;font-weight:600;text-decoration:none;padding:12px 28px;border-radius:6px;mso-padding-alt:0;">
<!--[if mso]><i style="mso-font-width:150%;mso-text-raise:18px;">&#160;</i><![endif]-->
View Monitor
<!--[if mso]><i style="mso-font-width:150%;">&#160;</i><![endif]-->
</a>
</td>
</tr>
{{end}}
</table>
</td>
</tr>
<!-- Footer with ECG motif -->
<tr>
<td style="padding:16px 32px;background-color:#f8fafc;border-top:1px solid #e2e8f0;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0">
<tr>
<td style="text-align:center;">
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 12" style="display:block;margin:0 auto 8px;width:200px;height:12px;">
<path d="M0,6 L50,6 L60,2 L65,10 L70,4 L75,8 L80,6 L200,6" fill="none" stroke="#0ea5e9" stroke-width="1" stroke-opacity="0.4"/>
</svg>
<p style="margin:0;font-size:12px;color:#94a3b8;">
Sent by Pulse Monitoring &bull; {{.Timestamp}}
</p>
</td>
</tr>
</table>
</td>
</tr>
</table>
</td>
</tr>
</table>
</body>
</html>`
