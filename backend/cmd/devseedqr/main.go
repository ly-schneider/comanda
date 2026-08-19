// Command devseedqr provisions a tenant's Ed25519 pickup key and signs the
// pickup tokens of seeded orders. Local dev tool for the db/seed/dev fixtures;
// not wired into the app.
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"backend/internal/crypto"
	"backend/internal/qrsign"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

func main() {
	tenant := flag.String("tenant", "", "tenant id")
	dsn := flag.String("dsn", "postgres://postgres:postgres@localhost:5432/comanda", "database url")
	envFile := flag.String("env", ".env.local", "env file holding PAYMENTS_MASTER_KEY")
	from := flag.String("from", "2026-08-14", "sign orders created on or after this date")
	flag.Parse()

	if *tenant == "" {
		log.Fatal("-tenant is required")
	}
	if err := godotenv.Overload(*envFile); err != nil {
		log.Printf("warning: could not load %s: %v", *envFile, err)
	}
	master := os.Getenv("PAYMENTS_MASTER_KEY")
	if master == "" {
		log.Fatal("PAYMENTS_MASTER_KEY is not set")
	}
	enc, err := crypto.NewEncryptorFromBase64(master)
	if err != nil {
		log.Fatalf("master key: %v", err)
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, *dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	priv, err := ensureKey(ctx, conn, enc, *tenant)
	if err != nil {
		log.Fatalf("qr key: %v", err)
	}

	signed, err := signOrders(ctx, conn, priv, *tenant, *from)
	if err != nil {
		log.Fatalf("sign: %v", err)
	}
	fmt.Printf("signed %d pickup tokens\n", signed)

	if err := verifyOne(ctx, conn, *tenant); err != nil {
		log.Fatalf("verify: %v", err)
	}
}

// verifyOne reads a token back and checks it against the tenant's stored public
// key — proof the seeded orders would actually scan at a station.
func verifyOne(ctx context.Context, conn *pgx.Conn, tenant string) error {
	var pubB64, token, orderID string
	err := conn.QueryRow(ctx,
		`SELECT s.ed25519_public_key, o.qr_payload, o.id
		   FROM settings s
		   JOIN "order" o ON o.tenant_id = s.tenant_id
		  WHERE s.tenant_id = $1 AND o.qr_payload IS NOT NULL
		  LIMIT 1`, tenant).Scan(&pubB64, &token, &orderID)
	if err != nil {
		return err
	}
	pub, err := base64.RawURLEncoding.DecodeString(pubB64)
	if err != nil {
		return err
	}
	payload, err := qrsign.Verify(ed25519.PublicKey(pub), token)
	if err != nil {
		return err
	}
	if payload.OrderID != orderID || payload.TenantID != tenant {
		return fmt.Errorf("token resolves to %s/%s, want %s/%s", payload.TenantID, payload.OrderID, tenant, orderID)
	}
	fmt.Printf("verified %s: v%d, %d redeemable line(s)\n", orderID, payload.Version, len(payload.Lines))
	return nil
}

func ensureKey(ctx context.Context, conn *pgx.Conn, enc *crypto.Encryptor, tenant string) (ed25519.PrivateKey, error) {
	var seedEnc *string
	err := conn.QueryRow(ctx,
		`SELECT ed25519_private_seed_enc FROM settings WHERE tenant_id = $1`, tenant,
	).Scan(&seedEnc)
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}

	if seedEnc != nil {
		seed, err := enc.Decrypt(*seedEnc)
		if err != nil {
			return nil, fmt.Errorf("decrypt seed: %w", err)
		}
		return ed25519.NewKeyFromSeed(seed), nil
	}

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	blob, err := enc.Encrypt(priv.Seed())
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx,
		`UPDATE settings SET ed25519_private_seed_enc = $1, ed25519_public_key = $2, updated_at = now()
		 WHERE tenant_id = $3`,
		blob, base64.RawURLEncoding.EncodeToString(pub), tenant,
	); err != nil {
		return nil, fmt.Errorf("persist key: %w", err)
	}
	fmt.Println("generated a new pickup signing key")
	return priv, nil
}

// signOrders mirrors the checkout's redeemable set: the physical items are the
// simple lines and the chosen bundle components, and a bundle parent only counts
// when it has no components of its own.
func signOrders(ctx context.Context, conn *pgx.Conn, priv ed25519.PrivateKey, tenant, from string) (int, error) {
	rows, err := conn.Query(ctx,
		`SELECT o.id, o.created_at,
		        l.id, l.line_type::text, l.product_id, coalesce(l.variant_id, ''), l.quantity,
		        coalesce(l.parent_line_id, '')
		   FROM "order" o
		   JOIN order_line l ON l.order_id = o.id
		  WHERE o.tenant_id = $1
		    AND o.created_at >= $2::date
		    AND o.status IN ('paid', 'partially_refunded', 'refunded')
		  ORDER BY o.id`, tenant, from)
	if err != nil {
		return 0, err
	}

	type line struct {
		id, lineType, productID, variantID, parentID string
		quantity                                     int
	}
	orders := map[string][]line{}
	issuedAt := map[string]time.Time{}
	var order []string
	for rows.Next() {
		var orderID string
		var createdAt time.Time
		var l line
		if err := rows.Scan(&orderID, &createdAt, &l.id, &l.lineType, &l.productID, &l.variantID, &l.quantity, &l.parentID); err != nil {
			rows.Close()
			return 0, err
		}
		if _, seen := orders[orderID]; !seen {
			order = append(order, orderID)
			issuedAt[orderID] = createdAt
		}
		orders[orderID] = append(orders[orderID], l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	batch := &pgx.Batch{}
	for _, orderID := range order {
		hasComponents := map[string]bool{}
		for _, l := range orders[orderID] {
			if l.lineType == "component" && l.parentID != "" {
				hasComponents[l.parentID] = true
			}
		}

		var redeemable []qrsign.Line
		for _, l := range orders[orderID] {
			switch l.lineType {
			case "simple", "component":
			case "bundle":
				if hasComponents[l.id] {
					continue
				}
			default:
				continue
			}
			redeemable = append(redeemable, qrsign.Line{
				ProductID: l.productID, VariantID: l.variantID, Quantity: l.quantity,
			})
		}
		if len(redeemable) == 0 {
			continue
		}

		token, err := qrsign.Sign(priv, qrsign.Payload{
			Version:  qrsign.Version,
			TenantID: tenant,
			OrderID:  orderID,
			IssuedAt: issuedAt[orderID].Unix(),
			Lines:    redeemable,
		})
		if err != nil {
			return 0, fmt.Errorf("order %s: %w", orderID, err)
		}
		batch.Queue(`UPDATE "order" SET qr_payload = $1 WHERE id = $2`, token, orderID)
	}

	results := conn.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()
	for i := 0; i < batch.Len(); i++ {
		if _, err := results.Exec(); err != nil {
			return 0, err
		}
	}
	return batch.Len(), nil
}
