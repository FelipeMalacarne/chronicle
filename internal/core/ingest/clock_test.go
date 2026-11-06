package ingest

import (
	"testing"
	"time"
)

func TestClampTimes(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	ahead := now.Add(90 * time.Second)
	behind := now.Add(-time.Minute)
	tests := []struct {
		name          string
		sent, bank    time.Time
		wantSent      time.Time
		wantBank      time.Time
		withContainer bool
	}{
		{name: "client behind is kept", sent: behind, bank: behind, wantSent: behind, wantBank: behind, withContainer: true},
		{name: "client ahead is bounded", sent: ahead, bank: ahead, wantSent: now, wantBank: now, withContainer: true},
		{name: "only sent_at ahead", sent: ahead, bank: behind, wantSent: now, wantBank: behind, withContainer: true},
		{name: "no containers", sent: ahead, wantSent: now},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Push{SentAt: tt.sent}
			var orig *ItemObservation
			if tt.withContainer {
				orig = &ItemObservation{State: ItemLive, ObservedAt: tt.bank}
				p.Data.Items = &ItemContainers{Bank: orig}
			}
			got := p.ClampTimes(now)
			if !got.SentAt.Equal(tt.wantSent) {
				t.Fatalf("sent_at = %v, want %v", got.SentAt, tt.wantSent)
			}
			if !tt.withContainer {
				if got.Data.Items != nil {
					t.Fatalf("items appeared: %+v", got.Data.Items)
				}
				return
			}
			if !got.Data.Items.Bank.ObservedAt.Equal(tt.wantBank) {
				t.Fatalf("bank observed_at = %v, want %v", got.Data.Items.Bank.ObservedAt, tt.wantBank)
			}
			if got.Data.Items.Inventory != nil || got.Data.Items.Equipment != nil {
				t.Fatalf("absent containers appeared")
			}
			if !orig.ObservedAt.Equal(tt.bank) || !p.SentAt.Equal(tt.sent) {
				t.Fatalf("caller's push was modified")
			}
		})
	}
}
