from __future__ import annotations

import logging
import os
import sys
import time

from apscheduler.schedulers.blocking import BlockingScheduler
from apscheduler.triggers.cron import CronTrigger
from apscheduler.triggers.interval import IntervalTrigger

from football_scrapers.espn import scrape_espn
from football_scrapers.http_client import make_client
from football_scrapers.soccerway import scrape_soccerway
from football_scrapers.storage import Storage
from football_scrapers.tickets import run_ticket_scrape_once


def run_once() -> None:
    dsn = os.environ.get("DATABASE_URL")
    if not dsn:
        raise SystemExit("DATABASE_URL is required")
    log = logging.getLogger(__name__)
    storage = Storage(dsn)
    with storage.connect() as conn:
        with make_client() as client:
            # ESPN runs first (bra.1 / bra.2 before cups in the map) so domestic leagues
            # usually establish teams and primary competition before cup passes.
            n_espn = scrape_espn(storage, conn, client)
            sleep_s = float(os.environ.get("SCRAPER_BETWEEN_SOURCES_SEC", "2"))
            time.sleep(sleep_s)
            n_sw = scrape_soccerway(storage, conn, client)
    log.info("scrape finished espn_rows=%s soccerway_rows=%s", n_espn, n_sw)


def run_ticket_scrape_safe() -> None:
    log = logging.getLogger(__name__)
    try:
        run_ticket_scrape_once()
    except Exception:
        log.exception("ticket scrape failed")


def main() -> None:
    level_name = os.environ.get("LOG_LEVEL", "INFO").upper()
    level = getattr(logging, level_name, logging.INFO)
    logging.basicConfig(
        level=level,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        stream=sys.stdout,
    )
    run_once()
    run_ticket_scrape_safe()

    cron = os.environ.get("SCRAPER_CRON", "").strip()
    hours_raw = os.environ.get("SCRAPER_INTERVAL_HOURS", "").strip()
    if cron:
        trigger: CronTrigger | IntervalTrigger = CronTrigger.from_crontab(cron, timezone="UTC")
        logging.getLogger(__name__).info("scheduler using SCRAPER_CRON=%r (UTC)", cron)
    elif hours_raw:
        trigger = IntervalTrigger(hours=int(hours_raw))
        logging.getLogger(__name__).info("scheduler using SCRAPER_INTERVAL_HOURS=%s", hours_raw)
    else:
        trigger = IntervalTrigger(hours=24)
        logging.getLogger(__name__).info("scheduler default interval 24h (UTC)")

    ticket_min_raw = os.environ.get("TICKET_SCRAPER_INTERVAL_MINUTES", "30").strip()
    try:
        ticket_minutes = int(ticket_min_raw)
        if ticket_minutes < 1:
            ticket_minutes = 30
    except ValueError:
        ticket_minutes = 30
    ticket_trigger = IntervalTrigger(minutes=ticket_minutes)
    logging.getLogger(__name__).info(
        "ticket scheduler using TICKET_SCRAPER_INTERVAL_MINUTES=%s",
        ticket_minutes,
    )

    sched = BlockingScheduler(timezone="UTC")
    sched.add_job(run_once, trigger, max_instances=1, coalesce=True, id="fixture_scrape")
    sched.add_job(run_ticket_scrape_safe, ticket_trigger, max_instances=1, coalesce=True, id="ticket_scrape")
    sched.start()


if __name__ == "__main__":
    main()
