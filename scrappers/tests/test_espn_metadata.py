from football_scrapers.espn import espn_short_name, espn_team_slug


def test_espn_team_slug_from_api_slug():
    assert espn_team_slug({"slug": "flamengo"}) == "flamengo"


def test_espn_team_slug_from_clubhouse_link():
    t = {
        "slug": None,
        "links": [
            {
                "href": "https://www.espn.com/soccer/club/_/id/6086/botafogo",
                "rel": ["clubhouse", "desktop", "team"],
            }
        ],
    }
    assert espn_team_slug(t) == "botafogo"


def test_espn_short_name_prefers_short_display():
    assert espn_short_name({"shortDisplayName": "Fla", "abbreviation": "FLA"}) == "Fla"


def test_espn_short_name_falls_back_to_abbreviation():
    assert espn_short_name({"abbreviation": "BOT"}) == "BOT"
