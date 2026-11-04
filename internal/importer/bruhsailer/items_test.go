package bruhsailer

import (
	"reflect"
	"testing"
)

func TestSplitItems(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"spade, 2 ashes", []string{"spade", "2 ashes"}},
		{
			"5241 gp (set aside 5k for Shades of Mort’ton materials, let’s say), spade",
			[]string{"5241 gp (set aside 5k for Shades of Mort’ton materials, let’s say)", "spade"},
		},
		{
			"2 beers (buy), dwarven stout. If you get asked for a bar you can reset them",
			[]string{"2 beers (buy)", "dwarven stout", "If you get asked for a bar you can reset them"},
		},
		{"bank as needed. 202224 gp, rope", []string{"bank as needed", "202224 gp", "rope"}},
		{"saw, 1 iron bar; rest after creating bank: rune pickaxe", []string{"saw", "1 iron bar", "rest after creating bank: rune pickaxe"}},
		{"melee gear (incl. torso/ring now), abyssal book.", []string{"melee gear (incl. torso/ring now)", "abyssal book"}},
		{"ring of charos(a), 3500 gp.", []string{"ring of charos(a)", "3500 gp"}},
	} {
		if got := splitItems(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitItems(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
