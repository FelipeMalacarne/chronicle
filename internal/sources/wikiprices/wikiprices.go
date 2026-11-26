// Package wikiprices is the adapter of the OSRS Wiki real-time prices API
// (docs/12 section 6): it fetches /latest and /mapping and normalizes them
// into ge.Price. Only this package knows the external shape.
package wikiprices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/ge"
)

// DefaultBaseURL is the v2 API; tests point the client elsewhere.
const DefaultBaseURL = "https://prices.runescape.wiki/api/v2/osrs"

// UserAgent identifies Chronicle to the wiki, which blocks generic ones.
const UserAgent = "chronicle item prices (github.com/FelipeMalacarne/chronicle)"

// maxBody caps a response; /latest is about 400 KB and /mapping 1.5 MB.
const maxBody = 16 << 20

// Client fetches the prices.
type Client struct {
	BaseURL   string
	HTTP      *http.Client
	UserAgent string
}

// New returns a client for the real API.
func New() *Client {
	return &Client{
		BaseURL:   DefaultBaseURL,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: UserAgent,
	}
}

// Fetch reads /latest and /mapping once each (never per item) and
// normalizes them at the time of the call.
func (c *Client) Fetch(ctx context.Context) ([]ge.Price, error) {
	latest, err := c.get(ctx, "/latest")
	if err != nil {
		return nil, err
	}
	mapping, err := c.get(ctx, "/mapping")
	if err != nil {
		return nil, err
	}
	return Normalize(latest, mapping, time.Now().UTC())
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wikiprices %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wikiprices %s: HTTP %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("wikiprices %s: %w", path, err)
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("wikiprices %s: response over %d bytes", path, maxBody)
	}
	return body, nil
}

type latestDoc struct {
	Data map[string]struct {
		High     *int64 `json:"high"`
		HighTime *int64 `json:"highTime"`
		Low      *int64 `json:"low"`
		LowTime  *int64 `json:"lowTime"`
	} `json:"data"`
}

type mappingItem struct {
	ID      int   `json:"id"`
	Members *bool `json:"members"`
	Limit   *int  `json:"limit"`
}

// Normalize joins /latest with /mapping. Every /latest item with at least
// one usable side is kept; limit and members come from /mapping when it has
// them. A nonsense entry (bad id, price or time) is dropped, not the batch;
// a document that does not parse is an error. Output is ordered by item id.
func Normalize(latest, mapping []byte, fetchedAt time.Time) ([]ge.Price, error) {
	var l latestDoc
	if err := json.Unmarshal(latest, &l); err != nil {
		return nil, fmt.Errorf("wikiprices latest: %w", err)
	}
	if l.Data == nil {
		return nil, errors.New("wikiprices latest: no data")
	}
	var m []mappingItem
	if err := json.Unmarshal(mapping, &m); err != nil {
		return nil, fmt.Errorf("wikiprices mapping: %w", err)
	}
	byID := make(map[int]mappingItem, len(m))
	for _, it := range m {
		byID[it.ID] = it
	}
	out := make([]ge.Price, 0, len(l.Data))
	for key, v := range l.Data {
		id, err := strconv.Atoi(key)
		if err != nil || id <= 0 {
			continue
		}
		p := ge.Price{ItemID: id, Source: ge.SourceWiki, FetchedAt: fetchedAt}
		if v.High != nil && v.HighTime != nil && *v.High > 0 && *v.HighTime > 0 {
			p.High, p.HighAt = *v.High, time.Unix(*v.HighTime, 0).UTC()
		}
		if v.Low != nil && v.LowTime != nil && *v.Low > 0 && *v.LowTime > 0 {
			p.Low, p.LowAt = *v.Low, time.Unix(*v.LowTime, 0).UTC()
		}
		if p.High == 0 && p.Low == 0 {
			continue
		}
		if it, ok := byID[id]; ok {
			if it.Limit != nil && *it.Limit > 0 {
				p.Limit = it.Limit
			}
			p.Members = it.Members
		}
		out = append(out, p)
	}
	sortByID(out)
	return out, nil
}

func sortByID(p []ge.Price) {
	slices.SortFunc(p, func(a, b ge.Price) int { return a.ItemID - b.ItemID })
}
