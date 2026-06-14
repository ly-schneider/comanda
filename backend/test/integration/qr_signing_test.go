package integration

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"testing"

	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/product"
	"backend/internal/qrsign"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/stretchr/testify/require"
)

func decodePub(s string) (ed25519.PublicKey, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(b), nil
}

// tenantCtx returns a background context carrying the default test tenant id so
// services that read repository.TenantIDFromContext (QR signing, settings row
// resolution) behave as they do under the request tx.
func tenantCtx() context.Context {
	return repository.ContextWithTenantID(context.Background(), DefaultTestTenantID)
}

func TestQRKeyService_Idempotency(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	svc := NewQRKeySvc(repos)
	ctx := tenantCtx()

	priv1, pub1, err := svc.EnsureTenantKey(ctx)
	require.NoError(t, err)
	require.Len(t, pub1, 32)

	priv2, pub2, err := svc.EnsureTenantKey(ctx)
	require.NoError(t, err)

	require.Equal(t, []byte(pub1), []byte(pub2), "second EnsureTenantKey must return the same public key")
	require.Equal(t, []byte(priv1), []byte(priv2), "second EnsureTenantKey must return the same private key")

	// Stored public key matches the derived one (decrypt round-trip).
	stored, err := repos.Settings.GetQRSigningKey(ctx)
	require.NoError(t, err)

	tok, err := qrsign.Sign(priv1, qrsign.Payload{Version: qrsign.Version, TenantID: DefaultTestTenantID, OrderID: "o", IssuedAt: 1})
	require.NoError(t, err)
	// Verify with a public key reconstructed from the stored base64url string.
	verPub, err := decodePub(stored.PublicKey)
	require.NoError(t, err)
	_, err = qrsign.Verify(verPub, tok)
	require.NoError(t, err, "stored public key must verify a token signed by the derived private key")
}

func TestPaymentService_SignsQRPayloadAtCreation(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	cfg := TestConfig()
	svc := NewPaymentSvc(cfg, repos, NewProductSvc(repos))
	keys := NewQRKeySvc(repos)
	ctx := tenantCtx()

	category := fixtures.CreateCategory("Drinks", 1, true)
	cola := fixtures.CreateProduct("Cola", category.ID, 350, product.TypeSimple, nil)
	sprite := fixtures.CreateProduct("Sprite", category.ID, 350, product.TypeSimple, nil)
	fixtures.AddInventory(cola.ID, 100, inventoryledger.ReasonOpeningBalance)
	fixtures.AddInventory(sprite.ID, 100, inventoryledger.ReasonOpeningBalance)

	input := service.CreateCheckoutInput{
		Items: []service.CheckoutItemInput{
			{ProductID: cola.ID, Quantity: 2},
			{ProductID: sprite.ID, Quantity: 1},
		},
	}

	prep, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, prep.QRPayload, "created order must carry a signed qr payload")

	// Persisted on the order row.
	ord, err := repos.Order.GetByID(ctx, prep.OrderID)
	require.NoError(t, err)
	require.NotNil(t, ord.QrPayload)
	require.Equal(t, prep.QRPayload, *ord.QrPayload)

	// Verifies with the tenant's stored public key.
	_, pub, err := keys.EnsureTenantKey(ctx)
	require.NoError(t, err)

	payload, err := qrsign.Verify(pub, prep.QRPayload)
	require.NoError(t, err)
	require.Equal(t, qrsign.Version, payload.Version)
	require.Equal(t, DefaultTestTenantID, payload.TenantID)
	require.Equal(t, prep.OrderID, payload.OrderID)
	require.NotZero(t, payload.IssuedAt)

	// Two redeemable (parent) lines: cola x2, sprite x1.
	require.Len(t, payload.Lines, 2)
	byProduct := map[string]int{}
	for _, l := range payload.Lines {
		byProduct[l.ProductID] = l.Quantity
	}
	require.Equal(t, 2, byProduct[cola.ID])
	require.Equal(t, 1, byProduct[sprite.ID])
}
