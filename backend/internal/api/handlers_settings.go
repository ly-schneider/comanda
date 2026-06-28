package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/auth"
	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/settings"
	"backend/internal/response"
	"backend/internal/service"
)

// GetSettings returns the current application settings.
// (GET /settings)
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	settingsData, err := h.settings.GetSettingsWithProducts(ctx)
	if err != nil {
		writeEntError(w, err)
		return
	}

	missingJetons, _ := h.products.CountActiveWithoutJeton(ctx)

	apiSettings := toAPISettings(settingsData, int(missingJetons))
	response.WriteJSON(w, http.StatusOK, apiSettings)
}

// UpdateSettings updates the application settings.
// (PATCH /settings)
func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var body generated.SettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.SystemEnabled != nil {
		if err := h.settings.SetSystemEnabled(ctx, *body.SystemEnabled); err != nil {
			writeError(w, http.StatusInternalServerError, "update_failed", err.Error())
			return
		}
	}

	if body.PosMode != nil {
		if err := h.settings.SetPosMode(ctx, settings.PosMode(*body.PosMode)); err != nil {
			var missingErr service.MissingJetonForActiveProductsError
			if errors.As(err, &missingErr) {
				response.WriteJSON(w, http.StatusBadRequest, map[string]any{
					"error":   "missing_jetons",
					"message": "Active products missing jeton",
					"missing": missingErr.Count,
				})
				return
			}
			writeError(w, http.StatusBadRequest, "update_failed", err.Error())
			return
		}
	}

	if brandingPresent(&body) {
		// Branding is tenant-admin only: an app-admin may PATCH food-system fields
		// but not branding, even via a crafted request.
		ok, err := auth.HasTenantTier(ctx, auth.TierTenantAdmin)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update_failed", err.Error())
			return
		}
		if !ok {
			writeError(w, http.StatusForbidden, "insufficient_role", "Branding requires the tenant-admin tier")
			return
		}
		in := service.BrandingInput{
			DisplayName:     body.DisplayName,
			ColorPrimary:    body.ColorPrimary,
			ColorPrimaryFg:  body.ColorPrimaryForeground,
			ColorSecondary:  body.ColorSecondary,
			ColorBackground: body.ColorBackground,
			ColorForeground: body.ColorForeground,
			FontKey:         body.FontKey,
			RadiusKey:       body.RadiusKey,
			ContactEmail:    body.ContactEmail,
			ContactPhone:    body.ContactPhone,
			LegalName:       body.LegalName,
			LegalAddress:    body.LegalAddress,
			Currency:        body.Currency,
			PaymentProvider: body.PaymentProvider,
		}
		if err := h.settings.UpdateBranding(ctx, in); err != nil {
			if errors.Is(err, service.ErrInvalidBranding) {
				writeError(w, http.StatusBadRequest, "invalid_body", "Invalid branding (hex color or currency)")
				return
			}
			writeError(w, http.StatusInternalServerError, "update_failed", err.Error())
			return
		}
	}

	if body.Club100FreeProductIds != nil || body.Club100MaxRedemptions != nil {
		var productIDs []string
		if body.Club100FreeProductIds != nil {
			productIDs = make([]string, 0, len(*body.Club100FreeProductIds))
			for _, id := range *body.Club100FreeProductIds {
				productIDs = append(productIDs, string(id))
			}
		}
		if err := h.settings.SetClub100Settings(ctx, productIDs, body.Club100MaxRedemptions); err != nil {
			writeError(w, http.StatusBadRequest, "update_failed", err.Error())
			return
		}
	}

	settingsData, err := h.settings.GetSettingsWithProducts(ctx)
	if err != nil {
		writeEntError(w, err)
		return
	}

	missingJetons, _ := h.products.CountActiveWithoutJeton(ctx)
	response.WriteJSON(w, http.StatusOK, toAPISettings(settingsData, int(missingJetons)))
}

func toAPISettings(e *ent.Settings, missingJetons int) generated.Settings {
	s := generated.Settings{
		Id:                     e.ID,
		SystemEnabled:          e.SystemEnabled,
		PosMode:                generated.PosFulfillmentMode(e.PosMode),
		Club100MaxRedemptions:  e.Club100MaxRedemptions,
		DisplayName:            e.DisplayName,
		LogoUrl:                e.LogoURL,
		FaviconUrl:             e.FaviconURL,
		ColorPrimary:           ptr(e.ColorPrimary),
		ColorPrimaryForeground: ptr(e.ColorPrimaryForeground),
		ColorSecondary:         ptr(e.ColorSecondary),
		ColorBackground:        ptr(e.ColorBackground),
		ColorForeground:        ptr(e.ColorForeground),
		FontKey:                ptr(e.FontKey),
		RadiusKey:              ptr(e.RadiusKey),
		ContactEmail:           e.ContactEmail,
		ContactPhone:           e.ContactPhone,
		LegalName:              e.LegalName,
		LegalAddress:           e.LegalAddress,
		Currency:               ptr(e.Currency),
		PaymentProvider:        ptr(e.PaymentProvider),
		UpdatedAt:              ptr(e.UpdatedAt),
	}

	if missingJetons > 0 {
		s.MissingJetons = ptr(missingJetons)
	}

	if e.Edges.Club100FreeProducts != nil {
		ids := make([]string, 0, len(e.Edges.Club100FreeProducts))
		for _, p := range e.Edges.Club100FreeProducts {
			ids = append(ids, p.ID)
		}
		s.Club100FreeProductIds = &ids
	}

	return s
}

// brandingPresent reports whether the update body carries any branding field.
// logoUrl/faviconUrl are set via the dedicated upload endpoint, not here.
func brandingPresent(b *generated.SettingsUpdate) bool {
	return b.DisplayName != nil ||
		b.ColorPrimary != nil ||
		b.ColorPrimaryForeground != nil ||
		b.ColorSecondary != nil ||
		b.ColorBackground != nil ||
		b.ColorForeground != nil ||
		b.FontKey != nil ||
		b.RadiusKey != nil ||
		b.ContactEmail != nil ||
		b.ContactPhone != nil ||
		b.LegalName != nil ||
		b.LegalAddress != nil ||
		b.Currency != nil ||
		b.PaymentProvider != nil
}
