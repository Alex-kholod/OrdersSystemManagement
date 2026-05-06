package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"order-service/internal/domain/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// Order model unit tests (no DB needed)
// ─────────────────────────────────────────────────────────────────────────────

func TestIsTransitionAllowed(t *testing.T) {
	cases := []struct {
		from    models.OrderStatus
		to      models.OrderStatus
		allowed bool
	}{
		{models.OrderStatusNew, models.OrderStatusConfirmed, true},
		{models.OrderStatusNew, models.OrderStatusCancelled, true},
		{models.OrderStatusNew, models.OrderStatusShipped, false},
		{models.OrderStatusNew, models.OrderStatusDelivered, false},
		{models.OrderStatusConfirmed, models.OrderStatusPaid, true},
		{models.OrderStatusConfirmed, models.OrderStatusCancelled, true},
		{models.OrderStatusConfirmed, models.OrderStatusNew, false},
		{models.OrderStatusPaid, models.OrderStatusProcessing, true},
		{models.OrderStatusPaid, models.OrderStatusCancelled, true},
		{models.OrderStatusPaid, models.OrderStatusDelivered, false},
		{models.OrderStatusProcessing, models.OrderStatusShipped, true},
		{models.OrderStatusProcessing, models.OrderStatusCancelled, true},
		{models.OrderStatusShipped, models.OrderStatusDelivered, true},
		{models.OrderStatusShipped, models.OrderStatusCancelled, false},
		{models.OrderStatusDelivered, models.OrderStatusCancelled, false},
		{models.OrderStatusDelivered, models.OrderStatusNew, false},
		{models.OrderStatusCancelled, models.OrderStatusNew, false},
	}

	for _, tc := range cases {
		t.Run(string(tc.from)+"→"+string(tc.to), func(t *testing.T) {
			got := models.IsTransitionAllowed(tc.from, tc.to)
			assert.Equal(t, tc.allowed, got)
		})
	}
}

func TestCancellableStatuses(t *testing.T) {
	cancellable := []models.OrderStatus{
		models.OrderStatusNew,
		models.OrderStatusConfirmed,
		models.OrderStatusPaid,
		models.OrderStatusProcessing,
	}
	notCancellable := []models.OrderStatus{
		models.OrderStatusShipped,
		models.OrderStatusDelivered,
		models.OrderStatusCancelled,
	}

	for _, s := range cancellable {
		assert.True(t, models.CancellableStatuses[s], "%s should be cancellable", s)
	}
	for _, s := range notCancellable {
		assert.False(t, models.CancellableStatuses[s], "%s should NOT be cancellable", s)
	}
}

func TestOrderItem_BeforeCreate(t *testing.T) {
	item := &models.OrderItem{
		OrderID:   uuid.New(),
		ProductID: uuid.New(),
		Quantity:  2,
		Price:     99.99,
	}
	assert.Equal(t, uuid.Nil, item.ID)
	err := item.BeforeCreate(nil)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, item.ID)
}

func TestUser_BeforeCreate_SetsDefaults(t *testing.T) {
	u := &models.User{
		Name:     "Test",
		Email:    "t@t.com",
		Password: "hash",
	}
	err := u.BeforeCreate(nil)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, u.ID)
	assert.Equal(t, models.RoleCustomer, u.Role)
}

func TestUser_BeforeCreate_PreservesExistingID(t *testing.T) {
	existingID := uuid.New()
	u := &models.User{
		ID:       existingID,
		Name:     "Test",
		Email:    "t@t.com",
		Password: "hash",
		Role:     models.RoleAdmin,
	}
	err := u.BeforeCreate(nil)
	require.NoError(t, err)
	assert.Equal(t, existingID, u.ID)
	assert.Equal(t, models.RoleAdmin, u.Role)
}

func TestProduct_BeforeCreate(t *testing.T) {
	p := &models.Product{Name: "Test", Price: 10.0}
	err := p.BeforeCreate(nil)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, p.ID)
}

// ─────────────────────────────────────────────────────────────────────────────
// ProductService unit tests with mock
// ─────────────────────────────────────────────────────────────────────────────

type mockProductRepo struct{}

func (m *mockProductRepo) Create(_ context.Context, p *models.Product) error { return nil }
func (m *mockProductRepo) FindByID(_ context.Context, id uuid.UUID) (*models.Product, error) {
	if id == uuid.Nil {
		return nil, nil
	}
	return &models.Product{ID: id, Name: "Mock Product", Price: 100.0, Stock: 10}, nil
}
func (m *mockProductRepo) List(_ context.Context, _ interface{}) ([]*models.Product, int64, error) {
	return nil, 0, nil
}
func (m *mockProductRepo) Update(_ context.Context, p *models.Product) error         { return nil }
func (m *mockProductRepo) Delete(_ context.Context, id uuid.UUID) error              { return nil }
func (m *mockProductRepo) DecreaseStock(_ context.Context, _ uuid.UUID, _ int) error { return nil }
func (m *mockProductRepo) IncreaseStock(_ context.Context, _ uuid.UUID, _ int) error { return nil }
