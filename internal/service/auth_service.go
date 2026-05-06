package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"order-service/internal/domain/models"
	domainrepo "order-service/internal/domain/repository"
)

var ErrEmailTaken = errors.New("email already in use")
var ErrInvalidCredentials = errors.New("invalid email or password")

type AuthClaims struct {
	jwt.RegisteredClaims
	Role models.Role `json:"role"`
}

type AuthService struct {
	users     domainrepo.UserRepository
	jwtSecret []byte
}

func NewAuthService(users domainrepo.UserRepository, jwtSecret string) *AuthService {
	return &AuthService{users: users, jwtSecret: []byte(jwtSecret)}
}

type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

type RegisterOutput struct {
	User  *models.User
	Token string
}

func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*RegisterOutput, error) {
	existing, err := s.users.FindByEmail(ctx, input.Email)
	if err != nil {
		return nil, fmt.Errorf("checking email: %w", err)
	}
	if existing != nil {
		return nil, ErrEmailTaken
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	user := &models.User{
		ID:       uuid.New(),
		Name:     input.Name,
		Email:    input.Email,
		Password: string(hashed),
		Role:     models.RoleCustomer,
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	token, err := s.generateToken(user)
	if err != nil {
		return nil, err
	}

	return &RegisterOutput{User: user, Token: token}, nil
}

type LoginInput struct {
	Email    string
	Password string
}

type LoginOutput struct {
	User  *models.User
	Token string
}

// Login validates credentials and returns a JWT token.
func (s *AuthService) Login(ctx context.Context, input LoginInput) (*LoginOutput, error) {
	user, err := s.users.FindByEmail(ctx, input.Email)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := s.generateToken(user)
	if err != nil {
		return nil, err
	}

	return &LoginOutput{User: user, Token: token}, nil
}

func (s *AuthService) generateToken(user *models.User) (string, error) {
	claims := AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Role: user.Role,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}
	return signed, nil
}

// ParseToken validates and parses a JWT string.
func (s *AuthService) ParseToken(tokenStr string) (*AuthClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AuthClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*AuthClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
