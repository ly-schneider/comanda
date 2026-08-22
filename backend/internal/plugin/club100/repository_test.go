package club100

import (
	"testing"

	"backend/internal/generated/ent"

	"github.com/stretchr/testify/require"
)

func redemption(personID, orderID string, qty int) *ent.Club100Redemption {
	return &ent.Club100Redemption{
		ElvantoPersonID:     personID,
		OrderID:             orderID,
		FreeProductQuantity: qty,
	}
}

func TestNetRedemptions(t *testing.T) {
	rows := []*ent.Club100Redemption{redemption("p1", "o1", 2)}

	require.Equal(t, map[string]int{"p1": 2}, netRedemptions(rows, nil))
	require.Equal(t, map[string]int{"p1": 2}, netRedemptions(rows, map[string]int{"other": 2}))
	require.Equal(t, map[string]int{"p1": 1}, netRedemptions(rows, map[string]int{"o1": 1}),
		"one refunded product returns one redemption")
	require.Equal(t, map[string]int{"p1": 0}, netRedemptions(rows, map[string]int{"o1": 2}),
		"a cancellation returns the whole redemption")
	require.Equal(t, map[string]int{"p1": 0}, netRedemptions(rows, map[string]int{"o1": 5}),
		"refunded paid units never return more than was comped")
}

func TestNetRedemptions_SumsPerPerson(t *testing.T) {
	rows := []*ent.Club100Redemption{
		redemption("p1", "o1", 1),
		redemption("p1", "o2", 1),
		redemption("p2", "o3", 2),
	}

	got := netRedemptions(rows, map[string]int{"o2": 1, "o3": 1})
	require.Equal(t, map[string]int{"p1": 1, "p2": 1}, got)
}

func TestNetRedemptions_RowsSharingAnOrderDrawFromOnePool(t *testing.T) {
	rows := []*ent.Club100Redemption{
		redemption("p1", "o1", 1),
		redemption("p2", "o1", 1),
	}

	got := netRedemptions(rows, map[string]int{"o1": 1})
	require.Equal(t, map[string]int{"p1": 0, "p2": 1}, got, "the returned unit is spent once")
}

func TestOrderIDsOf_Dedupes(t *testing.T) {
	rows := []*ent.Club100Redemption{
		redemption("p1", "o1", 1),
		redemption("p2", "o1", 1),
		redemption("p3", "o2", 1),
	}

	require.Equal(t, []string{"o1", "o2"}, orderIDsOf(rows))
}
