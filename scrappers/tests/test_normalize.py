from football_scrapers.normalize import normalize_team_name


def test_normalize_ascii_fold():
    assert normalize_team_name("São Paulo") == "sao paulo"
    assert normalize_team_name("  Grêmio  ") == "gremio"
