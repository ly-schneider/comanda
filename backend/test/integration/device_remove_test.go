package integration

import (
	"context"
	"testing"

	entDevice "backend/internal/generated/ent/device"
	entBinding "backend/internal/generated/ent/devicebinding"
	"backend/internal/generated/ent/product"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/stretchr/testify/require"
)

// TestDeviceService_Remove locks in the hard-remove flow: revoke the device's
// active bindings (cutting off auth and evicting the token-hash cache), delete
// its device_product rows, then delete the device — without touching any other
// device.
func TestDeviceService_Remove(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)

	// sessionRepo is nil: Remove never touches it. Wiring mirrors production
	// otherwise (cached binding repo so the revoke exercises cache eviction).
	svc := service.NewDeviceService(tdb.Client, repos.Device, repos.DeviceProduct, repos.DeviceBinding, nil)
	ctx := context.Background()

	cat := fixtures.CreateCategory("Drinks", 1, true)
	cola := fixtures.CreateProduct("Cola", cat.ID, 350, product.TypeSimple, nil)
	sprite := fixtures.CreateProduct("Sprite", cat.ID, 350, product.TypeSimple, nil)

	station := fixtures.CreateDevice("Station 1", "stn-key-1", entDevice.TypeSTATION, entDevice.StatusApproved)
	fixtures.AssignProductToDevice(station.ID, cola.ID)
	fixtures.AssignProductToDevice(station.ID, sprite.ID)
	stationToken := repository.HashToken("station-1-session-token")
	fixtures.CreateDeviceBinding(entBinding.DeviceTypeSTATION, stationToken, station.ID)

	// An unrelated device that must be left completely untouched.
	other := fixtures.CreateDevice("POS 1", "pos-key-1", entDevice.TypePOS, entDevice.StatusApproved)
	fixtures.AssignProductToDevice(other.ID, cola.ID)
	otherToken := repository.HashToken("pos-1-session-token")
	fixtures.CreateDeviceBinding(entBinding.DeviceTypePOS, otherToken, other.ID)

	// Warm the auth cache so the eviction path on revoke is exercised.
	warm, err := repos.DeviceBinding.GetByTokenHash(ctx, stationToken)
	require.NoError(t, err)
	require.Equal(t, station.ID, *warm.DeviceID)

	require.NoError(t, svc.Remove(ctx, station.ID))

	t.Run("device is deleted", func(t *testing.T) {
		_, err := repos.Device.GetByID(ctx, station.ID)
		require.ErrorIs(t, err, repository.ErrNotFound)
	})

	t.Run("device_product rows are gone", func(t *testing.T) {
		rows, err := repos.DeviceProduct.GetByDeviceID(ctx, station.ID)
		require.NoError(t, err)
		require.Empty(t, rows)
	})

	t.Run("binding is revoked and auth is cut off", func(t *testing.T) {
		// GetByTokenHash filters revoked bindings and is the auth gate; a miss
		// here proves both the DB revoke and the cache eviction.
		_, err := repos.DeviceBinding.GetByTokenHash(ctx, stationToken)
		require.ErrorIs(t, err, repository.ErrNotFound)

		active, err := repos.DeviceBinding.ListActiveByDeviceID(ctx, station.ID)
		require.NoError(t, err)
		require.Empty(t, active)
	})

	t.Run("unrelated device is untouched", func(t *testing.T) {
		d, err := repos.Device.GetByID(ctx, other.ID)
		require.NoError(t, err)
		require.Equal(t, other.ID, d.ID)

		rows, err := repos.DeviceProduct.GetByDeviceID(ctx, other.ID)
		require.NoError(t, err)
		require.Len(t, rows, 1)

		b, err := repos.DeviceBinding.GetByTokenHash(ctx, otherToken)
		require.NoError(t, err)
		require.Equal(t, other.ID, *b.DeviceID)
	})
}

// TestDeviceBindingRepo_ListActiveByDeviceID asserts the new lookup filters by
// device_id and excludes already-revoked bindings.
func TestDeviceBindingRepo_ListActiveByDeviceID(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	ctx := context.Background()

	target := fixtures.CreateDevice("Target", "target-key", entDevice.TypeSTATION, entDevice.StatusApproved)
	other := fixtures.CreateDevice("Other", "other-key", entDevice.TypePOS, entDevice.StatusApproved)

	b1 := fixtures.CreateDeviceBinding(entBinding.DeviceTypeSTATION, repository.HashToken("t1"), target.ID)
	b2 := fixtures.CreateDeviceBinding(entBinding.DeviceTypeSTATION, repository.HashToken("t2"), target.ID)
	fixtures.CreateDeviceBinding(entBinding.DeviceTypePOS, repository.HashToken("o1"), other.ID)
	revoked := fixtures.CreateDeviceBinding(entBinding.DeviceTypeSTATION, repository.HashToken("t3"), target.ID)
	require.NoError(t, repos.DeviceBinding.Revoke(ctx, revoked.ID))

	active, err := repos.DeviceBinding.ListActiveByDeviceID(ctx, target.ID)
	require.NoError(t, err)

	ids := make([]string, len(active))
	for i, b := range active {
		ids[i] = b.ID
	}
	require.ElementsMatch(t, []string{b1.ID, b2.ID}, ids,
		"must return only active bindings for the target device (excludes the other device and the revoked binding)")
}
