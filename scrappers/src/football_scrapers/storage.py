from __future__ import annotations

import logging
from dataclasses import dataclass, field
from datetime import datetime, timezone

import psycopg
from psycopg import errors as pg_errors
from psycopg import sql
from psycopg.rows import dict_row

from football_scrapers.normalize import normalize_team_name
from football_scrapers.schema import table

log = logging.getLogger(__name__)


@dataclass
class TeamLookup:
    """Teams keyed by primary competition code, plus a global map for cup / cross-competition fixtures."""

    by_competition_code: dict[str, dict[str, int]] = field(default_factory=dict)
    by_normalized_name: dict[str, int] = field(default_factory=dict)


class Storage:
    """Loads team ids from DB and upserts matches."""

    def __init__(self, dsn: str) -> None:
        self._dsn = dsn

    def connect(self) -> psycopg.Connection:
        return psycopg.connect(self._dsn, row_factory=dict_row)

    def load_teams(self, conn: psycopg.Connection) -> TeamLookup:
        """Map competition_code -> normalized_team_name -> id, plus global name -> id for cups."""
        lookup = TeamLookup()
        with conn.cursor() as cur:
            cur.execute(
                sql.SQL("""
                SELECT t.id, t.name, c.code AS competition_code
                FROM {} t
                JOIN {} tc ON tc.team_id = t.id
                JOIN {} c ON c.id = tc.competition_id
                """).format(table("teams"), table("team_competitions"), table("competitions"))
            )
            for row in cur.fetchall():
                assert isinstance(row, dict)
                lid = int(row["id"])
                name = str(row["name"])
                code = str(row["competition_code"])
                bucket = lookup.by_competition_code.setdefault(code, {})
                bucket[normalize_team_name(name)] = lid
                bucket[name.strip().lower()] = lid
                lookup.by_normalized_name[normalize_team_name(name)] = lid
                lookup.by_normalized_name[name.strip().lower()] = lid
        return lookup

    def upsert_match(
        self,
        conn: psycopg.Connection,
        *,
        competition_id: int,
        home_team_id: int,
        away_team_id: int,
        kickoff_utc: datetime,
        venue: str | None,
        source: str,
        external_match_id: str,
    ) -> None:
        with conn.cursor() as cur:
            cur.execute(
                sql.SQL("""
                INSERT INTO {} (
                    competition_id, home_team_id, away_team_id, kickoff_utc, venue,
                    source, external_match_id, scraped_at
                )
                VALUES (%s, %s, %s, %s, %s, %s, %s, now())
                ON CONFLICT (source, external_match_id) DO UPDATE SET
                    competition_id = EXCLUDED.competition_id,
                    home_team_id = EXCLUDED.home_team_id,
                    away_team_id = EXCLUDED.away_team_id,
                    kickoff_utc = EXCLUDED.kickoff_utc,
                    venue = EXCLUDED.venue,
                    scraped_at = EXCLUDED.scraped_at
                """).format(table("matches")),
                (
                    competition_id,
                    home_team_id,
                    away_team_id,
                    kickoff_utc.astimezone(timezone.utc),
                    venue,
                    source,
                    external_match_id,
                ),
            )


def resolve_team_id(
    lookup: TeamLookup,
    competition_code: str,
    display_name: str,
) -> int | None:
    """Resolve a scraped team name to our team id (primary competition bucket, then global)."""
    key = normalize_team_name(display_name)
    low = display_name.strip().lower()
    b = lookup.by_competition_code.get(competition_code, {})
    if key in b:
        return b[key]
    if low in b:
        return b[low]
    if key in lookup.by_normalized_name:
        return lookup.by_normalized_name[key]
    if low in lookup.by_normalized_name:
        return lookup.by_normalized_name[low]
    return None


def upsert_team_metadata(
    conn: psycopg.Connection,
    team_id: int,
    *,
    short_name: str | None = None,
    espn_slug: str | None = None,
    soccerway_id: str | None = None,
) -> None:
    """Set team metadata from scrapers; non-empty values overwrite existing columns."""
    parts: list[sql.SQL] = []
    vals: list[object] = []
    if short_name is not None and (s := short_name.strip()):
        parts.append(sql.SQL("short_name = %s"))
        vals.append(s[:64])
    if espn_slug is not None and (e := espn_slug.strip()):
        parts.append(sql.SQL("espn_slug = %s"))
        vals.append(e[:128])
    if soccerway_id is not None and (w := soccerway_id.strip()):
        parts.append(sql.SQL("soccerway_id = %s"))
        vals.append(w[:128])
    if not parts:
        return
    vals.append(team_id)
    with conn.cursor() as cur:
        cur.execute(
            sql.SQL("UPDATE {} SET {} WHERE id = %s").format(
                table("teams"),
                sql.SQL(", ").join(parts),
            ),
            vals,
        )


# Série A/B take precedence as "primary" in the API when linked (even if a cup row existed first).
_DOMESTIC_LEAGUES = frozenset({"BRASILEIRAO_A", "BRASILEIRAO_B"})


def _clear_primary_for_team(cur: psycopg.Cursor, team_id: int) -> None:
    """At most one row per team may have is_primary (partial unique index)."""
    cur.execute(
        sql.SQL("UPDATE {} SET is_primary = false WHERE team_id = %s").format(table("team_competitions")),
        (team_id,),
    )


def link_team_competition(
    conn: psycopg.Connection,
    team_id: int,
    competition_id: int,
    *,
    competition_code: str,
) -> None:
    """Ensure team_id is linked to competition_id; set is_primary per competition tier."""
    domestic = competition_code in _DOMESTIC_LEAGUES
    try:
        with conn.cursor() as cur:
            if domestic:
                _clear_primary_for_team(cur, team_id)
                cur.execute(
                    sql.SQL("""
                    INSERT INTO {} (team_id, competition_id, is_primary)
                    VALUES (%s, %s, true)
                    ON CONFLICT (team_id, competition_id) DO UPDATE SET is_primary = true
                    """).format(table("team_competitions")),
                    (team_id, competition_id),
                )
                log.info(
                    "team_competition linked (domestic) team_id=%s competition_id=%s code=%s rowcount=%s",
                    team_id,
                    competition_id,
                    competition_code,
                    cur.rowcount,
                )
                return

            # Cup / international: re-scrapes call link every fixture. If this pair already exists,
            # skip — do not recompute is_primary from COUNT() (that would falsely demote the only primary row).
            cur.execute(
                sql.SQL("SELECT 1 FROM {} WHERE team_id = %s AND competition_id = %s LIMIT 1").format(
                    table("team_competitions")
                ),
                (team_id, competition_id),
            )
            if cur.fetchone():
                log.debug(
                    "team_competition already present, skip team_id=%s competition_id=%s code=%s",
                    team_id,
                    competition_id,
                    competition_code,
                )
                return

            cur.execute(
                sql.SQL("SELECT COUNT(*)::int AS n FROM {} WHERE team_id = %s").format(table("team_competitions")),
                (team_id,),
            )
            row = cur.fetchone()
            n = int(row["n"]) if row else 0  # type: ignore[index]
            is_primary = n == 0
            # Setting is_primary=true must not violate uq_team_competitions_one_primary (one true per team).
            if is_primary:
                _clear_primary_for_team(cur, team_id)
            cur.execute(
                sql.SQL("""
                INSERT INTO {} (team_id, competition_id, is_primary)
                VALUES (%s, %s, %s)
                ON CONFLICT (team_id, competition_id) DO NOTHING
                """).format(table("team_competitions")),
                (team_id, competition_id, is_primary),
            )
            log.info(
                "team_competition linked (cup/intl) team_id=%s competition_id=%s code=%s is_primary=%s rowcount=%s",
                team_id,
                competition_id,
                competition_code,
                is_primary,
                cur.rowcount,
            )
    except pg_errors.UniqueViolation as exc:
        log.error(
            "team_competition insert failed (unique) team_id=%s competition_id=%s code=%s: %s",
            team_id,
            competition_id,
            competition_code,
            exc.diag.message_detail or exc.diag.message_primary or str(exc),
        )
        raise
    except pg_errors.ForeignKeyViolation as exc:
        log.error(
            "team_competition insert failed (fk) team_id=%s competition_id=%s code=%s: %s",
            team_id,
            competition_id,
            competition_code,
            exc.diag.message_detail or str(exc),
        )
        raise


def register_team(lookup: TeamLookup, competition_code: str, name: str, team_id: int) -> None:
    """Add a team id to lookup maps (e.g. after INSERT)."""
    bucket = lookup.by_competition_code.setdefault(competition_code, {})
    bucket[normalize_team_name(name)] = team_id
    bucket[name.strip().lower()] = team_id
    lookup.by_normalized_name[normalize_team_name(name)] = team_id
    lookup.by_normalized_name[name.strip().lower()] = team_id


def ensure_team_id(
    conn: psycopg.Connection,
    lookup: TeamLookup,
    *,
    competition_id: int,
    competition_code: str,
    display_name: str,
    short_name: str | None,
    espn_slug: str | None = None,
    soccerway_id: str | None = None,
    auto_create: bool,
) -> int | None:
    """
    Resolve team id; when auto_create, insert a teams row (unique name) and team_competitions link.
    Used for cups (foreign clubs) and for national leagues when the DB has no SQL seed.
    """
    tid = resolve_team_id(lookup, competition_code, display_name)
    if tid is not None:
        upsert_team_metadata(
            conn,
            tid,
            short_name=short_name,
            espn_slug=espn_slug,
            soccerway_id=soccerway_id,
        )
        link_team_competition(conn, tid, competition_id, competition_code=competition_code)
        return tid
    if not auto_create:
        log.warning("unmatched team %r for competition %s", display_name, competition_code)
        return None
    name = display_name.strip()
    if not name:
        return None
    sn = short_name.strip() if short_name else None
    if sn == "":
        sn = None
    es = espn_slug.strip() if espn_slug else None
    if es == "":
        es = None
    sw = soccerway_id.strip() if soccerway_id else None
    if sw == "":
        sw = None
    with conn.cursor() as cur:
        cur.execute(
            sql.SQL("""
            INSERT INTO {} AS t (name, short_name, espn_slug, soccerway_id)
            VALUES (%s, %s, %s, %s)
            ON CONFLICT (name) DO UPDATE SET
                short_name = COALESCE(EXCLUDED.short_name, t.short_name),
                espn_slug = COALESCE(EXCLUDED.espn_slug, t.espn_slug),
                soccerway_id = COALESCE(EXCLUDED.soccerway_id, t.soccerway_id)
            RETURNING t.id
            """).format(table("teams")),
            (name, sn, es, sw),
        )
        row = cur.fetchone()
        if not row:
            log.error("could not upsert team %r", name)
            return None
        new_id = int(row["id"])
    link_team_competition(conn, new_id, competition_id, competition_code=competition_code)
    register_team(lookup, competition_code, name, new_id)
    log.info("created team %r id=%s for competition %s", name, new_id, competition_code)
    return new_id


def list_teams_with_ticket_sale_url(conn: psycopg.Connection) -> list[tuple[int, str, str]]:
    """Return (team_id, ticket_sale_url, name) for teams with a non-empty ticket_sale_url."""
    out: list[tuple[int, str, str]] = []
    with conn.cursor() as cur:
        cur.execute(
            sql.SQL("""
            SELECT id, name, ticket_sale_url
            FROM {}
            WHERE ticket_sale_url IS NOT NULL AND btrim(ticket_sale_url) <> ''
            ORDER BY id
            """).format(table("teams"))
        )
        for row in cur.fetchall():
            assert isinstance(row, dict)
            tid = int(row["id"])
            name = str(row["name"])
            url = str(row["ticket_sale_url"]).strip()
            out.append((tid, url, name))
    return out


def upsert_ticket_announcement(
    conn: psycopg.Connection,
    *,
    seller_team_id: int,
    match_id: int | None,
    source: str,
    source_url: str,
    sale_schedule_text: str,
    prices_text: str,
) -> None:
    """One row per source article: full scraped section blobs."""
    with conn.cursor() as cur:
        cur.execute(
            sql.SQL("""
            INSERT INTO {} (
                seller_team_id, match_id, source, source_url,
                sale_schedule_text, prices_text, scraped_at
            )
            VALUES (%s, %s, %s, %s, %s, %s, now())
            ON CONFLICT (seller_team_id, source, source_url) DO UPDATE SET
                match_id = EXCLUDED.match_id,
                sale_schedule_text = EXCLUDED.sale_schedule_text,
                prices_text = EXCLUDED.prices_text,
                scraped_at = EXCLUDED.scraped_at
            """).format(table("ticket_announcements")),
            (
                seller_team_id,
                match_id,
                source,
                source_url,
                sale_schedule_text,
                prices_text,
            ),
        )


def find_home_match_id(
    conn: psycopg.Connection,
    *,
    home_team_id: int,
    away_name_substr: str | None,
) -> int | None:
    """Best-effort: upcoming home match where away team name matches a substring."""
    if not away_name_substr:
        return None
    key = away_name_substr.strip()
    if len(key) < 2:
        return None
    like = f"%{key}%"
    with conn.cursor() as cur:
        cur.execute(
            sql.SQL("""
            SELECT m.id
            FROM {} m
            JOIN {} at ON at.id = m.away_team_id
            WHERE m.home_team_id = %s
              AND m.kickoff_utc > now() - interval '2 days'
              AND at.name ILIKE %s
            ORDER BY m.kickoff_utc ASC
            LIMIT 1
            """).format(table("matches"), table("teams")),
            (home_team_id, like),
        )
        row = cur.fetchone()
        if not row:
            return None
        assert isinstance(row, dict)
        return int(row["id"])


def competition_id_for_code(conn: psycopg.Connection, competition_code: str) -> int | None:
    with conn.cursor() as cur:
        cur.execute(
            sql.SQL("SELECT id FROM {} WHERE code = %s").format(table("competitions")),
            (competition_code,),
        )
        row = cur.fetchone()
        if not row:
            return None
        return int(row["id"])  # type: ignore[index]
