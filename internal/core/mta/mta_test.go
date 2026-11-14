package mta

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type vector struct {
	Name  string  `json:"name"`
	Input Input   `json:"input"`
	Want  *Result `json:"want"`
	Error bool    `json:"error"`
}

func loadVectors(t *testing.T) []vector {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vs []vector
	if err := json.Unmarshal(raw, &vs); err != nil {
		t.Fatal(err)
	}
	return vs
}

func TestVectors(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			got, err := Calculate(v.Input)
			if v.Error {
				if err == nil {
					t.Fatalf("want an error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, *v.Want) {
				t.Errorf("got  %+v\nwant %+v", got, *v.Want)
			}
		})
	}
}

// TestVectorsCopy: the plugin's JUnit test reads its own copy of the vectors, which must be this file.
func TestVectorsCopy(t *testing.T) {
	copyPath := filepath.Join("..", "..", "..", "microbot", "src", "test", "resources", "mta-vectors.json")
	want, err := os.ReadFile(filepath.Join("testdata", "vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatalf("the plugin's copy of the vectors: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("%s differs from testdata/vectors.json: copy it over", copyPath)
	}
}

// The wiki's own totals for the whole shop (Template:Mage Training Arena Shop stock).
func TestShopTotals(t *testing.T) {
	var sum Points
	for _, r := range Data().Rewards {
		sum.Telekinetic += r.Cost.Telekinetic
		sum.Graveyard += r.Cost.Graveyard
		sum.Enchantment += r.Cost.Enchantment
		sum.Alchemist += r.Cost.Alchemist
	}
	if want := (Points{2825, 2825, 29000, 3275}); sum != want {
		t.Errorf("shop totals %+v, want %+v", sum, want)
	}
}

func TestEveryRequiresExists(t *testing.T) {
	for _, r := range Data().Rewards {
		if r.Requires != "" {
			if _, ok := rewards[r.Requires]; !ok {
				t.Errorf("%s requires unknown %s", r.ID, r.Requires)
			}
		}
	}
}

func TestInvalidInput(t *testing.T) {
	for name, in := range map[string]Input{
		"negative points":  {Rewards: []string{"infinity_boots"}, Points: Points{Telekinetic: -1}, Magic: 99},
		"negative magic":   {Rewards: []string{"infinity_boots"}, Magic: -1},
		"negative enchant": {Rewards: []string{"infinity_boots"}, Magic: 99, EnchantLevel: -2},
	} {
		if _, err := Calculate(in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: got %v, want ErrInvalidInput", name, err)
		}
	}
	if _, err := Calculate(Input{Rewards: []string{"nope"}}); !errors.Is(err, ErrUnknownReward) {
		t.Errorf("unknown reward: got %v", err)
	}
}
