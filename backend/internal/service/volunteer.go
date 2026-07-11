package service

import (
	"context"
	"errors"

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
