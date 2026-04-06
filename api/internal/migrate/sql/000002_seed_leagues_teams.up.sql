-- Competition catalog only. Teams and team_competitions rows are filled by scrapers
-- (ESPN/Soccerway) on first run — see scrappers README.
INSERT INTO footballfan.competitions (code, name) VALUES
    ('BRASILEIRAO_A', 'Brasileirão Série A'),
    ('BRASILEIRAO_B', 'Brasileirão Série B'),
    ('COPA_LIBERTADORES', 'Copa Libertadores'),
    ('COPA_SULAMERICANA', 'Copa Sul-Americana'),
    ('COPA_BRASIL', 'Copa do Brasil');
