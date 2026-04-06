"""Orchestrate ticket announcement scrapes (per team adapter)."""

from __future__ import annotations

import logging
import os

from football_scrapers.flamengo_tickets import scrape_flamengo_ticket_news, should_use_flamengo_adapter
from football_scrapers.http_client import make_client
from football_scrapers.storage import Storage, list_teams_with_ticket_sale_url

log = logging.getLogger(__name__)


def run_ticket_scrape_once() -> None:
    enabled = os.environ.get("TICKET_SCRAPER_ENABLED", "true").lower() in ("1", "true", "yes")
    if not enabled:
        log.info("ticket scraper disabled (TICKET_SCRAPER_ENABLED)")
        return
    dsn = os.environ.get("DATABASE_URL")
    if not dsn:
        raise SystemExit("DATABASE_URL is required")
    storage = Storage(dsn)
    with storage.connect() as conn:
        with make_client() as client:
            teams = list_teams_with_ticket_sale_url(conn)
            if not teams:
                log.info("ticket scrape: no teams with ticket_sale_url")
                return
            for team_id, url, name in teams:
                if should_use_flamengo_adapter(name):
                    n = scrape_flamengo_ticket_news(conn, client, team_id=team_id, listing_url=url)
                    log.info("ticket scrape flamengo team_id=%s announcements_saved=%s", team_id, n)
                else:
                    log.warning("ticket scrape: no adapter for team %s (%s); skipped", team_id, name)
