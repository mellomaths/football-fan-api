"""Flamengo: news listing → ticket announcement articles → DB upserts."""

from __future__ import annotations

import logging
import os
import re
from urllib.parse import urljoin, urlparse

import httpx
from bs4 import BeautifulSoup
from psycopg import Connection

from football_scrapers.http_client import with_backoff
from football_scrapers.normalize import normalize_team_name
from football_scrapers.storage import (
    find_home_match_id,
    upsert_ticket_announcement,
)
from football_scrapers.ticket_parse import (
    extract_opponent_from_home_title,
    extract_prices_full,
    extract_sale_schedule_full,
    flamengo_is_home,
    prefer_listing_title,
    slug_suggests_ticket_sales,
    title_matches_fla_ticket,
)

log = logging.getLogger(__name__)

SOURCE = "flamengo_noticias"
_ARTICLE_HREF_RE = re.compile(r"/noticias/futebol/[^/?#]+$")


def _article_urls_from_listing(html: str, listing_url: str, phrase: str) -> list[tuple[str, str]]:
    """Return (absolute_url, title) in document order (newest-first on page).

    Cards repeat the same href on image, headline, and teaser links; the last anchor used to
    overwrite the headline with teaser text and broke title filtering. We merge with
    prefer_listing_title and use <img alt=...> when the image link has no visible text.
    """
    base = f"{urlparse(listing_url).scheme}://{urlparse(listing_url).netloc}"
    soup = BeautifulSoup(html, "lxml")
    seen: dict[str, str] = {}
    order: list[str] = []
    for a in soup.find_all("a", href=True):
        href = str(a.get("href", "")).strip()
        if not href or "page=" in href:
            continue
        full = urljoin(base, href)
        if not _ARTICLE_HREF_RE.search(urlparse(full).path or ""):
            continue
        if full.rstrip("/") in (base + "/noticias/futebol", base + "/noticias/futebol/"):
            continue
        title = a.get_text(" ", strip=True)
        if not title:
            img = a.find("img", alt=True)
            if img and (alt := str(img.get("alt") or "").strip()):
                title = alt
        if not title:
            continue
        if full not in seen:
            order.append(full)
            seen[full] = title
        else:
            seen[full] = prefer_listing_title(seen[full], title, phrase)
    return [(u, seen[u]) for u in order]


def scrape_flamengo_ticket_news(
    conn: Connection,
    client: httpx.Client,
    *,
    team_id: int,
    listing_url: str,
) -> int:
    """
    Poll listing_url, process matching posts. Returns number of announcement rows upserted.
    """
    phrase = os.environ.get("FLAMENGO_TICKET_TITLE_PHRASE", "informações sobre venda de ingresso")
    try:
        max_articles = int(os.environ.get("FLAMENGO_TICKET_MAX_ARTICLES_PER_RUN", "25"))
    except ValueError:
        max_articles = 25
    max_articles = max(1, min(max_articles, 100))

    resp = with_backoff(lambda: client.get(listing_url))
    resp.raise_for_status()
    items = _article_urls_from_listing(resp.text, listing_url, phrase)
    saved = 0
    processed = 0
    for article_url, title in items:
        if not title_matches_fla_ticket(title, phrase) and not slug_suggests_ticket_sales(article_url):
            continue
        try:
            aresp = with_backoff(lambda u=article_url: client.get(u))
            aresp.raise_for_status()
        except (httpx.HTTPError, OSError) as e:
            log.warning("flamengo article fetch failed %s: %s", article_url, e)
            continue
        body_text = BeautifulSoup(aresp.text, "lxml").get_text("\n", strip=True)
        if not flamengo_is_home(title, body_text):
            log.info("skip away or ambiguous ticket post: %s", title[:120])
            continue
        opponent = extract_opponent_from_home_title(title)
        match_id = find_home_match_id(conn, home_team_id=team_id, away_name_substr=opponent or "")

        sale_blob = extract_sale_schedule_full(body_text)
        prices_blob = extract_prices_full(body_text)

        with conn.transaction():
            upsert_ticket_announcement(
                conn,
                seller_team_id=team_id,
                match_id=match_id,
                source=SOURCE,
                source_url=article_url,
                sale_schedule_text=sale_blob,
                prices_text=prices_blob,
            )
        saved += 1
        log.info(
            "flamengo ticket scrape article=%s title=%r sale_chars=%s prices_chars=%s",
            article_url,
            title[:160],
            len(sale_blob),
            len(prices_blob),
        )
        processed += 1
        if processed >= max_articles:
            break

    return saved


def should_use_flamengo_adapter(team_name: str) -> bool:
    return normalize_team_name(team_name) == normalize_team_name("Flamengo")
