package events

import (
	"slices"
	"sync"
	"testing"
	"time"
)

func TestPublishRoutesByAccount(t *testing.T) {
	at := time.Date(2026, 11, 5, 20, 14, 0, 0, time.UTC)
	h := NewHub([]string{"oziris-main", "bruhsailer-main"})
	h.now = func() time.Time { return at }
	a := h.Subscribe("acc_a")
	b := h.Subscribe("acc_b")
	defer a.Close()
	defer b.Close()

	tests := []struct {
		name string
		in   Event
		want Event
	}{
		{
			name: "progress without guides gets every guide and now",
			in:   Event{Type: TypeProgress, Account: "acc_a", Source: SourcePlugin, StepsChanged: 2},
			want: Event{Type: TypeProgress, Account: "acc_a", Source: SourcePlugin, StepsChanged: 2,
				Guides: []string{"oziris-main", "bruhsailer-main"}, At: at},
		},
		{
			name: "owner event keeps its guide",
			in:   Event{Type: TypeProgress, Account: "acc_a", Source: SourceOwner, Guides: []string{"bruhsailer-main"}},
			want: Event{Type: TypeProgress, Account: "acc_a", Source: SourceOwner, Guides: []string{"bruhsailer-main"}, At: at},
		},
		{
			name: "sync event keeps no guides and its own time",
			in:   Event{Type: TypeSync, Account: "acc_a", Source: SourceHiscores, Status: "rate_limited", At: at.Add(time.Hour)},
			want: Event{Type: TypeSync, Account: "acc_a", Source: SourceHiscores, Status: "rate_limited", At: at.Add(time.Hour)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.Publish(tt.in)
			select {
			case got := <-a.C:
				if got.Type != tt.want.Type || got.Account != tt.want.Account || got.Source != tt.want.Source ||
					got.StepsChanged != tt.want.StepsChanged || got.Status != tt.want.Status ||
					!got.At.Equal(tt.want.At) || !slices.Equal(got.Guides, tt.want.Guides) {
					t.Errorf("got %+v, want %+v", got, tt.want)
				}
			default:
				t.Fatal("subscriber of acc_a got nothing")
			}
			select {
			case got := <-b.C:
				t.Errorf("acc_b got an event of acc_a: %+v", got)
			default:
			}
		})
	}
}

func TestSlowSubscriberIsDropped(t *testing.T) {
	h := NewHub(nil)
	h.buffer = 2
	slow := h.Subscribe("acc")
	fast := h.Subscribe("acc")
	defer slow.Close()
	defer fast.Close()
	for i := range 3 {
		h.Publish(Event{Type: TypeProgress, Account: "acc", StepsChanged: i})
		<-fast.C // the fast one keeps up
	}
	n := 0
	for range slow.C {
		n++
	}
	if n != 2 {
		t.Errorf("slow subscriber got %d buffered events before its channel closed, want 2", n)
	}
	if got := h.Subscribers("acc"); got != 1 {
		t.Errorf("subscribers = %d, want 1 (the slow one dropped)", got)
	}
	slow.Close() // closing a dropped subscription is a no-op
}

func TestCloseIsIdempotent(t *testing.T) {
	h := NewHub(nil)
	s := h.Subscribe("acc")
	s.Close()
	s.Close()
	if _, ok := <-s.C; ok {
		t.Error("channel still open after Close")
	}
	if got := h.Subscribers("acc"); got != 0 {
		t.Errorf("subscribers = %d", got)
	}
	h.Publish(Event{Type: TypeSync, Account: "acc"}) // no subscribers: no panic
}

// TestConcurrentPublishSubscribe is meant for -race: publishers, subscribers
// that read and leave, and slow ones that never read, all at once.
func TestConcurrentPublishSubscribe(t *testing.T) {
	h := NewHub([]string{"g"})
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					h.Publish(Event{Type: TypeProgress, Account: "acc", Source: SourcePlugin})
				}
			}
		})
	}
	for i := range 32 {
		wg.Go(func() {
			s := h.Subscribe("acc")
			defer s.Close()
			if i%2 == 0 {
				return // never reads: dropped when its queue fills, or closed here
			}
			for range 5 {
				select {
				case _, ok := <-s.C:
					if !ok {
						return
					}
				case <-stop:
					return
				}
			}
		})
	}
	time.Sleep(20 * time.Millisecond)
	close(stop)
	wg.Wait()
	if got := h.Subscribers("acc"); got != 0 {
		t.Errorf("subscribers left = %d", got)
	}
}
