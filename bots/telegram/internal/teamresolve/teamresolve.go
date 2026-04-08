// Package teamresolve applies disambiguation rules to team search results.
package teamresolve

import (
	"errors"

	"github.com/mellomaths/football-fan-api/bots/telegram/internal/apiclient"
)

// ErrNotFound is returned when no team matches the query.
var ErrNotFound = errors.New("team not found")

// ErrAmbiguous is returned when more than one team matches.
var ErrAmbiguous = errors.New("multiple teams match; please refine the name")

// PickTeam returns the team id when exactly one team is in the slice.
func PickTeam(teams []apiclient.Team) (int64, error) {
	switch len(teams) {
	case 0:
		return 0, ErrNotFound
	case 1:
		return teams[0].ID, nil
	default:
		return 0, ErrAmbiguous
	}
}
