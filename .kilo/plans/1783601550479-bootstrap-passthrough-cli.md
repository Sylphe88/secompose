# secret-compose: Secret Hydration Plan

## Current State

The CLI is already implemented and working:
- `cmd/secret-compose/main.go` — entry point
- `internal/runner/runner.go` — reads compose file, pipes to `docker compose -f -`
- `internal/compose/args.go` — `-f`/`--file` flag parsing and rewriting

The runner already reads the compose file into memory. The next step is to hydrate it before piping.

---

## Goal

Scan the in-memory compose file for `${lookup(...)}` expressions, resolve each secret from the appropriate backend (Vault first), and substitute the resolved values before passing the file to docker compose.

---

## Syntax

```
${lookup(backend, path, key)}
${lookup(backend, path, key, version=N)}
```

| Argument | Required | Notes |
|---|---|---|
| `backend` | yes | e.g. `vault`. Registry maps name to `Backend` impl. |
| `path` | yes (for Vault) | First segment = KV mount, rest = path within mount. e.g. `secret/db/prod` → mount `secret`, path `db/prod` |
| `key` | yes (for Vault) | Field name inside the secret |
| `version=N` | no | Positive integer. If malformed: warn to stderr, default to latest. If absent: latest. |

---

## Design Decisions

| Decision | Value |
|---|---|
| Substitution | Regex over raw file text (`ReplaceAllFunc`) |
| Scope | All occurrences in file, including multiple per line |
| Concurrency | 4-goroutine bounded worker pool |
| Deduplication | Identical `(backend, path, key, version)` tuples resolved once, result reused |
| Error behaviour | Fail fast — first error cancels all workers, exits non-zero, docker compose never called |
| Vault auth | `VAULT_ADDR` + `VAULT_TOKEN` from env vars |
| Vault API | KV v2: `GET /v1/{mount}/data/{path}?version={n}` |
| Malformed version | Warn to stderr, default to latest |
| Missing `VAULT_ADDR`/`VAULT_TOKEN` | Fail fast at backend init, before any Vault call |

---

## New Files

### `internal/backend/backend.go`
- `Backend` interface:
  ```go
  type Backend interface {
      Lookup(ctx context.Context, path, key string, version int) (string, error)
  }
  ```
- `version=0` means latest

### `internal/backend/vault.go`
- `Vault` struct holding `addr` and `token` (read from env at construction)
- `New() (*Vault, error)` — reads `VAULT_ADDR` + `VAULT_TOKEN`, errors if either missing
- `Lookup` implementation:
  - Splits path on `/`: first segment = mount, remainder = subpath
  - Calls `GET {VAULT_ADDR}/v1/{mount}/data/{subpath}?version={n}` (omit `?version` when 0)
  - Extracts `data.data.{key}` from JSON response
  - Returns value as string

### `internal/backend/registry.go`
- `map[string]func() (Backend, error)` — maps `"vault"` → `vault.New`
- `Get(name string) (Backend, error)` — returns registered backend or error

### `internal/compose/hydrate.go`
- Regex: `` `\$\{lookup\(\s*(\w+)\s*,\s*([^,)]+)\s*,\s*([^,)]+)\s*(?:,\s*version=(\d+[^)]*))?\s*\)}` ``
- `Hydrate(ctx context.Context, data []byte) ([]byte, error)`:
  1. Find all matches, deduplicate by `(backend, path, key, version)` tuple
  2. Validate and parse `version` — warn + default to 0 if malformed
  3. Dispatch all unique lookups through a 4-goroutine worker pool
  4. On first error: cancel context, drain workers, return error
  5. On success: single `ReplaceAllFunc` pass substituting all matches from resolved map
  6. Return hydrated bytes

---

## Changes to Existing Files

### `internal/runner/runner.go`
- After `os.ReadFile`, call `compose.Hydrate(ctx, data)`
- On error: print to stderr, return exit code 1
- On success: pipe hydrated bytes to docker compose as before

---

## Failure Modes

| Condition | Behaviour |
|---|---|
| `VAULT_ADDR` or `VAULT_TOKEN` missing | Error before any lookup, exit 1 |
| Vault unreachable | Fail fast, exit 1 |
| Secret path not found (404) | Fail fast, exit 1 |
| Key missing from secret data | Fail fast, exit 1 |
| Permission denied (403) | Fail fast, exit 1 |
| Malformed `version=abc` | Warn to stderr, use latest |
| Unknown backend name | Fail fast, exit 1 |
| No `${lookup(...)}` in file | No-op, file passed through as-is |

---

## Validation

```bash
# Build
go build -o build/secret-compose ./cmd/secret-compose

# Compose file with lookup:
# environment:
#   DB_PASS: ${lookup(vault, secret/db/prod, password)}

VAULT_ADDR=http://localhost:8200 VAULT_TOKEN=mytoken ./build/secret-compose config
# → resolved value appears in output

# Multiple lookups per line
# DB_URL: "postgres://${lookup(vault, secret/db, user)}:${lookup(vault, secret/db, password)}@host"
# → both resolved correctly

# Malformed version
# ${lookup(vault, secret/db, password, version=abc)}
# → warning on stderr, latest fetched

# Missing VAULT_TOKEN
VAULT_ADDR=http://localhost:8200 ./build/secret-compose config
# → "secret-compose: VAULT_TOKEN is not set" on stderr, exit 1

# No lookups in file
# → file passed through unchanged, docker compose behaves normally
```

---

## Out of Scope

- Additional backends (Infisical, SOPS) — future work
- AppRole / Kubernetes / AWS auth for Vault — future work
- Tests — future work
