package syncrun

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestDue(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	ts := func(d time.Duration) pgtype.Timestamptz { return pgtype.Timestamptz{Time: now.Add(d), Valid: true} }
	none := pgtype.Timestamptz{}
	cases := []struct {
		name       string
		last, next pgtype.Timestamptz
		want       bool
	}{
		{"never ran", none, none, true},
		{"ran just now", ts(-time.Minute), none, false},
		{"ran 40 minutes ago", ts(-40 * time.Minute), none, true},
		{"backoff still running", ts(-40 * time.Minute), ts(time.Minute), false},
		{"backoff over", ts(-40 * time.Minute), ts(-time.Second), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Due("acc", tc.last, tc.next, now, 30*time.Minute, 5*time.Minute); got != tc.want {
				t.Fatalf("due = %v, want %v", got, tc.want)
			}
		})
	}
	for i := 0; i < 50; i++ {
		last := now.Add(-time.Duration(i) * time.Minute)
		if o := offset("acc", last, 5*time.Minute); o < -5*time.Minute || o > 5*time.Minute {
			t.Fatalf("offset %v out of the jitter", o)
		}
	}
}

func TestBackoff(t *testing.T) {
	for n, want := range map[int]time.Duration{0: 0, 1: 5 * time.Minute, 2: 10 * time.Minute, 4: 40 * time.Minute, 7: 320 * time.Minute, 8: 6 * time.Hour, 30: 6 * time.Hour} {
		if got := Backoff(n); got != want {
			t.Fatalf("Backoff(%d) = %v, want %v", n, got, want)
		}
	}
}
