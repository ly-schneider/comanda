package api

import (
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/internal/response"
)

// onboardingStatus reports the vendor's onboarding progress. Per-step state is
// derived from real data (a logo means branding is set, a Payrexx timestamp
// means payments are wired, a verified domain means a custom domain is live), so
// the wizard is inherently resumable: the data IS the progress. CompletedAt is
// the one persisted flag, set when the vendor finishes or dismisses the wizard.
type onboardingStatus struct {
	CompletedAt *time.Time      `json:"completedAt"`
	Steps       map[string]bool `json:"steps"`
}

// GetOnboardingStatus returns the derived onboarding checklist for the vendor's
// tenant. Read-only and available to any member so the account overview renders
// for staff too; the steps themselves are gated by the tenant-admin routes.
func (h *Handlers) GetOnboardingStatus(w http.ResponseWriter, r *http.Request) {
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

	hasVerifiedDomain := false
	for _, d := range domains {
		if d.VerifiedAt != nil {
			hasVerifiedDomain = true
			break
		}
	}

	response.WriteJSON(w, http.StatusOK, onboardingStatus{
		CompletedAt: s.OnboardingCompletedAt,
		Steps: map[string]bool{
			"brand":    s.LogoURL != nil && *s.LogoURL != "",
			"payments": s.PayrexxConfiguredAt != nil,
			"domain":   hasVerifiedDomain,
		},
	})
}

// CompleteOnboarding marks the wizard as finished (or dismissed) for the tenant.
// It only records that the vendor is done; the underlying steps stay editable.
func (h *Handlers) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
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
