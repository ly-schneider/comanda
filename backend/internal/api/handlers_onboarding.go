package api

import (
	"net/http"
	"strings"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/response"
)

func nonEmpty(p *string) bool {
	return p != nil && strings.TrimSpace(*p) != ""
}

// GetOnboardingStatus returns the derived onboarding checklist for the vendor's
// tenant. Per-step state is derived from real data (a logo, a Payrexx timestamp,
// a verified domain), so the wizard is inherently resumable; CompletedAt is the
// one persisted flag. Read-only and available to any member so the account
// overview renders for staff too; the steps are gated by the tenant-admin routes.
func (h *Handlers) GetOnboardingStatus(w http.ResponseWriter, r *http.Request, _ generated.AccountTenantParam) {
	ctx := r.Context()
	if _, ok := auth.GetTenantID(ctx); !ok {
		writeError(w, http.StatusForbidden, "no_tenant", "No tenant resolved for this request")
		return
	}

	s, err := h.settings.GetSettings(ctx)
	if err != nil {
		writeEntError(w, err)
		return
	}
	domains, err := h.domains.List(ctx)
	if err != nil {
		writeEntError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, generated.OnboardingStatus{
		CompletedAt: s.OnboardingCompletedAt,
		Steps: generated.OnboardingSteps{
			// The brand step is the legal imprint: company name, address and a
			// contact email. Logo and colours are optional polish, not a gate.
			Brand:    nonEmpty(s.LegalName) && nonEmpty(s.LegalAddress) && nonEmpty(s.ContactEmail),
			Payments: s.PayrexxConfiguredAt != nil,
			Domain:   hasVerifiedBYODomain(domains),
		},
	})
}

// Platform-assigned subdomains are seeded verified but tokenless.
func hasVerifiedBYODomain(domains []*ent.TenantDomain) bool {
	for _, d := range domains {
		if d.VerifiedAt != nil && d.VerificationToken != "" {
			return true
		}
	}
	return false
}

// CompleteOnboarding marks the wizard as finished (or dismissed) for the tenant.
// It only records that the vendor is done; the underlying steps stay editable.
func (h *Handlers) CompleteOnboarding(w http.ResponseWriter, r *http.Request, _ generated.AccountTenantParam) {
	ctx := r.Context()
	if _, ok := auth.GetTenantID(ctx); !ok {
		writeError(w, http.StatusForbidden, "no_tenant", "No tenant resolved for this request")
		return
	}
	if err := h.settings.SetOnboardingCompleted(ctx, true); err != nil {
		writeEntError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
