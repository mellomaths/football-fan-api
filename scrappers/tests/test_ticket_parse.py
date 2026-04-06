from __future__ import annotations

from football_scrapers.ticket_parse import (
    extract_prices_full,
    extract_sale_schedule_full,
    flamengo_is_home,
    prefer_listing_title,
    slug_suggests_ticket_sales,
    title_matches_fla_ticket,
)


def test_title_matches_fla_ticket() -> None:
    assert title_matches_fla_ticket("Informações sobre venda de ingresso: Fla x Santos")
    assert not title_matches_fla_ticket("Outra notícia")


def test_prefer_listing_title_keeps_headline_over_teaser() -> None:
    phrase = "informações sobre venda de ingresso"
    headline = "Flamengo x Santos: informações sobre venda de ingressos para o duelo"
    teaser = "Saiba como garantir seu ingresso para o jogo da 10ª rodada"
    merged = prefer_listing_title(teaser, headline, phrase)
    assert title_matches_fla_ticket(merged, phrase)
    assert "Santos" in merged


def test_slug_suggests_ticket_sales() -> None:
    u = "https://www.flamengo.com.br/noticias/futebol/flamengo-x-santos--informacoes-sobre-venda-de-ingressos-para-o-duelo-valido-pelo-brasileiro"
    assert slug_suggests_ticket_sales(u)
    assert not slug_suggests_ticket_sales("https://www.flamengo.com.br/noticias/futebol/outra-noticia")


def test_flamengo_is_home() -> None:
    assert flamengo_is_home("Informações sobre venda de ingresso: Flamengo x Santos", "")
    assert not flamengo_is_home("Informações: Santos x Flamengo", "")
    assert not flamengo_is_home("Empate", "sem padrão")


def test_extract_sale_schedule_full() -> None:
    body = """
Intro.

Data e hora das aberturas de vendas:

Line one.

26/03 às 16h: Tier A

Valores:

Norte
"""
    out = extract_sale_schedule_full(body)
    assert "Data e hora das aberturas de vendas" in out
    assert "26/03" in out
    assert "Valores" not in out


def test_extract_prices_full_truncates_before_estacionamento() -> None:
    body = """
Valores:

Norte (Flamengo)
Nação: R$ 1,00

Serviços extra

Estacionamento

R$ 100
"""
    out = extract_prices_full(body)
    assert out.lower().startswith("valores:")
    assert "Norte" in out
    assert "Estacionamento" not in out
    assert "R$ 100" not in out


def test_extract_prices_full_truncates_before_cancelamento() -> None:
    body = """Valores:\n\nFoo\n\nInformações sobre cancelamento:\nbar"""
    out = extract_prices_full(body)
    assert "Foo" in out
    assert "cancelamento" not in out.lower()
