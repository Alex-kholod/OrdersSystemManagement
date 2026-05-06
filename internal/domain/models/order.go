package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrderStatus string

const (
	OrderStatusNew        OrderStatus = "new"
	OrderStatusConfirmed  OrderStatus = "confirmed"
	OrderStatusPaid       OrderStatus = "paid"
	OrderStatusProcessing OrderStatus = "processing"
	OrderStatusShipped    OrderStatus = "shipped"
	OrderStatusDelivered  OrderStatus = "delivered"
	OrderStatusCancelled  OrderStatus = "cancelled"
)

var AllowedTransitions = map[OrderStatus][]OrderStatus{
	OrderStatusNew:        {OrderStatusConfirmed, OrderStatusCancelled},
	OrderStatusConfirmed:  {OrderStatusPaid, OrderStatusCancelled},
	OrderStatusPaid:       {OrderStatusProcessing, OrderStatusCancelled},
	OrderStatusProcessing: {OrderStatusShipped, OrderStatusCancelled},
	OrderStatusShipped:    {OrderStatusDelivered},
	OrderStatusDelivered:  {},
	OrderStatusCancelled:  {},
}

var CancellableStatuses = map[OrderStatus]bool{
	OrderStatusNew:        true,
	OrderStatusConfirmed:  true,
	OrderStatusPaid:       true,
	OrderStatusProcessing: true,
}

type Order struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey"            json:"id"`
	UserID      uuid.UUID   `gorm:"type:uuid;not null"              json:"user_id"`
	User        *User       `gorm:"foreignKey:UserID"               json:"user,omitempty"`
	Status      OrderStatus `gorm:"not null;default:'new'"          json:"status"`
	Address     string      `gorm:"not null"                        json:"address"`
	TotalAmount float64     `gorm:"type:decimal(10,2)"              json:"total_amount"`
	Items       []OrderItem `gorm:"foreignKey:OrderID"              json:"items,omitempty"`
	CreatedAt   time.Time   `                                        json:"created_at"`
	UpdatedAt   time.Time   `                                        json:"updated_at"`
}

func (o *Order) BeforeCreate(tx *gorm.DB) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	if o.Status == "" {
		o.Status = OrderStatusNew
	}
	return nil
}

type OrderItem struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"        json:"id"`
	OrderID   uuid.UUID `gorm:"type:uuid;not null"          json:"order_id"`
	ProductID uuid.UUID `gorm:"type:uuid;not null"          json:"product_id"`
	Product   *Product  `gorm:"foreignKey:ProductID"        json:"product,omitempty"`
	Quantity  int       `gorm:"not null"                    json:"quantity"`
	Price     float64   `gorm:"type:decimal(10,2);not null" json:"price"`
}

func (oi *OrderItem) BeforeCreate(tx *gorm.DB) error {
	if oi.ID == uuid.Nil {
		oi.ID = uuid.New()
	}
	return nil
}

func IsTransitionAllowed(current, next OrderStatus) bool {
	allowed, ok := AllowedTransitions[current]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == next {
			return true
		}
	}
	return false
}
