package kube

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// ParseQuantity reads a resource quantity as the API writes it ("700m",
// "1Gi", "403585713n", "856788Ki", "1e3"): a decimal number with an optional
// binary suffix (Ki to Ei), decimal suffix (n, u, m, k, M, G, T, P, E) or
// decimal exponent. It is the subset of resource.Quantity that usage and
// quotas need, exact (a rational, not a float).
func ParseQuantity(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	num, mult := s, big.NewRat(1, 1)
	if suffix, ok := binarySuffix(s); ok {
		num = s[:len(s)-2]
		mult.SetInt(new(big.Int).Lsh(big.NewInt(1), suffix))
	} else if i := strings.IndexAny(s, "eE"); i > 0 && i < len(s)-1 {
		exp, err := strconv.Atoi(s[i+1:])
		if err != nil || exp < -18 || exp > 18 {
			return nil, fmt.Errorf("kube: bad quantity %q", s)
		}
		num, mult = s[:i], pow10(exp)
	} else if n := len(s); n > 0 {
		if exp, ok := decimalSuffixes[s[n-1]]; ok {
			num, mult = s[:n-1], pow10(exp)
		}
	}
	if !quantityNumber.MatchString(num) {
		return nil, fmt.Errorf("kube: bad quantity %q", s)
	}
	r, ok := new(big.Rat).SetString(num)
	if !ok {
		return nil, fmt.Errorf("kube: bad quantity %q", s)
	}
	return r.Mul(r, mult), nil
}

var quantityNumber = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)$`)

var decimalSuffixes = map[byte]int{'n': -9, 'u': -6, 'm': -3, 'k': 3, 'M': 6, 'G': 9, 'T': 12, 'P': 15, 'E': 18}

// binarySuffix returns the power of two of a Ki..Ei suffix.
func binarySuffix(s string) (uint, bool) {
	if len(s) < 3 || s[len(s)-1] != 'i' {
		return 0, false
	}
	i := strings.IndexByte("KMGTPE", s[len(s)-2])
	if i < 0 {
		return 0, false
	}
	return uint(10 * (i + 1)), true
}

func pow10(exp int) *big.Rat {
	p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(exp))), nil)
	if exp < 0 {
		return new(big.Rat).SetFrac(big.NewInt(1), p)
	}
	return new(big.Rat).SetInt(p)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
