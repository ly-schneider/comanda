package id

import (
	"strings"
	"testing"
)

func TestNewShape(t *testing.T) {
	for range 1000 {
		got := New()
		if len(got) != Length {
			t.Fatalf("New() = %q, want length %d", got, Length)
		}
		if !Valid(got) {
			t.Fatalf("New() = %q, not Valid", got)
		}
		for i := 0; i < len(got); i++ {
			if !strings.ContainsRune(Alphabet, rune(got[i])) {
				t.Fatalf("New() = %q contains %q outside alphabet", got, got[i])
			}
		}
	}
}

func TestNewUniqueness(t *testing.T) {
	const draws = 10_000
	seen := make(map[string]struct{}, draws)
	for range draws {
		got := New()
		if _, dup := seen[got]; dup {
			t.Fatalf("duplicate id %q after %d draws", got, len(seen))
		}
		seen[got] = struct{}{}
	}
}

func TestValid(t *testing.T) {
	valid := []string{"tenant_seed1", "tenant_test1", "HS79U1yH7Zd3", "------------", "____________"}
	for _, s := range valid {
		if !Valid(s) {
			t.Errorf("Valid(%q) = false, want true", s)
		}
	}

	invalid := []string{
		"",
		"short",
		"tenant_seed12",
		"01970000-000",
		"00000000-0000-7000-8000-000000000001",
		"tenant.seed1",
		"tenant seed1",
		"OIJLoijl0000",
		"tenant_seed\x00",
	}
	for _, s := range invalid {
		if Valid(s) {
			t.Errorf("Valid(%q) = true, want false", s)
		}
	}

	for _, c := range "0OoIiJjLl" {
		s := strings.Repeat(string(c), Length)
		if Valid(s) {
			t.Errorf("Valid(%q) = true, want false (confusable %q)", s, c)
		}
	}
}

func TestAlphabetInvariants(t *testing.T) {
	if len(Alphabet) != 55 {
		t.Fatalf("alphabet length = %d, want 55", len(Alphabet))
	}
	seen := map[rune]bool{}
	for _, c := range Alphabet {
		if seen[c] {
			t.Fatalf("duplicate %q in alphabet", c)
		}
		seen[c] = true
	}
	for _, c := range "0OoIiJjLl" {
		if seen[c] {
			t.Fatalf("confusable %q must not be in alphabet", c)
		}
	}
}
