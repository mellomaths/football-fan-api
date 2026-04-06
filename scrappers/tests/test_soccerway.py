from pathlib import Path

from football_scrapers.soccerway import parse_fixtures_html

FIXTURE = Path(__file__).parent / "fixtures" / "soccerway_sample.html"


def test_parse_fixtures_html_reads_rows():
    html = FIXTURE.read_text(encoding="utf-8")
    rows = parse_fixtures_html(html, "BRASILEIRAO_A")
    assert len(rows) == 1
    r = rows[0]
    assert r["home"] == "Flamengo"
    assert r["away"] == "Palmeiras"
    assert r["competition_code"] == "BRASILEIRAO_A"
    assert r["kickoff_utc"].year == 2026
    assert r["kickoff_utc"].month == 4
    assert r["kickoff_utc"].day == 15
    assert r["home_soccerway_id"] == "2344"
    assert r["away_soccerway_id"] == "2345"
