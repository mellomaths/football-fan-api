from __future__ import annotations

import logging
import os
import re
import time
from datetime import date, datetime, timedelta, timezone

import httpx
import psycopg

from football_scrapers.http_client import with_backoff
from football_scrapers.storage import Storage, competition_id_for_code, ensure_team_id

log = logging.getLogger(__name__)

# ESPN site.api path segments -> internal competition codes (see migrations).
# Slugs may change; tune if scoreboard requests fail.
ESPN_COMPETITION_MAP = {
    "bra.1": "BRASILEIRAO_A",
    "bra.2": "BRASILEIRAO_B",
    "conmebol.libertadores": "COPA_LIBERTADORES",
    "conmebol.sudamericana": "COPA_SULAMERICANA",
    # ESPN scoreboard slug (bra.copa returns 400)
    "bra.copa_do_brazil": "COPA_BRASIL",
}

# Create missing team rows when the source names a club we do not have yet (all mapped ESPN slugs).
# National leagues populate the DB instead of SQL seeds; cups still auto-create foreign clubs.
AUTO_CREATE_TEAM_CODES = frozenset(
    {
        "BRASILEIRAO_A",
        "BRASILEIRAO_B",
        "COPA_LIBERTADORES",
        "COPA_SULAMERICANA",
        "COPA_BRASIL",
    }
)


def _parse_iso_utc(s: str) -> datetime:
    if s.endswith("Z"):
        s = s[:-1] + "+00:00"
    dt = datetime.fromisoformat(s)
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    return dt.astimezone(timezone.utc)


def espn_short_name(tinfo: dict) -> str | None:
    for key in ("shortDisplayName", "abbreviation", "name"):
        v = tinfo.get(key)
        if isinstance(v, str) and v.strip():
            return v.strip()[:64]
    return None


def espn_team_slug(tinfo: dict) -> str | None:
    s = tinfo.get("slug")
    if isinstance(s, str) and s.strip():
        return s.strip()[:128]
    for link in tinfo.get("links") or []:
        href = link.get("href")
        if not isinstance(href, str):
            continue
        m = re.search(r"/id/\d+/([^/?#]+)", href)
        if m:
            return m.group(1)[:128]
    return None


def fetch_scoreboard_json(client: httpx.Client, espn_slug: str, dates: str) -> dict:
    url = f"https://site.api.espn.com/apis/site/v2/sports/soccer/{espn_slug}/scoreboard"
    resp = with_backoff(lambda: client.get(url, params={"dates": dates}))
    resp.raise_for_status()
    return resp.json()


def scrape_espn(storage: Storage, conn: psycopg.Connection, client: httpx.Client) -> int:
    """Fetch upcoming fixtures from ESPN scoreboard API. Returns upsert count."""
    teams = storage.load_teams(conn)
    inserted = 0
    horizon_days = int(os.environ.get("SCRAPER_HORIZON_DAYS", "45"))
    chunk = int(os.environ.get("SCRAPER_ESPN_DATE_CHUNK_DAYS", "7"))
    sleep_chunk = float(os.environ.get("SCRAPER_ESPN_SLEEP_SEC", "0.25"))
    today = date.today()
    end = today + timedelta(days=horizon_days)

    for espn_slug, competition_code in ESPN_COMPETITION_MAP.items():
        comp_db_id = competition_id_for_code(conn, competition_code)
        if comp_db_id is None:
            log.error("missing competition row for %s", competition_code)
            continue

        d = today
        while d <= end:
            d2 = min(d + timedelta(days=chunk - 1), end)
            dates_param = f"{d.strftime('%Y%m%d')}-{d2.strftime('%Y%m%d')}"
            try:
                data = fetch_scoreboard_json(client, espn_slug, dates_param)
            except Exception as e:
                log.warning("espn fetch failed %s %s: %s", espn_slug, dates_param, e)
                d = d2 + timedelta(days=1)
                continue

            events = data.get("events") or []
            for ev in events:
                eid = ev.get("id")
                if eid is None:
                    continue
                comps = (ev.get("competitions") or [])
                if not comps:
                    continue
                comp = comps[0]
                when = comp.get("date")
                if not when:
                    continue
                kickoff = _parse_iso_utc(str(when))
                home_name = away_name = None
                home_short = away_short = None
                home_slug = away_slug = None
                for c in comp.get("competitors") or []:
                    tinfo = c.get("team") or {}
                    team = tinfo.get("displayName")
                    ha = c.get("homeAway")
                    if not team or not ha:
                        continue
                    sn = espn_short_name(tinfo)
                    slug = espn_team_slug(tinfo)
                    if ha == "home":
                        home_name = str(team)
                        home_short = sn
                        home_slug = slug
                    elif ha == "away":
                        away_name = str(team)
                        away_short = sn
                        away_slug = slug
                if not home_name or not away_name:
                    continue
                auto_create = competition_code in AUTO_CREATE_TEAM_CODES
                hid = ensure_team_id(
                    conn,
                    teams,
                    competition_id=comp_db_id,
                    competition_code=competition_code,
                    display_name=home_name,
                    short_name=home_short,
                    espn_slug=home_slug,
                    soccerway_id=None,
                    auto_create=auto_create,
                )
                aid = ensure_team_id(
                    conn,
                    teams,
                    competition_id=comp_db_id,
                    competition_code=competition_code,
                    display_name=away_name,
                    short_name=away_short,
                    espn_slug=away_slug,
                    soccerway_id=None,
                    auto_create=auto_create,
                )
                if hid is None or aid is None:
                    continue
                venue_name = None
                venue_obj = comp.get("venue")
                if isinstance(venue_obj, dict):
                    venue_name = venue_obj.get("fullName") or venue_obj.get("shortName")
                ext = f"{competition_code}:{eid}"
                storage.upsert_match(
                    conn,
                    competition_id=comp_db_id,
                    home_team_id=hid,
                    away_team_id=aid,
                    kickoff_utc=kickoff,
                    venue=str(venue_name) if venue_name else None,
                    source="espn",
                    external_match_id=ext[:256],
                )
                inserted += 1
            time.sleep(sleep_chunk)
            d = d2 + timedelta(days=1)
        conn.commit()
    return inserted
