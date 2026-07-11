package service

import (
	"context"
	"errors"
	"net/http"

	"backend/internal/generated/ent"
)

var (
	ErrVolunteerCampaignNotFound          = errors.New("volunteer_campaign_not_found")
	ErrVolunteerCampaignInactive          = errors.New("volunteer_campaign_inactive")
	ErrVolunteerCampaignOutsideValid      = errors.New("volunteer_campaign_outside_validity")
	ErrVolunteerAccessCodeInvalid         = errors.New("volunteer_access_code_invalid")
	ErrVolunteerCampaignHasNoProducts     = errors.New("volunteer_campaign_has_no_products")
	ErrVolunteerMaxRedemptionsReached     = errors.New("volunteer_max_redemptions_reached")
	ErrVolunteerMaxBelowCount             = errors.New("volunteer_max_redemptions_below_current_count")
	ErrVolunteerStationNoMatchingProducts = errors.New("volunteer_station_has_no_matching_products")
)

// VolunteerErrorResponse maps a known campaign error to its HTTP status, stable
// error code, and user-facing message. ok is false for errors outside the known
// set, which callers surface as a 500. Shared by the core claim handlers and the
// staff-perks plugin so the mapping lives in one place.
func VolunteerErrorResponse(err error) (status int, code, message string, ok bool) {
	switch {
	case errors.Is(err, ErrVolunteerCampaignNotFound):
		return http.StatusNotFound, "campaign_not_found", "The volunteer campaign does not exist.", true
	case errors.Is(err, ErrVolunteerCampaignInactive):
		return http.StatusGone, "campaign_inactive", "This volunteer campaign is no longer active.", true
	case errors.Is(err, ErrVolunteerCampaignOutsideValid):
		return http.StatusGone, "campaign_outside_validity", "This volunteer campaign is outside its validity window.", true
	case errors.Is(err, ErrVolunteerAccessCodeInvalid):
		return http.StatusUnauthorized, "invalid_access_code", "The access code is incorrect.", true
	case errors.Is(err, ErrVolunteerCampaignHasNoProducts):
		return http.StatusBadRequest, "no_products", "At least one product is required.", true
	case errors.Is(err, ErrVolunteerMaxRedemptionsReached):
		return http.StatusConflict, "max_redemptions_reached", "This campaign has reached its maximum number of redemptions.", true
	case errors.Is(err, ErrVolunteerMaxBelowCount):
		return http.StatusConflict, "max_redemptions_below_count", "New maximum cannot be lower than the current redemption count.", true
	case errors.Is(err, ErrVolunteerStationNoMatchingProducts):
		return http.StatusConflict, "station_no_matching_products", "Diese Station bietet keine Artikel aus diesem Gutschein an.", true
	default:
		return 0, "", "", false
	}
}

// CampaignClaimService is implemented by the staff-perks plugin. The public
// /claim and station redeem routes stay hand-wired in core and reach the plugin's
// campaign logic only through these methods.
type CampaignClaimService interface {
	VerifyAccess(ctx context.Context, token string, code string) (sessionID string, err error)
	GetClaimView(ctx context.Context, token string) (*VolunteerClaimView, error)
	RedeemSharedQR(ctx context.Context, claimToken string, stationID string, idempotencyKey string) (*VolunteerRedemptionResult, error)
}

type VolunteerCampaignProductView struct {
	ProductID    string
	ProductName  string
	ProductImage *string
	Quantity     int
}

type VolunteerClaimView struct {
	Campaign  *ent.VolunteerCampaign
	Products  []VolunteerCampaignProductView
	QRPayload string
}

type VolunteerRedemptionResult struct {
	OrderID         string
	RedemptionCount int
	MaxRedemptions  int
	StationResult   map[string]any
}
