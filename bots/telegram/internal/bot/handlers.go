// Package bot registers Telegram command handlers.
package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/mellomaths/football-fan-api/bots/telegram/internal/apiclient"
	"github.com/mellomaths/football-fan-api/bots/telegram/internal/teamresolve"
)

// Deps bundles handler dependencies.
type Deps struct {
	API *apiclient.Client
	Log *slog.Logger
}

// Subscribe handles /subscribe [TEAM].
func (d Deps) Subscribe(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveMessage == nil || ctx.EffectiveChat == nil {
		return nil
	}
	q := subscribeTeamQueryFromMessage(ctx.EffectiveMessage.GetText())
	if q == "" {
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Usage: /subscribe <team name>", nil)
		return err
	}
	if d.Log != nil {
		d.Log.Info("subscribe command",
			slog.String("name_query", q),
			slog.Int64("chat_id", ctx.EffectiveChat.Id),
		)
	}
	c, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	teams, err := d.API.ListTeams(c, q)
	if err != nil {
		if d.Log != nil {
			d.Log.Error("subscribe list teams failed", slog.String("name_query", q), slog.Any("err", err))
		}
		_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "Could not reach the API. Try again later.", nil)
		return errors.Join(err, sendErr)
	}
	teamID, err := d.resolveSubscribeTeamID(b, ctx, q, teams)
	if err != nil {
		return err
	}

	team, err := d.API.GetTeam(c, teamID)
	if err != nil {
		_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "Could not load team details.", nil)
		return errors.Join(err, sendErr)
	}
	if team.TicketSaleURL == nil || strings.TrimSpace(*team.TicketSaleURL) == "" {
		if _, warnErr := b.SendMessage(ctx.EffectiveChat.Id,
			"Note: this team has no ticket sale URL configured yet. You may not receive ticket sale announcements.", nil); warnErr != nil && d.Log != nil {
			d.Log.Warn("send ticket url notice", slog.Any("err", warnErr))
		}
	}

	u := ctx.EffectiveUser
	if u == nil {
		_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "Could not read your user profile.", nil)
		return sendErr
	}
	extKey := fmt.Sprintf("tg:%d", u.Id)
	chatStr := strconv.FormatInt(ctx.EffectiveChat.Id, 10)
	display := strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
	var dispPtr *string
	if display != "" {
		dispPtr = &display
	}
	meta, err := json.Marshal(map[string]string{"integration": "telegram"})
	if err != nil {
		return fmt.Errorf("marshal subscriber meta: %w", err)
	}
	sub, err := d.API.UpsertUser(c, extKey, chatStr, dispPtr, meta)
	if err != nil {
		_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "Could not register your account with the API.", nil)
		return errors.Join(err, sendErr)
	}
	if subErr := d.API.AddSubscription(c, sub.ID, teamID); subErr != nil {
		msg := "Could not save subscription."
		if strings.Contains(subErr.Error(), "409") {
			msg = "You are already subscribed to this team."
		}
		_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, msg, nil)
		return sendErr
	}
	_, err = b.SendMessage(ctx.EffectiveChat.Id,
		fmt.Sprintf("You are subscribed to match and ticket updates for %s.", team.Name), nil)
	return err
}

func (d Deps) resolveSubscribeTeamID(b *gotgbot.Bot, ctx *ext.Context, q string, teams []apiclient.Team) (int64, error) {
	teamID, err := teamresolve.PickTeam(teams)
	if err == nil {
		return teamID, nil
	}
	if errors.Is(err, teamresolve.ErrNotFound) {
		if d.Log != nil {
			d.Log.Warn("subscribe no team match",
				slog.String("name_query", q),
				slog.Int("teams_from_api", len(teams)),
				slog.String("hint", "GET /teams only returns clubs with at least one team_competitions row; verify the club is linked to a competition or compare with SQL on team_competitions, not teams alone"),
			)
		}
		_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "No team matched that name.", nil)
		return 0, sendErr
	}
	if errors.Is(err, teamresolve.ErrAmbiguous) {
		names := make([]string, 0, len(teams))
		for _, t := range teams {
			names = append(names, fmt.Sprintf("%s (id %d)", t.Name, t.ID))
		}
		_, sendErr := b.SendMessage(ctx.EffectiveChat.Id,
			"Several teams matched. Please pick a more specific name.\n"+strings.Join(names, "\n"), nil)
		return 0, sendErr
	}
	return 0, err
}

// Start handles /start with a short help text.
func Start(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat == nil {
		return nil
	}
	text := "Football Fan bot.\n\nUse /subscribe <team name> to follow a club."
	_, err := b.SendMessage(ctx.EffectiveChat.Id, text, nil)
	return err
}
