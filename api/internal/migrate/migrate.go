// Package migrate applies embedded SQL migrations in lexical order.
package migrate

import (
	"context"
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mellomaths/football-fan-api/api/internal/db"
)

//go:embed sql/*.sql
var sqlFiles embed.FS

const sqlDir = "sql"

// Up applies embedded migrations in lexical order. Each file runs at most once.
func Up(ctx context.Context, pool *pgxpool.Pool) error {
	if err := ensureSchemaMigrations(ctx, pool); err != nil {
		return err
	}
	names, err := sqlMigrationNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := applyOneMigration(ctx, pool, name); err != nil {
			return err
		}
	}
	return nil
}

func ensureSchemaMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE SCHEMA IF NOT EXISTS %s;
		CREATE TABLE IF NOT EXISTS %s.schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`, db.AppSchema, db.AppSchema))
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

func sqlMigrationNames() ([]string, error) {
	entries, err := sqlFiles.ReadDir(sqlDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".sql") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

func applyOneMigration(ctx context.Context, pool *pgxpool.Pool, name string) error {
	version := path.Join(sqlDir, name)
	var exists bool
	err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS(SELECT 1 FROM %s.schema_migrations WHERE version = $1)`,
		db.AppSchema,
	), version).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check migration %s: %w", name, err)
	}
	if exists {
		return nil
	}
	body, err := sqlFiles.ReadFile(version)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx, string(body)); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("exec migration %s: %w (rollback: %v)", version, err, rbErr)
		}
		return fmt.Errorf("exec migration %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s.schema_migrations (version) VALUES ($1)`,
		db.AppSchema,
	), version); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("record migration %s: %w (rollback: %v)", name, err, rbErr)
		}
		return fmt.Errorf("record migration %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}
