package service

import "testing"

func TestDepositReturnReason(t *testing.T) {
	const def = "Depot-Rückgabe"
	custom := "Flasche zerbrochen"
	blank := "   "

	cases := []struct {
		name string
		in   *string
		want string
	}{
		{"nil defaults", nil, def},
		{"empty defaults", strptr(""), def},
		{"whitespace defaults", strptr(blank), def},
		{"custom wins", strptr(custom), custom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := depositReturnReason(c.in)
			if got == nil || *got != c.want {
				t.Fatalf("depositReturnReason(%v) = %v, want %q", c.in, got, c.want)
			}
		})
	}
}
