package client

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
)

// Recording is the answer of GET /api/account/recording: the events the
// plugin recorded from Since to Until (docs/10 section 11). It is also the
// file `chronicle recording -o` writes and the content MCP's propose_tasks
// reads.
type Recording struct {
	Account   string            `json:"account"`
	Since     time.Time         `json:"since"`
	Until     time.Time         `json:"until"`
	Events    []recording.Event `json:"events"`
	Truncated bool              `json:"truncated"`
}

// recordingPage is the most the server returns in one answer.
const recordingPage = 5000

// Recording reads the account's recording from since to until (zero until:
// up to now), following the pages until the end or max events (0: no
// bound). Truncated says max cut it.
func (c *Client) Recording(ctx context.Context, since, until time.Time, maxEvents int) (Recording, error) {
	q, err := c.accountQuery(ctx)
	if err != nil {
		return Recording{}, err
	}
	if until.IsZero() {
		until = time.Now().UTC().Add(time.Second)
	}
	out := Recording{Since: since, Until: until, Events: []recording.Event{}}
	seen := map[string]bool{}
	from := since
	for {
		q.Set("since", from.UTC().Format(time.RFC3339Nano))
		q.Set("until", until.UTC().Format(time.RFC3339Nano))
		q.Set("limit", strconv.Itoa(recordingPage))
		var page Recording
		if err := c.do(ctx, http.MethodGet, "/api/account/recording", q, nil, &page); err != nil {
			return out, err
		}
		out.Account = page.Account
		added := 0
		for _, e := range page.Events {
			// since is inclusive, so the next page starts with the last
			// event of this one.
			if k := e.Recording + "#" + strconv.FormatInt(e.Seq, 10); !seen[k] {
				seen[k] = true
				out.Events = append(out.Events, e)
				added++
			}
		}
		if maxEvents > 0 && len(out.Events) >= maxEvents {
			out.Events, out.Truncated = out.Events[:maxEvents], true
			break
		}
		if !page.Truncated || added == 0 {
			// added == 0: a page of one instant only; the server's order
			// within an instant is stable, so nothing more can come.
			out.Truncated = page.Truncated
			break
		}
		from = page.Events[len(page.Events)-1].At
	}
	slices.SortStableFunc(out.Events, recording.Compare)
	return out, nil
}

// DeleteRecording is DELETE /api/account/recording: every event of the
// account, or of one recording. It returns how many were removed.
func (c *Client) DeleteRecording(ctx context.Context, recordingID string) (int64, error) {
	q, err := c.accountQuery(ctx)
	if err != nil {
		return 0, err
	}
	if recordingID != "" {
		q.Set("recording", recordingID)
	}
	var out struct {
		Deleted int64 `json:"deleted"`
	}
	err = c.do(ctx, http.MethodDelete, "/api/account/recording", q, nil, &out)
	return out.Deleted, err
}
