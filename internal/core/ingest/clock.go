package ingest

import "time"

// ClampTimes returns a copy of the push whose sent_at and container
// observed_at are not after now. The server clock bounds the plugin's: a
// client running ahead must not win "newest observation" against hiscores or
// later pushes (docs/10 section 5). The containers are copied, so the caller's
// push is left untouched.
func (p Push) ClampTimes(now time.Time) Push {
	if p.SentAt.After(now) {
		p.SentAt = now
	}
	if p.Data.Items == nil {
		return p
	}
	items := *p.Data.Items
	items.Bank = clampObservation(items.Bank, now)
	items.Inventory = clampObservation(items.Inventory, now)
	items.Equipment = clampObservation(items.Equipment, now)
	p.Data.Items = &items
	return p
}

func clampObservation(obs *ItemObservation, now time.Time) *ItemObservation {
	if obs == nil {
		return nil
	}
	c := *obs
	if c.ObservedAt.After(now) {
		c.ObservedAt = now
	}
	return &c
}
