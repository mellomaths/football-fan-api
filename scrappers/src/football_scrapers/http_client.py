import os
import time
from collections.abc import Callable

import httpx

DEFAULT_UA = (
    "football-scrapers/0.1 (+https://github.com/mellomaths/football-fan-api; research)"
)


def default_headers() -> dict[str, str]:
    ua = os.environ.get("SCRAPER_USER_AGENT", DEFAULT_UA)
    return {"User-Agent": ua, "Accept": "application/json, text/html;q=0.9,*/*;q=0.8"}


def make_client() -> httpx.Client:
    timeout = float(os.environ.get("SCRAPER_HTTP_TIMEOUT_SEC", "30"))
    return httpx.Client(timeout=timeout, headers=default_headers(), follow_redirects=True)


def with_backoff(
    fn: Callable[[], httpx.Response],
    *,
    retries: int = 3,
    base_delay_sec: float = 0.5,
) -> httpx.Response:
    last: Exception | None = None
    for attempt in range(retries):
        try:
            return fn()
        except (httpx.TimeoutException, httpx.TransportError) as e:
            last = e
            time.sleep(base_delay_sec * (2**attempt))
    assert last is not None
    raise last
