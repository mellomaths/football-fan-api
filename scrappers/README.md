# Football scrapers (Python)

Package **`football_scrapers`** pulls fixture data from public web sources (currently **ESPN** JSON APIs and **Soccerway** HTML), normalizes it, and **upserts** rows into PostgreSQL schema **`footballfan`** (same tables as the Go API migrations).

Dependencies are managed with **[uv](https://docs.astral.sh/uv/)** using [`pyproject.toml`](pyproject.toml) and a committed **[`uv.lock`](uv.lock)** for reproducible installs (local and Docker).

## How to run

### Prerequisites

- Python 3.11+
- **[uv](https://docs.astral.sh/uv/getting-started/installation/)** installed (`curl -LsSf https://astral.sh/uv/install.sh | sh` on Unix, or see the docs for Windows and package managers)
- A reachable PostgreSQL database already migrated (easiest: start the **API** once so migrations apply, or run the same SQL manually)

Copy [.env.example](.env.example) to `.env` in this directory and set `DATABASE_URL`, or export variables manually. For creating a local database with `psql` / `createdb`, see the root [README.md](../README.md#local-postgresql-setup).

### Install dependencies

From the `scrappers/` directory:

```bash
uv sync
```

This creates `.venv/`, installs runtime dependencies, and includes the **`dev`** group (e.g. `pytest`). For a minimal environment without dev tools (similar to the Docker image), use:

```bash
uv sync --no-dev
```

After changing dependencies in `pyproject.toml`, refresh the lockfile and environment:

```bash
uv lock
uv sync
```

### Environment variables

#### Required

| Variable       | Description                                    |
| -------------- | ---------------------------------------------- |
| `DATABASE_URL` | PostgreSQL connection string (same as the API) |

#### Scheduling (after the first run)

The process **always runs one full scrape immediately**, then starts the scheduler.

| Variable                 | Description                                                                                                                                 |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `SCRAPER_CRON`           | If set (non-empty), **wins** over interval. Standard **5-field cron** in **UTC**, e.g. `0 3 * * *` for 03:00 UTC daily.                     |
| `SCRAPER_INTERVAL_HOURS` | If `SCRAPER_CRON` is unset, repeat every this many hours (integer).                                                                       |
| *(default)*              | If neither is set, repeats every **24** hours.                                                                                            |

#### General

| Variable                      | Default | Description                                      |
| ----------------------------- | ------- | ------------------------------------------------ |
| `LOG_LEVEL`                   | `INFO`  | Python logging level name (`DEBUG`, `INFO`, …)   |
| `SCRAPER_BETWEEN_SOURCES_SEC` | `2`     | Pause between ESPN and Soccerway passes          |

#### HTTP client

| Variable                   | Default                  | Description                           |
| -------------------------- | ------------------------ | ------------------------------------- |
| `SCRAPER_USER_AGENT`       | project default string   | `User-Agent` header for outbound HTTP |
| `SCRAPER_HTTP_TIMEOUT_SEC` | `30`                     | Request timeout in seconds            |

#### ESPN (`espn` module)

| Variable                       | Default | Description                                            |
| ------------------------------ | ------- | ------------------------------------------------------ |
| `SCRAPER_HORIZON_DAYS`         | `45`    | How far ahead from today to request scoreboard windows |
| `SCRAPER_ESPN_DATE_CHUNK_DAYS` | `7`     | Each API call covers this many days (batched range)    |
| `SCRAPER_ESPN_SLEEP_SEC`       | `0.25`  | Delay after each chunk to reduce request rate          |

ESPN path segments (e.g. `bra.1`, `bra.2`, `conmebol.libertadores`, `conmebol.sudamericana`, `bra.copa`) are mapped in code to internal **competition codes** (`BRASILEIRAO_A`, `COPA_LIBERTADORES`, …). Adjust mappings in `espn.py` if ESPN changes URLs or you add competitions.

#### Soccerway (`soccerway` module)

| Variable                      | Description                                 |
| ----------------------------- | ------------------------------------------- |
| `SOCCERWAY_URL_BRASILEIRAO_A` | Override default fixtures URL for Série A |
| `SOCCERWAY_URL_BRASILEIRAO_B` | Override default fixtures URL for Série B |

Defaults point at Soccerway “fixtures” pages; HTML structure can change by season—override URLs if parsing returns no rows.

### Command line

```bash
export DATABASE_URL="postgres://football:football@localhost:5432/football?sslmode=disable"
uv run python -m football_scrapers
```

`uv run` executes inside the project virtual environment without activating it manually.

Docker (from repository root, with Compose networking):

```bash
docker compose run --rm scraper
```

The scraper image runs `uv sync --frozen --no-dev` at build time so installs match `uv.lock` exactly.

## How the scrapers work

### Pipeline

1. **Connect** to Postgres and build a **team lookup** from `teams` + `team_competitions` + `competitions`:
   per **competition code** plus a **global** normalized-name map so cup fixtures can match clubs first
   seen in a national league.
2. **ESPN:** For each configured ESPN slug (national leagues first in the map, then cups), request the **scoreboard** API for sliding date ranges up to `SCRAPER_HORIZON_DAYS`. Parse events, home/away display names, kickoff time, optional venue. `external_match_id` is scoped by competition to avoid collisions; `source = espn`. Missing clubs are **created** (`teams` + `team_competitions`) for all mapped competitions (leagues and cups).
3. **Pause** (`SCRAPER_BETWEEN_SOURCES_SEC`).
4. **Soccerway:** Fetch configured fixture pages, parse HTML tables (best-effort). Build stable ids from links or a fallback string; `source = soccerway`. For Série A/B, unknown names are **auto-created** like ESPN.
5. For each parsed row, **resolve or create** home/away to internal `team_id` values; skip rows that cannot be matched when auto-create is off.
6. **Upsert** into `matches` with `ON CONFLICT (source, external_match_id) DO UPDATE` so repeated runs refresh kickoff/venue/participants.

### Team name matching

There is **no SQL seed list of clubs**. Resolution tries the **event’s competition code** first (via `team_competitions` memberships), then **global** name matching. **Série A/B** links are treated as **primary** when applied (`is_primary`), so a later cup scrape does not replace domestic league as the default row for `GET /teams`. New links are inserted when a fixture is scraped.

- Normalized folding (Unicode NFKD, strip accents, lowercase, collapse spaces), and
- Plain lowercase aliases.

Unmatched names are logged as warnings; those fixtures are skipped. To support **estaduais** or new tournaments, add a row to `competitions` and extend scraper URL/slug maps.

### Tests

```bash
cd scrappers
uv sync
uv run pytest -q
```

Parser tests use **saved HTML fixtures** under `tests/fixtures/` (no network in CI).

### Package layout

| Location                             | Role                              |
| ------------------------------------ | --------------------------------- |
| `pyproject.toml`                     | Project metadata and dependencies |
| `uv.lock`                            | Locked versions (commit this file) |
| `src/football_scrapers/espn.py`      | ESPN JSON integration             |
| `src/football_scrapers/soccerway.py` | Soccerway HTML parsing            |
| `src/football_scrapers/storage.py`   | DB load + match upsert            |
| `src/football_scrapers/http_client.py` | Shared client, retries, backoff |
| `src/football_scrapers/normalize.py` | Name normalization                |
| `src/football_scrapers/__main__.py`  | Entry: one run + APScheduler loop |
