// Package notify runs scheduled digests and ticket announcement notifications.
package notify

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/robfig/cron/v3"

	"github.com/mellomaths/football-fan-api/bots/telegram/internal/apiclient"
)

// Scheduler runs cron jobs; call Stop on shutdown.
type Scheduler struct {
	digest *cron.Cron
	ticket *cron.Cron
}

// Stop stops background crons and waits for running jobs to finish.
func (s *Scheduler) Stop() {
	if s.digest != nil {
		<-s.digest.Stop().Done()
	}
	if s.ticket != nil {
		<-s.ticket.Stop().Done()
	}
}

// Start launches monthly match digests (09:00 on day 1 in loc) and ticket checks (00:00,06:00,12:00,18:00 UTC).
// displayLoc is used to format kickoff times in outbound messages.
func Start(bot *gotgbot.Bot, api *apiclient.Client, displayLoc *time.Location) (*Scheduler, error) {
	if displayLoc == nil {
		displayLoc = time.UTC
	}
	out := &Scheduler{}

	d := cron.New(cron.WithLocation(displayLoc))
	if _, err := d.AddFunc("0 9 1 * *", func() {
		runMonthlyDigest(context.Background(), bot, api, displayLoc)
	}); err != nil {
		return nil, fmt.Errorf("digest cron: %w", err)
	}
	d.Start()
	out.digest = d

	t := cron.New(cron.WithLocation(time.UTC))
	if _, err := t.AddFunc("0 0,6,12,18 * * *", func() {
		runTicketAnnouncements(context.Background(), bot, api, displayLoc)
	}); err != nil {
		return nil, fmt.Errorf("ticket cron: %w", err)
	}
	t.Start()
	out.ticket = t

	return out, nil
}

func runMonthlyDigest(ctx context.Context, bot *gotgbot.Bot, api *apiclient.Client, loc *time.Location) {
	c, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	now := time.Now().In(loc)
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	last := first.AddDate(0, 1, -1)
	from := first.Format("2006-01-02")
	to := last.Format("2006-01-02")

	subs, err := api.ListSubscriptions(c)
	if err != nil {
		return
	}
	monthLabel := first.Format("January 2006")
	for _, sub := range subs {
		matches, err := api.ListMatches(c, sub.TeamID, from, to)
		if err != nil {
			continue
		}
		if len(matches) == 0 {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Matches in %s:\n", monthLabel)
		for _, m := range matches {
			b.WriteString(formatMatchLine(m, loc))
			b.WriteByte('\n')
		}
		chatID, err := strconv.ParseInt(sub.DeliveryTarget, 10, 64)
		if err != nil {
			continue
		}
		if _, sendErr := bot.SendMessage(chatID, strings.TrimSuffix(b.String(), "\n"), nil); sendErr != nil {
			continue
		}
	}
}

func formatMatchLine(m apiclient.Match, loc *time.Location) string {
	t, err := time.Parse(time.RFC3339, m.KickoffUTC)
	if err != nil {
		t = time.Now().UTC()
	}
	t = t.In(loc)
	clock := t.Format("01-02 3:04 PM")
	venue := ""
	if m.Location != nil && m.Location.Name != "" {
		venue = " - " + m.Location.Name
	}
	return fmt.Sprintf("%s %s X %s%s", clock, m.Home.Name, m.Away.Name, venue)
}

func runTicketAnnouncements(ctx context.Context, bot *gotgbot.Bot, api *apiclient.Client, displayLoc *time.Location) {
	if displayLoc == nil {
		displayLoc = time.UTC
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	to := time.Now().UTC()
	from := to.Add(-6 * time.Hour)

	subs, err := api.ListSubscriptions(c)
	if err != nil {
		return
	}
	for _, sub := range subs {
		anns, err := api.ListTicketAnnouncements(c, sub.TeamID, from, to)
		if err != nil {
			continue
		}
		for _, ann := range anns {
			inserted, err := api.PostNotificationReceipt(c, sub.SubscriberID, ann.ID)
			if err != nil || !inserted {
				continue
			}
			var b strings.Builder
			if ann.Match != nil {
				b.WriteString(formatMatchLine(*ann.Match, displayLoc))
				b.WriteByte('\n')
			}
			b.WriteString(ann.SaleScheduleText)
			b.WriteByte('\n')
			b.WriteString(ann.PricesText)

			chatID, err := strconv.ParseInt(sub.DeliveryTarget, 10, 64)
			if err != nil {
				continue
			}
			if _, sendErr := bot.SendMessage(chatID, b.String(), nil); sendErr != nil {
				continue
			}
		}
	}
}
