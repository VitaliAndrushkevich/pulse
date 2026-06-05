// Package version exposes build-time version information.
// Set via ldflags: -ldflags "-X github.com/VitaliAndrushkevich/pulse/internal/version.Version=1.2.3"
package version

// Version is injected at build time. Falls back to "dev" for local runs.
var Version = "dev"

// UserAgent returns the User-Agent string for HTTP checks.
func UserAgent() string {
	return "Pulse/" + Version
}
