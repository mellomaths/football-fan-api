package teamresolve

import (
	"errors"
	"testing"

	"github.com/mellomaths/football-fan-api/bots/telegram/internal/apiclient"
)

func TestPickTeam(t *testing.T) {
	t.Parallel()
	_, err := PickTeam(nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
	id, err := PickTeam([]apiclient.Team{{ID: 5, Name: "A"}})
	if err != nil || id != 5 {
		t.Fatalf("got id=%d err=%v", id, err)
	}
	_, err = PickTeam([]apiclient.Team{{ID: 1}, {ID: 2}})
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("got %v, want ErrAmbiguous", err)
	}
}
