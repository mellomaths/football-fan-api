from __future__ import annotations

import logging
import os
import re
from datetime import datetime, timezone

import httpx
import psycopg
from bs4 import BeautifulSoup

from football_scrapers.http_client import with_backoff
from football_scrapers.storage import Storage, competition_id_for_code, ensure_team_id

log = logging.getLogger(__name__)

# Default pages list scheduled fixtures; structure may change season-to-season.
DEFAULT_URLS = {
    "BRASILEIRAO_A": "https://int.soccerway.com/national/brazil/serie-a/c101/fixtures/",
    "BRASILEIRAO_B": "https://int.soccerway.com/national/brazil/serie-b/c102/fixtures/",
}

# Same as ESPN: create club rows from fixture tables when names are unknown (no SQL team seed).
SOCCERWAY_AUTO_CREATE_CODES = frozenset({"BRASILEIRAO_A", "BRASILEIRAO_B"})


def fetch_html(client: httpx.Client, url: str) -> str:
    resp = with_backoff(lambda: client.get(url))
    resp.raise_for_status()
    return resp.text


def _parse_dt_from_text(text: str) -> datetime | None:
    text = re.sub(r"\s+", " ", text.strip())
    m = re.match(r"(\d{2})/(\d{2})/(\d{4})(?:\s+(\d{2}):(\d{2}))?", text)
    if not m:
        return None
    d, mo, y = int(m.group(1)), int(m.group(2)), int(m.group(3))
    hh, mm = 12, 0
    if m.group(4):
        hh, mm = int(m.group(4)), int(m.group(5))
    return datetime(y, mo, d, hh, mm, tzinfo=timezone.utc)


def extract_soccerway_team_id(href: str) -> str | None:
    """Numeric team id from a Soccerway /teams/... URL path."""
    if not href:
        return None
    path = href.split("?")[0].rstrip("/")
    parts = [p for p in path.split("/") if p]
    for p in reversed(parts):
        if p.isdigit():
            return p[:128]
    return None


def _team_name_and_soccerway_id(td) -> tuple[str, str | None]:
    if td is None:
        return "", None
    a = td.find("a", href=True)
    if a and isinstance(a.get("href"), str):
        name = a.get_text(" ", strip=True)
        sid = extract_soccerway_team_id(a["href"])
    else:
        name = td.get_text(" ", strip=True)
        sid = None
    return name, sid


def parse_fixtures_html(html: str, competition_code: str) -> list[dict]:
    """Best-effort parse of Soccerway fixtures table rows."""
    soup = BeautifulSoup(html, "lxml")
    out: list[dict] = []
    for tr in soup.select("table.matches__table tbody tr, table.matches-table tbody tr"):
        tds = tr.find_all("td")
        if len(tds) < 3:
            continue
        date_td = tds[0]
        home_td = tds[1]
        away_td = tds[2] if len(tds) > 2 else None
        if away_td is None:
            continue
        dt_text = date_td.get_text(" ", strip=True)
        kickoff = _parse_dt_from_text(dt_text)
        if kickoff is None:
            continue
        home, home_sid = _team_name_and_soccerway_id(home_td)
        away, away_sid = _team_name_and_soccerway_id(away_td)
        if not home or not away:
            continue
        mid = None
        link = tr.find("a", href=re.compile(r"/matches/.+/"))
        if link and isinstance(link.get("href"), str):
            parts = link["href"].strip("/").split("/")
            if len(parts) >= 2:
                mid = parts[-1]
        ext = f"{competition_code}:{mid or f'{home}-{away}-{kickoff.date().isoformat()}'}"
        out.append(
            {
                "external_match_id": ext[:256],
                "kickoff_utc": kickoff,
                "home": home,
                "away": away,
                "home_soccerway_id": home_sid,
                "away_soccerway_id": away_sid,
                "competition_code": competition_code,
            }
        )
    return out


def scrape_soccerway(storage: Storage, conn: psycopg.Connection, client: httpx.Client) -> int:
    """Fetch Soccerway HTML fixtures pages and upsert matches."""
    teams = storage.load_teams(conn)
    inserted = 0
    for competition_code, default_url in DEFAULT_URLS.items():
        url = os_getenv_url(competition_code, default_url)
        comp_db_id = competition_id_for_code(conn, competition_code)
        if comp_db_id is None:
            log.error("missing competition row for %s", competition_code)
            continue
        try:
            html = fetch_html(client, url)
        except Exception as e:
            log.warning("soccerway fetch failed %s: %s", url, e)
            continue
        rows = parse_fixtures_html(html, competition_code)
        auto_create = competition_code in SOCCERWAY_AUTO_CREATE_CODES
        for row in rows:
            hid = ensure_team_id(
                conn,
                teams,
                competition_id=comp_db_id,
                competition_code=competition_code,
                display_name=row["home"],
                short_name=None,
                espn_slug=None,
                soccerway_id=row.get("home_soccerway_id"),
                auto_create=auto_create,
            )
            aid = ensure_team_id(
                conn,
                teams,
                competition_id=comp_db_id,
                competition_code=competition_code,
                display_name=row["away"],
                short_name=None,
                espn_slug=None,
                soccerway_id=row.get("away_soccerway_id"),
                auto_create=auto_create,
            )
            if hid is None or aid is None:
                continue
            storage.upsert_match(
                conn,
                competition_id=comp_db_id,
                home_team_id=hid,
                away_team_id=aid,
                kickoff_utc=row["kickoff_utc"],
                venue=None,
                source="soccerway",
                external_match_id=row["external_match_id"],
            )
            inserted += 1
        conn.commit()
    return inserted


def os_getenv_url(competition_code: str, default: str) -> str:
    key = f"SOCCERWAY_URL_{competition_code}"
    return os.environ.get(key, default)
