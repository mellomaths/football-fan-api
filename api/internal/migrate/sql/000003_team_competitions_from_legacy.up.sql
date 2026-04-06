-- Upgrade path: migrate from teams.competition_id + UNIQUE (competition_id, name) to
-- teams.name UNIQUE + footballfan.team_competitions. No-op when 000001 already created the new shape.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'footballfan' AND table_name = 'teams' AND column_name = 'competition_id'
    ) THEN
    CREATE TABLE IF NOT EXISTS footballfan.team_competitions (
        team_id         BIGINT NOT NULL REFERENCES footballfan.teams (id) ON DELETE CASCADE,
        competition_id  BIGINT NOT NULL REFERENCES footballfan.competitions (id) ON DELETE RESTRICT,
        season          VARCHAR(32),
        is_primary      BOOLEAN NOT NULL DEFAULT false,
        PRIMARY KEY (team_id, competition_id)
    );

    CREATE UNIQUE INDEX IF NOT EXISTS uq_team_competitions_one_primary
        ON footballfan.team_competitions (team_id)
        WHERE is_primary;

    CREATE INDEX IF NOT EXISTS idx_team_competitions_competition
        ON footballfan.team_competitions (competition_id);

    INSERT INTO footballfan.team_competitions (team_id, competition_id, is_primary)
    SELECT id, competition_id, true
    FROM footballfan.teams
    ON CONFLICT (team_id, competition_id) DO NOTHING;

    ALTER TABLE footballfan.teams DROP CONSTRAINT IF EXISTS teams_competition_id_fkey;
    ALTER TABLE footballfan.teams DROP CONSTRAINT IF EXISTS teams_competition_id_name_key;
    DROP INDEX IF EXISTS footballfan.idx_teams_competition;

    ALTER TABLE footballfan.teams DROP COLUMN competition_id;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'teams_name_key' AND conrelid = 'footballfan.teams'::regclass
    ) THEN
        ALTER TABLE footballfan.teams ADD CONSTRAINT teams_name_key UNIQUE (name);
    END IF;
    END IF;
END $$;
