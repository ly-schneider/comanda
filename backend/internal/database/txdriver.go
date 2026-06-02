package database

import (
	"context"
	"database/sql"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

// txDriver wraps an Ent SQL driver that is bound to an already-open
// database/sql transaction (the per-request tenant tx). Its Tx() returns a
// no-op transaction over the same connection instead of trying to BeginTx on a
// *sql.DB (which Ent's Driver.Tx does via DB() and which panics when the
// underlying ExecQuerier is a *sql.Tx). This makes Ent graph operations that
// internally request a transaction reuse the outer request tx, so a service or
// repository never has to manage nested transactions explicitly.
type txDriver struct {
	*entsql.Driver
}

// NewTxEntDriver binds an Ent driver to an open *sql.Tx so the resulting Ent
// client runs every statement inside that transaction and treats nested Tx()
// calls as no-ops over the same tx.
func NewTxEntDriver(tx *sql.Tx) dialect.Driver {
	return &txDriver{Driver: entsql.NewDriver(dialect.Postgres, entsql.Conn{ExecQuerier: tx})}
}

func (d *txDriver) Tx(context.Context) (dialect.Tx, error) {
	return dialect.NopTx(d), nil
}

func (d *txDriver) BeginTx(context.Context, *entsql.TxOptions) (dialect.Tx, error) {
	return dialect.NopTx(d), nil
}
