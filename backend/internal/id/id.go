// Package id generates and validates the 12-char nanoid entity IDs.
// App-side only — columns have no DB default (PG17 has no uuidv7()).
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
