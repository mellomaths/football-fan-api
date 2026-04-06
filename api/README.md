# Football Fan API (Go)

HTTP API that reads **teams** and **scheduled matches** from PostgreSQL. The server applies embedded SQL migrations on startup, then listens for requests.

## How to run

### Environment variables

| Variable       | Required | Default | Description                                                                 |
| -------------- | -------- | ------- | --------------------------------------------------------------------------- |
| `DATABASE_URL` | Yes      | —       | PostgreSQL URL (for example `postgres://user:pass@host:5432/dbname?sslmode=disable`) |
| `HTTP_ADDR`    | No       | `:8080` | Listen address (`host:port` or `:port`)                                     |

Copy [.env.example](.env.example) to `.env` in this directory, edit `DATABASE_URL`, then load it in your shell before running:

```bash
set -a && source .env && set +a   # bash
```

### Local (Go installed)

Ensure PostgreSQL is running and a database exists. See the root [README.md](../README.md#local-postgresql-setup) for `createdb` / `psql` setup.

Example:

```bash
export DATABASE_URL="postgres://football:football@localhost:5432/football?sslmode=disable"
cd api
go run ./cmd/server
```

The process logs JSON to stdout and listens on `HTTP_ADDR`.

### Docker

Full stack (Postgres, scraper, pgAdmin) from the repo root: see [README.md](../README.md#how-to-run-the-whole-project-docker) and the root [Justfile](../Justfile) (`just up`, `just restart api`, …).

Build and run **only** the API image when you supply your own Postgres:

```bash
docker build -t football-api ./api
docker run --rm -e DATABASE_URL="postgres://..." -p 8080:8080 football-api
```

## How the API works

### Routing

The server uses Go 1.22 `net/http` `ServeMux` with method-specific patterns:

- `GET /healthz` — liveness
- `GET /teams` — list registered teams
- `GET /teams/{teamId}` — one team by id (metadata from `teams` row)
- `GET /teams/{teamId}/matches` — matches for a team in a date window

Path parameters are read with `Request.PathValue("teamId")`.

### Endpoints

#### `GET /healthz`

Returns `200` and a small JSON body, e.g. `{"status":"ok"}`.

#### `GET /teams`

Returns a JSON array of teams, ordered by primary competition code then name (same row shape as `GET /teams/{teamId}`). Each item includes:

- `id` — stable integer id (assigned when the club row is first created, usually by a scraper)
- `name` — display name
- `short_name`, `espn_slug`, `soccerway_id` — strings from the `teams` row when set; omitted when null or empty in the database

Example:

```bash
curl -s http://localhost:8080/teams
```

#### `GET /teams/{teamId}`

Returns **`200`** and a JSON object:

- `id` — team id  
- `name` — display name  
- `short_name`, `espn_slug`, `soccerway_id` — strings from the `teams` row when set; omitted when null or empty in the database

Example:

```bash
curl -s http://localhost:8080/teams/7
```

Errors:

- `400` — invalid `teamId`
- `404` — unknown team id
- `500` — database or internal failure

#### `GET /teams/{teamId}/matches`

Query parameters **`from`** and **`to`** are **required**. Both must be calendar dates in **`YYYY-MM-DD`** interpreted in **UTC**.

Rules:

1. `from` and `to` must parse as dates.
2. `from` must be on or before `to`.
3. The span between the two dates (in whole days, same idea as PostgreSQL date subtraction) must be **at most 31 days**. For example, `2026-04-01` through `2026-04-30` is allowed; a 32-day gap is rejected.

**Match window:** Kickoff times are filtered with `kickoff_utc >= from` at `00:00:00 UTC` and `kickoff_utc < (to + 1 day)` at `00:00:00 UTC`, so the entire `to` day is included.

Response: JSON array of matches where the given team is home or away, **across all competitions** in the window (Série A, cups, etc.). Each element has:

- `id` — match id  
- `kickoff_utc` — RFC3339 timestamp in UTC  
- `location` — optional object `{ "name": "<venue>" }` when a venue is stored  
- `home` / `away` — objects with `id`, `name`, and optional `short_name`, `espn_slug`, `soccerway_id`  
- `competition` — object with `id`, `name`, and `code` (e.g. `BRASILEIRAO_A`)

Example:

```bash
curl -s "http://localhost:8080/teams/1/matches?from=2026-04-01&to=2026-04-30"
```

Errors:

- `400` — missing/invalid dates, invalid `teamId`, or validation message in JSON `{"error":"..."}`
- `404` — unknown `teamId`
- `500` — database or internal failure

### Migrations

On startup, the binary ensures schema `footballfan` exists, creates `footballfan.schema_migrations`, then runs all embedded `.sql` files under `internal/migrate/sql/` in order. The API will not serve traffic usefully until migrations succeed.

### Project layout (API)

| Path                   | Purpose                                                         |
| ---------------------- | --------------------------------------------------------------- |
| `cmd/server/main.go`   | Entry: config, pool, migrate, HTTP server, graceful shutdown    |
| `internal/httpapi/`    | Handlers and mux                                                |
| `internal/db/`         | Queries and JSON DTOs                                           |
| `internal/validate/`   | Date-range validation for matches                               |
| `internal/migrate/`    | Embedded SQL migrations                                         |

### Tests

```bash
cd api
go test ./...
```
