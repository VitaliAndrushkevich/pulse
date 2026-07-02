// Package resolve implements name-to-identifier resolution for Pulse resources.
// It handles UUID detection and paginated name lookups with case-sensitive matching.
package resolve

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vandrushkevich/pulse/mcp/internal/mcperr"
	"github.com/vandrushkevich/pulse/mcp/internal/pulseapi"
)

// uuidPattern matches UUID format: 8-4-4-4-12 hex characters with hyphens.
var uuidPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

// IsUUID reports whether ref is a valid UUID string (8-4-4-4-12 hex with hyphens).
func IsUUID(ref string) bool {
	return uuidPattern.MatchString(ref)
}

// Monitor resolves a monitor reference to a concrete monitor ID.
//
// If ref is a valid UUID, it is returned directly (the caller is expected to
// handle not-found from the downstream endpoint).
//
// Otherwise ref is treated as a name. The function pages through all monitors
// via client.ListMonitors and collects monitors whose Name equals ref under
// case-sensitive exact string comparison (Req 5.4).
//
//   - Exactly one match → returns its ID.
//   - Zero matches → returns a NOT_FOUND MCPError (Req 5.7).
//   - Two or more matches → returns an AMBIGUOUS_NAME MCPError listing all matching IDs (Req 5.5).
func Monitor(ctx context.Context, client pulseapi.PulseClient, ref string) (string, error) {
	if IsUUID(ref) {
		return ref, nil
	}

	var matchIDs []string
	page := 1
	const pageSize = 100

	for {
		result, err := client.ListMonitors(ctx, pulseapi.MonitorQuery{
			Page:  page,
			Limit: pageSize,
		})
		if err != nil {
			return "", err
		}

		for i := range result.Monitors {
			if result.Monitors[i].Name == ref {
				matchIDs = append(matchIDs, result.Monitors[i].ID)
			}
		}

		// No more pages to fetch.
		if page >= result.TotalPages || len(result.Monitors) == 0 {
			break
		}
		page++
	}

	switch len(matchIDs) {
	case 0:
		return "", mcperr.NotFound(fmt.Sprintf("no monitor found with name %q", ref))
	case 1:
		return matchIDs[0], nil
	default:
		return "", mcperr.AmbiguousName(
			fmt.Sprintf("name %q matches %d monitors: %s", ref, len(matchIDs), strings.Join(matchIDs, ", ")),
		)
	}
}
