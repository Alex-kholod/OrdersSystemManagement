package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Role string

const (
	RoleCustomer Role = "customer"
	RoleAdmin    Role = "admin"
)

type User struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"        json:"id"`
	Name      string    `gorm:"not null"                    json:"name"`
	Email     string    `gorm:"not null"                    json:"email"`
	Password  string    `gorm:"not null"                    json:"-"`
	Role      Role      `gorm:"not null;default:'customer'" json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	if u.Role == "" {
		u.Role = RoleCustomer
	}
	return nil
}
