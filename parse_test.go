package solanaswap_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/franco-bianco/solanaswap-go/v2"
)

// testCase is one entry of testdata/cases.json. Each case is a real
// mainnet transaction (testdata/transactions/<file>) whose expected result
// was derived independently from the raw transaction: inner instructions and
// pre/post balances.
type testCase struct {
	File        string   `json:"file"`
	Description string   `json:"description"`
	Err         string   `json:"err,omitempty"` // "no swap" or "failed"
	Trader      string   `json:"trader,omitempty"`
	InMint      string   `json:"inMint,omitempty"`
	InAmount    string   `json:"inAmount,omitempty"`
	OutMint     string   `json:"outMint,omitempty"`
	OutAmount   string   `json:"outAmount,omitempty"`
	AMMs        []string `json:"amms,omitempty"`
	Routers     []string `json:"routers,omitempty"`
	Fees        []struct {
		Kind      string `json:"kind"`
		Amount    string `json:"amount"`
		Recipient string `json:"recipient"`
	} `json:"fees,omitempty"`
	// Legs are individual hops that must appear in SwapInfo.Swaps.
	Legs []struct {
		Protocol  string `json:"protocol"`
		InMint    string `json:"inMint"`
		InAmount  string `json:"inAmount"`
		OutMint   string `json:"outMint"`
		OutAmount string `json:"outAmount"`
	} `json:"legs,omitempty"`
}

// matches reports whether got equals want, treating an empty want as "any".
func matches(got, want string) bool { return want == "" || got == want }

func loadCases(t *testing.T) []testCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []testCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("decode cases.json: %v", err)
	}
	return cases
}

func TestParseFixtures(t *testing.T) {
	for _, tc := range loadCases(t) {
		t.Run(tc.File, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "transactions", tc.File))
			if err != nil {
				t.Fatal(err)
			}
			info, err := solanaswap.ParseJSON(data)

			switch tc.Err {
			case "no swap":
				if !errors.Is(err, solanaswap.ErrNoSwap) {
					t.Fatalf("err = %v, want ErrNoSwap", err)
				}
				return
			case "failed":
				if !errors.Is(err, solanaswap.ErrTransactionFailed) {
					t.Fatalf("err = %v, want ErrTransactionFailed", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			check := func(field, got, want string) {
				t.Helper()
				if want != "" && got != want {
					t.Errorf("%s = %s, want %s", field, got, want)
				}
			}
			check("Trader", info.Trader.String(), tc.Trader)
			check("TokenInMint", info.TokenInMint.String(), tc.InMint)
			check("TokenInAmount", strconv.FormatUint(info.TokenInAmount, 10), tc.InAmount)
			check("TokenOutMint", info.TokenOutMint.String(), tc.OutMint)
			check("TokenOutAmount", strconv.FormatUint(info.TokenOutAmount, 10), tc.OutAmount)
			if tc.AMMs != nil && !slices.Equal(info.AMMs, tc.AMMs) {
				t.Errorf("AMMs = %q, want %q", info.AMMs, tc.AMMs)
			}
			for _, r := range tc.Routers {
				if !slices.Contains(info.Routers, r) {
					t.Errorf("Routers = %q, want it to contain %q", info.Routers, r)
				}
			}
			for _, want := range tc.Fees {
				found := slices.ContainsFunc(info.Fees, func(f solanaswap.Fee) bool {
					return string(f.Kind) == want.Kind && strconv.FormatUint(f.Amount, 10) == want.Amount &&
						(want.Recipient == "" || f.Recipient.String() == want.Recipient)
				})
				if !found {
					t.Errorf("Fees = %+v, want a %s fee of %s to %s", info.Fees, want.Kind, want.Amount, want.Recipient)
				}
			}
			for _, want := range tc.Legs {
				found := slices.ContainsFunc(info.Swaps, func(s solanaswap.Swap) bool {
					return matches(s.Protocol, want.Protocol) &&
						matches(s.InputMint.String(), want.InMint) &&
						matches(strconv.FormatUint(s.InputAmount, 10), want.InAmount) &&
						matches(s.OutputMint.String(), want.OutMint) &&
						matches(strconv.FormatUint(s.OutputAmount, 10), want.OutAmount)
				})
				if !found {
					t.Errorf("Swaps = %+v, want a %s leg %s %s -> %s %s", info.Swaps, want.Protocol, want.InAmount, want.InMint, want.OutAmount, want.OutMint)
				}
			}
			if info.Timestamp.IsZero() {
				t.Error("Timestamp is zero")
			}
			if len(info.Signatures) == 0 || len(info.Signers) == 0 {
				t.Error("missing signatures or signers")
			}
		})
	}
}

// TestFixturesHaveCases guards against fixtures that are never asserted.
func TestFixturesHaveCases(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "transactions", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	covered := make(map[string]bool)
	for _, tc := range loadCases(t) {
		if covered[tc.File] {
			t.Errorf("duplicate case for %s", tc.File)
		}
		covered[tc.File] = true
	}
	for _, f := range files {
		if !covered[filepath.Base(f)] {
			t.Errorf("fixture %s has no case in cases.json", filepath.Base(f))
		}
	}
}
