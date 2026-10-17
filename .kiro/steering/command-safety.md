# Command safety: avoid hangs on unavailable dependencies

These rules prevent commands from hanging indefinitely when a dependency (database, network
service, container) is missing or unreachable. A hang wastes time and hides the real cause: the
dependency is down, not the code.

## Preflight: check a dependency before running anything that needs it

Before running a command that connects to an external dependency, verify the dependency is
reachable first with a fast, bounded check. If the check fails, STOP and report that the
dependency is unavailable — do not run the dependent command.

- PostgreSQL / TimescaleDB (default `localhost:5432`): `pg_isready -h localhost -p 5432 -t 3`
  (or `nc -z -w3 localhost 5432`). The project DB comes up via `make dev-local` (backend +
  Postgres/TimescaleDB) or `make dev` (full stack).
- Any TCP service: `nc -z -w3 <host> <port>` before connecting.
- Docker-backed dependencies: confirm the daemon is up (`docker info`) and the container is
  running (`docker ps`) before using it.

## Database-gated tests

The backend integration tests are gated behind `PULSE_RUN_DB_TESTS=1`. Without the flag they SKIP;
with the flag they attempt a real connection to `DATABASE_URL` and will BLOCK on the connection
timeout when no database is reachable.

- Run DB-gated tests ONLY after a successful `pg_isready` preflight.
- When no database is available, run the suite WITHOUT `PULSE_RUN_DB_TESTS` (tests skip cleanly)
  and state explicitly that integration tests were skipped — never claim they passed.
- Do not loop a DB-gated test command (e.g. `for i in 1 2 3 ...`) to chase flakes until the
  preflight passes; a loop over an unreachable DB multiplies the hang by the iteration count.

## Always bound commands that can block

- Add an explicit timeout to any command that can wait on I/O or a network peer. Prefer the
  execution tool's own timeout parameter. The GNU `timeout` wrapper is NOT installed by default on
  macOS (a bare `timeout ...` fails with "command not found" / exit 127); only use it when present
  (or `gtimeout` from coreutils) — otherwise rely on the tool timeout.
- Pass `-count=1` deliberately (it disables the Go test cache, so every run really executes and
  really re-attempts any connection) — combine it with a preflight so a cache-miss run cannot hang.
- Never pipe a long/blocking command's output through `grep` alone: if it hangs, the terminal
  shows nothing and the hang is invisible. Let output stream, or add a timeout.

## When something is stuck or cannot proceed

1. Stop the stuck command rather than waiting indefinitely (interrupt it; for a background process
   use the process-control tooling to stop it).
2. Diagnose the root cause with a bounded check (preflight above) instead of retrying blindly.
3. If the blocker is a missing dependency, report it plainly with the exact remediation
   (e.g. "Postgres is not reachable on :5432 — start it with `make dev-local`") and do not retry
   the dependent command until the dependency is confirmed up.
4. Do not keep re-running a failing/hanging command hoping it resolves. Two attempts of the same
   approach is the limit — then change the approach or surface the blocker.

## Long-running / interactive commands

Dev servers and watchers (`make dev`, `pnpm dev`, `*-watch`) do not terminate on their own. Run
them as background processes, or tell the user to run them manually — never as a blocking
foreground command in an automated step.

## Command construction: no stray `cd`, no broken tokens

Malformed command strings are a recurring problem. They come from leading no-op `cd`s and from
line breaks inserted inside a single command, which split tokens like `-count` / `=1` or a path
`/tmp/pu` / `lse.log` across lines and break the command.

- Set the working directory with the execution tool's `cwd` parameter, NOT with a leading `cd`.
  A `cd` does not persist between separate tool calls, so pass `cwd` on every call that needs it.
- Never emit a leading `cd <workspace> &&` that just lands where you already are. Chaining
  `cd A && cd B` is a smell — pass the final directory as `cwd` instead.
- When a command genuinely must start in a subdirectory for that one call, use a single
  `cd <dir> && <command>`; do not stack multiple `cd`s.
- Write each command as ONE unbroken line. Do not insert newlines inside a command, and never
  inside a token: `-count=1`, flag values, URLs, and file paths must never be split across lines.
  Newlines belong only inside a quoted string argument, never between the pieces of one flag or path.
- Keep env vars attached to the command that needs them (`env VAR=val cmd ...` or inline
  `VAR=val cmd`), on the same line as the command.
- Prefer redirecting long output to a file and reading the file back, rather than piping a
  blocking command through `grep` alone (see the blocking-command rule above).
