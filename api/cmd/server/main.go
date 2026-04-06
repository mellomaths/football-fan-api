// The server command runs the football fan HTTP API and applies migrations on startup.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mellomaths/football-fan-api/api/internal/config"
	"github.com/mellomaths/football-fan-api/api/internal/db"
	"github.com/mellomaths/football-fan-api/api/internal/httpapi"
	"github.com/mellomaths/football-fan-api/api/internal/migrate"
)

func main() {
	os.Exit(run())
}

func run() int {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", slog.Any("err", err))
		return 1
	}

	ctx := context.Background()
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Error("db config", slog.Any("err", err))
		return 1
	}
	poolCfg.MaxConns = 25
	poolCfg.MaxConnIdleTime = time.Minute
	poolCfg.MaxConnLifetime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		log.Error("db connect", slog.Any("err", err))
		return 1
	}
	defer pool.Close()

	if err := migrate.Up(ctx, pool); err != nil {
		log.Error("migrate", slog.Any("err", err))
		return 1
	}

	store := db.NewStore(pool)
	srv := httpapi.NewServer(log, store)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", slog.String("addr", cfg.HTTPAddr))
		errCh <- httpServer.ListenAndServe()
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Error("http server", slog.Any("err", err))
			return 1
		}
		return 0
	case <-sig:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Error("shutdown", slog.Any("err", err))
			return 1
		}
		if err := <-errCh; err != nil && err != http.ErrServerClosed {
			log.Error("http server", slog.Any("err", err))
			return 1
		}
		return 0
	}
}
