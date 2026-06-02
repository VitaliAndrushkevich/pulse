// Package migrations embeds all SQL migration files for use at application startup.
package migrations

import "embed"

// FS contains all .sql migration files embedded at compile time.
//
//go:embed *.sql
var FS embed.FS
