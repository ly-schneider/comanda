package api

import (
	"testing"
	"time"

	"backend/internal/generated/ent"

	"github.com/stretchr/testify/require"
)

func TestHasVerifiedBYODomain(t *testing.T) {
	now := time.Now()

	platformSeeded := &ent.TenantDomain{VerifiedAt: &now}
	byoUnverified := &ent.TenantDomain{VerificationToken: "tok"}
	byoVerified := &ent.TenantDomain{VerifiedAt: &now, VerificationToken: "tok"}

	require.False(t, hasVerifiedBYODomain(nil))
	require.False(t, hasVerifiedBYODomain([]*ent.TenantDomain{platformSeeded}),
		"platform-seeded domain must not count")
	require.False(t, hasVerifiedBYODomain([]*ent.TenantDomain{platformSeeded, byoUnverified}))
	require.True(t, hasVerifiedBYODomain([]*ent.TenantDomain{platformSeeded, byoVerified}))
}
