package service

import (
	"context"
	"errors"
	"fmt"

	"backend/internal/config"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/device"
	"backend/internal/generated/ent/order"
	"backend/internal/repository"
)

type POSService interface {
	// Device
	GetDeviceByToken(ctx context.Context, token string) (*ent.Device, error)
	GetDeviceByID(ctx context.Context, id string) (*ent.Device, error)
	// Orders
	CreateOrder(ctx context.Context, items []POSCheckoutItem, customerEmail *string) (string, error)
	PayCash(ctx context.Context, orderID string, deviceID *string, pluginKey *string) error
	PayCard(ctx context.Context, orderID string, deviceID *string, card *repository.CardMeta, pluginKey *string) error
	PayTwint(ctx context.Context, orderID string, deviceID *string, pluginKey *string) error
	PayGratisGuest(ctx context.Context, orderID string, deviceID *string) error
	PayGratisVIP(ctx context.Context, orderID string, deviceID *string) error
	PayGratisStaff(ctx context.Context, orderID string, deviceID *string) error
	PayGratisPlugin(ctx context.Context, orderID string, deviceID *string, pluginKey string) error
}

type POSCheckoutItem struct {
	ProductID             string            `json:"productId"`
	Quantity              int               `json:"quantity"`
	Configuration         map[string]string `json:"configuration,omitempty"`
	ConfigurationVariants map[string]string `json:"configurationVariants,omitempty"`
	VariantID             *string           `json:"variantId,omitempty"`
}

type posService struct {
	cfg      config.Config
	devices  repository.DeviceRepository
	orders   repository.OrderRepository
	payments PaymentService
}

func NewPOSService(
	cfg config.Config,
	devices repository.DeviceRepository,
	orders repository.OrderRepository,
	payments PaymentService,
) POSService {
	return &posService{
		cfg:      cfg,
		devices:  devices,
		orders:   orders,
		payments: payments,
	}
}

func (s *posService) GetDeviceByToken(ctx context.Context, token string) (*ent.Device, error) {
	d, err := s.devices.GetByDeviceKey(ctx, token)
	if err != nil {
		return nil, err
	}
	if d.Type != device.TypePOS {
		return nil, errors.New("device_not_pos")
	}
	return d, nil
}

func (s *posService) GetDeviceByID(ctx context.Context, id string) (*ent.Device, error) {
	d, err := s.devices.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if d.Type != device.TypePOS {
		return nil, errors.New("device_not_pos")
	}
	return d, nil
}

func (s *posService) CreateOrder(ctx context.Context, items []POSCheckoutItem, customerEmail *string) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no items")
	}

	// Convert to payment service input
	checkoutItems := make([]CheckoutItemInput, len(items))
	for i, item := range items {
		checkoutItems[i] = CheckoutItemInput(item)
	}

	in := CreateCheckoutInput{
		Items:         checkoutItems,
		CustomerEmail: customerEmail,
		Origin:        order.OriginPos,
	}

	prep, err := s.payments.PrepareAndCreateOrder(ctx, in, nil, nil)
	if err != nil {
		return "", err
	}

	return prep.OrderID, nil
}

func (s *posService) PayCash(ctx context.Context, orderID string, deviceID *string, pluginKey *string) error {
	if orderID == "" {
		return fmt.Errorf("invalid order id")
	}

	ord, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if ord.Status != order.StatusPending {
		return fmt.Errorf("not_pending")
	}

	return s.orders.SetPosPaymentCash(ctx, orderID, deviceID, ord.TotalCents, pluginKey)
}

func (s *posService) PayCard(ctx context.Context, orderID string, deviceID *string, card *repository.CardMeta, pluginKey *string) error {
	if orderID == "" {
		return fmt.Errorf("invalid order id")
	}

	ord, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if ord.Status != order.StatusPending {
		return fmt.Errorf("not_pending")
	}

	return s.orders.SetPosPaymentCard(ctx, orderID, deviceID, ord.TotalCents, card, pluginKey)
}

func (s *posService) PayTwint(ctx context.Context, orderID string, deviceID *string, pluginKey *string) error {
	if orderID == "" {
		return fmt.Errorf("invalid order id")
	}

	ord, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if ord.Status != order.StatusPending {
		return fmt.Errorf("not_pending")
	}

	return s.orders.SetPosPaymentTwint(ctx, orderID, deviceID, ord.TotalCents, pluginKey)
}

func (s *posService) PayGratisGuest(ctx context.Context, orderID string, deviceID *string) error {
	if orderID == "" {
		return fmt.Errorf("invalid order id")
	}

	ord, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if ord.Status != order.StatusPending {
		return fmt.Errorf("not_pending")
	}

	return s.orders.SetPosPaymentGratisGuest(ctx, orderID, deviceID, ord.TotalCents)
}

func (s *posService) PayGratisVIP(ctx context.Context, orderID string, deviceID *string) error {
	if orderID == "" {
		return fmt.Errorf("invalid order id")
	}

	ord, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if ord.Status != order.StatusPending {
		return fmt.Errorf("not_pending")
	}

	return s.orders.SetPosPaymentGratisVIP(ctx, orderID, deviceID, ord.TotalCents)
}

func (s *posService) PayGratisStaff(ctx context.Context, orderID string, deviceID *string) error {
	if orderID == "" {
		return fmt.Errorf("invalid order id")
	}

	ord, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if ord.Status != order.StatusPending {
		return fmt.Errorf("not_pending")
	}

	return s.orders.SetPosPaymentGratisStaff(ctx, orderID, deviceID, ord.TotalCents)
}

func (s *posService) PayGratisPlugin(ctx context.Context, orderID string, deviceID *string, pluginKey string) error {
	if orderID == "" {
		return fmt.Errorf("invalid order id")
	}

	ord, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if ord.Status != order.StatusPending {
		return fmt.Errorf("not_pending")
	}

	return s.orders.SetPosPaymentGratisPlugin(ctx, orderID, deviceID, ord.TotalCents, pluginKey)
}
