// Package id generates and validates the 12-char nanoid entity IDs.
// App-side only — columns have no DB default (PG17 has no uuidv7()).
package id

import (
	gonanoid "github.com/matoous/go-nanoid/v2"
)

const (
	// No 0OoIiJjLl confusables; no '-'/'_' (invalid at K8s label-value edges).
	Alphabet = "123456789ABCDEFGHKMNPQRSTUVWXYZabcdefghkmnpqrstuvwxyz"
	// Validation also accepts '-'/'_': existing rows and seed sentinels carry them.
	validChars = Alphabet + "-_"
	Length     = 12
)

var validChar [256]bool

func init() {
	for i := 0; i < len(validChars); i++ {
		validChar[validChars[i]] = true
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
		if !validChar[s[i]] {
			return false
		}
	}
	return true
}
