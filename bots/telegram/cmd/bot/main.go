// The bot command runs the Telegram integration for Football Fan API.
package main

import (
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"github.com/mellomaths/football-fan-api/bots/telegram/internal/apiclient"
	"github.com/mellomaths/football-fan-api/bots/telegram/internal/bot"
	"github.com/mellomaths/football-fan-api/bots/telegram/internal/config"
	"github.com/mellomaths/football-fan-api/bots/telegram/internal/notify"
)

func main() {
	os.Exit(run())
}

func run() int {
	logLevel := slog.LevelInfo
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LOG_LEVEL")), "debug") {
		logLevel = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", slog.Any("err", err))
		return 1
	}

	loc, err := time.LoadLocation(cfg.NotifyTZ)
	if err != nil {
		log.Error("notify timezone", slog.String("tz", cfg.NotifyTZ), slog.Any("err", err))
		return 1
	}

	b, err := gotgbot.NewBot(cfg.TelegramToken, nil)
	if err != nil {
		log.Error("telegram bot", slog.Any("err", err))
		return 1
	}

	api := apiclient.New(cfg.APIBaseURL, cfg.APIKey, log)
	deps := bot.Deps{API: api, Log: log}

	d := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(_ *gotgbot.Bot, _ *ext.Context, err error) ext.DispatcherAction {
			log.Error("dispatcher", slog.Any("err", err))
			return ext.DispatcherActionNoop
		},
		Logger: log,
	})
	d.AddHandler(handlers.NewCommand("start", bot.Start))
	d.AddHandler(handlers.NewCommand("subscribe", deps.Subscribe))

	u := ext.NewUpdater(d, &ext.UpdaterOpts{Logger: log})
	err = u.StartPolling(b, &ext.PollingOpts{DropPendingUpdates: true})
	if err != nil {
		log.Error("start polling", slog.Any("err", err))
		return 1
	}

	sched, err := notify.Start(b, api, loc)
	if err != nil {
		log.Error("notify scheduler", slog.Any("err", err))
		return 1
	}
	log.Info("telegram bot running")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		sched.Stop()
		if err := u.Stop(); err != nil {
			log.Error("updater stop", slog.Any("err", err))
		}
	}()

	u.Idle()
	return 0
}
