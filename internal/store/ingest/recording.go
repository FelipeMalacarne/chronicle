package ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// persistRecording stores the demonstration events of a push (docs/10
// section 11). An event already stored (the same recording and seq, from a
// push resent after a timeout or a 409) is left as it was.
func persistRecording(ctx context.Context, q *storedb.Queries, accountID, snapshotID string, events *[]recording.Event) error {
	if events == nil {
		return nil
	}
	for _, e := range *events {
		raw, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal recorded event: %w", err)
		}
		if _, err := q.InsertRecordedEvent(ctx, storedb.InsertRecordedEventParams{
			AccountID: accountID, Recording: e.Recording, Seq: e.Seq, At: timestamptz(e.At),
			Kind: e.Kind, Event: raw, SnapshotID: snapshotID,
		}); err != nil {
			return fmt.Errorf("insert recorded event %s#%d: %w", e.Recording, e.Seq, err)
		}
	}
	return nil
}

// withoutRecording drops data.recording from a raw push: the recording is
// kept only in recorded_events, where the owner can delete it.
func withoutRecording(raw json.RawMessage) (json.RawMessage, error) {
	var push map[string]json.RawMessage
	if err := json.Unmarshal(raw, &push); err != nil {
		return nil, fmt.Errorf("raw push: %w", err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(push["data"], &data); err != nil {
		return nil, fmt.Errorf("raw push data: %w", err)
	}
	if _, ok := data["recording"]; !ok {
		return raw, nil
	}
	delete(data, "recording")
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	push["data"] = encoded
	return json.Marshal(push)
}
