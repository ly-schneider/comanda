// Package id generates and validates Comanda entity IDs: 12-char nanoids
// drawn from a 55-char alphabet that excludes the confusables 0/O/o, I/i,
// J/j, and L/l. IDs are generated app-side only (crypto/rand) — Postgres
// columns are varchar(12) with no DB-side default, because the platform's
// managed Postgres (PG17) has no uuidv7() and we don't want a DB generator.
package id

import (
	gonanoid "github.com/matoous/go-nanoid/v2"
)

const (
	Alphabet = "123456789ABCDEFGHKMNPQRSTUVWXYZ_abcdefghkmnpqrstuvwxyz-"
	Length   = 12
)

var inAlphabet [256]bool

func init() {
	for i := 0; i < len(Alphabet); i++ {
		inAlphabet[Alphabet[i]] = true
	}
}

func New() string {
	return gonanoid.MustGenerate(Alphabet, Length)
}

func Valid(s string) bool {
	if len(s) != Length {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !inAlphabet[s[i]] {
			return false
		}
	}
	return true
}
