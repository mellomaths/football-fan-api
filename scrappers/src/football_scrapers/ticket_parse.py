"""Helpers for Flamengo ticket announcement pages (listing titles, section extraction)."""

from __future__ import annotations

import re
import unicodedata
from urllib.parse import urlparse

# Title filter for Flamengo news listing (case- and accent-insensitive).
FLAMENGO_TICKET_TITLE_PHRASE = "informações sobre venda de ingresso"

# Stop "Valores:" blob before these sections (plain text from BeautifulSoup).
_PRICES_SECTION_END_RE = re.compile(
    r"\n\s*(?:"
    r"estacionamento\b|"
    r"informa[cç][oõ]es\s+sobre\s+cancelamento\b"
    r")",
    re.IGNORECASE | re.MULTILINE,
)


def normalize_text(s: str) -> str:
    if not s:
        return ""
    n = unicodedata.normalize("NFKD", s)
    n = "".join(ch for ch in n if not unicodedata.combining(ch))
    return n.casefold()


def title_matches_fla_ticket(title: str, phrase: str = FLAMENGO_TICKET_TITLE_PHRASE) -> bool:
    return normalize_text(phrase) in normalize_text(title)


def slug_suggests_ticket_sales(url: str) -> bool:
    """True when URL path looks like Flamengo ticket posts (ASCII slug: informacoes-sobre-venda-de-ingresso…)."""
    path = (urlparse(url).path or "").strip("/")
    slug = path.rsplit("/", 1)[-1]
    n = normalize_text(slug.replace("-", " "))
    return all(p in n for p in ("informaco", "venda", "ingresso"))


def prefer_listing_title(existing: str, candidate: str, phrase: str) -> str:
    """
    Merge duplicate <a href> titles on listing cards: image link (often empty text), headline, teaser.
    Prefer the string that matches the ticket phrase; never replace it with a teaser that does not.
    """
    em = title_matches_fla_ticket(existing, phrase)
    cm = title_matches_fla_ticket(candidate, phrase)
    if cm and not em:
        return candidate
    if em and not cm:
        return existing
    return existing if len(existing) >= len(candidate) else candidate


def flamengo_is_home(title: str, body_text: str) -> bool:
    """
    Heuristic: home when Flamengo is listed first (Flamengo x … / Fla x …).
    Away when pattern 'x Flamengo' suggests visitor.
    """
    t = normalize_text(title)
    if re.search(r"\b(flamengo|fla)\s+x\s+", t):
        return True
    if re.search(r"\sx\s+(flamengo|fla)\b", t):
        return False
    b = normalize_text(body_text)
    if "maracan" in b and ("mando" in b or "jogando em casa" in b):
        return True
    return False


def extract_sale_schedule_full(text: str) -> str:
    """
    Full text from 'Data e hora das aberturas de vendas' through the line before 'Valores:'.
    Preserves site wording and line breaks (normalized by get_text).
    """
    low = text.lower()
    key = "data e hora das aberturas de vendas"
    i = low.find(key)
    if i < 0:
        return ""
    j = low.find("valores:", i)
    if j < 0:
        return text[i:].strip()
    return text[i:j].strip()


def extract_prices_full(text: str) -> str:
    """
    Full text from 'Valores:' through Maracanã Mais / related blocks, stopping before
    Estacionamento or 'Informações sobre cancelamento'.
    """
    low = text.lower()
    key = "valores:"
    i = low.find(key)
    if i < 0:
        return ""
    chunk = text[i:]
    m = _PRICES_SECTION_END_RE.search(chunk)
    if m:
        chunk = chunk[: m.start()]
    return chunk.strip()


def extract_opponent_from_home_title(title: str) -> str | None:
    """From 'Flamengo x Santos' return 'Santos'."""
    t = title.strip()
    m = re.search(r"(?i)(?:flamengo|fla)\s+x\s+(.+)$", t)
    if not m:
        return None
    return m.group(1).strip().rstrip(".")
