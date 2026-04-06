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


def main() -> None:
    level_name = os.environ.get("LOG_LEVEL", "INFO").upper()
    level = getattr(logging, level_name, logging.INFO)
    logging.basicConfig(
        level=level,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        stream=sys.stdout,
    )
    run_once()

    cron = os.environ.get("SCRAPER_CRON", "").strip()
    hours_raw = os.environ.get("SCRAPER_INTERVAL_HOURS", "").strip()
    if cron:
        trigger: CronTrigger | IntervalTrigger = CronTrigger.from_crontab(
            cron, timezone="UTC"
        )
        logging.getLogger(__name__).info("scheduler using SCRAPER_CRON=%r (UTC)", cron)
    elif hours_raw:
        trigger = IntervalTrigger(hours=int(hours_raw))
        logging.getLogger(__name__).info(
            "scheduler using SCRAPER_INTERVAL_HOURS=%s", hours_raw
        )
    else:
        trigger = IntervalTrigger(hours=24)
        logging.getLogger(__name__).info("scheduler default interval 24h (UTC)")

    sched = BlockingScheduler(timezone="UTC")
    sched.add_job(run_once, trigger, max_instances=1, coalesce=True)
    sched.start()


if __name__ == "__main__":
    main()
