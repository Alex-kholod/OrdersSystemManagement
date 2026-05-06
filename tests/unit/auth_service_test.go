package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"order-service/internal/domain/models"
	"order-service/internal/service"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mock UserRepository
// ─────────────────────────────────────────────────────────────────────────────

type mockUserRepo struct{ mock.Mock }

func (m *mockUserRepo) Create(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *mockUserRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

// ─────────────────────────────────────────────────────────────────────────────
// Register tests
// ─────────────────────────────────────────────────────────────────────────────

func TestAuthService_Register_Success(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthService(repo, "secret")

	repo.On("FindByEmail", mock.Anything, "new@test.com").Return(nil, nil)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*models.User")).Return(nil)

	out, err := svc.Register(context.Background(), service.RegisterInput{
		Name:     "Test User",
		Email:    "new@test.com",
		Password: "password123",
	})

	require.NoError(t, err)
	assert.NotNil(t, out.User)
	assert.NotEmpty(t, out.Token)
	assert.Equal(t, "new@test.com", out.User.Email)
	assert.Equal(t, models.RoleCustomer, out.User.Role)
	repo.AssertExpectations(t)
}

func TestAuthService_Register_EmailTaken(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthService(repo, "secret")

	existing := &models.User{ID: uuid.New(), Email: "taken@test.com"}
	repo.On("FindByEmail", mock.Anything, "taken@test.com").Return(existing, nil)

	out, err := svc.Register(context.Background(), service.RegisterInput{
		Name:     "X",
		Email:    "taken@test.com",
		Password: "pass",
	})

	assert.Nil(t, out)
	assert.ErrorIs(t, err, service.ErrEmailTaken)
	repo.AssertExpectations(t)
}

func TestAuthService_Register_RepoError(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthService(repo, "secret")

	repo.On("FindByEmail", mock.Anything, mock.Anything).Return(nil, nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("db error"))

	out, err := svc.Register(context.Background(), service.RegisterInput{
		Name:     "X",
		Email:    "err@test.com",
		Password: "pass123",
	})

	assert.Nil(t, out)
	assert.Error(t, err)
}

// ─────────────────────────────────────────────────────────────────────────────
// Login tests
// ─────────────────────────────────────────────────────────────────────────────

func TestAuthService_Login_Success(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthService(repo, "secret")

	// Pre-register to get a bcrypt hash
	repo.On("FindByEmail", mock.Anything, "user@test.com").Return(nil, nil).Once()
	repo.On("Create", mock.Anything, mock.AnythingOfType("*models.User")).
		Run(func(args mock.Arguments) {
			// nothing — user is stored in mock
		}).Return(nil).Once()

	regOut, err := svc.Register(context.Background(), service.RegisterInput{
		Name:     "User",
		Email:    "user@test.com",
		Password: "mypassword",
	})
	require.NoError(t, err)

	// Now simulate login lookup returning the registered user
	repo.On("FindByEmail", mock.Anything, "user@test.com").Return(regOut.User, nil).Once()

	loginOut, err := svc.Login(context.Background(), service.LoginInput{
		Email:    "user@test.com",
		Password: "mypassword",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, loginOut.Token)
	assert.Equal(t, regOut.User.ID, loginOut.User.ID)
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthService(repo, "secret")

	// Register first to get hashed password
	repo.On("FindByEmail", mock.Anything, "user2@test.com").Return(nil, nil).Once()
	repo.On("Create", mock.Anything, mock.AnythingOfType("*models.User")).Return(nil).Once()

	regOut, err := svc.Register(context.Background(), service.RegisterInput{
		Name:     "User2",
		Email:    "user2@test.com",
		Password: "correctpass",
	})
	require.NoError(t, err)

	repo.On("FindByEmail", mock.Anything, "user2@test.com").Return(regOut.User, nil).Once()

	out, err := svc.Login(context.Background(), service.LoginInput{
		Email:    "user2@test.com",
		Password: "wrongpass",
	})

	assert.Nil(t, out)
	assert.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestAuthService_Login_UserNotFound(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthService(repo, "secret")

	repo.On("FindByEmail", mock.Anything, "ghost@test.com").Return(nil, nil)

	out, err := svc.Login(context.Background(), service.LoginInput{
		Email:    "ghost@test.com",
		Password: "anypass",
	})

	assert.Nil(t, out)
	assert.ErrorIs(t, err, service.ErrInvalidCredentials)
}

// ─────────────────────────────────────────────────────────────────────────────
// ParseToken tests
// ─────────────────────────────────────────────────────────────────────────────

func TestAuthService_ParseToken_Valid(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthService(repo, "my-secret")

	repo.On("FindByEmail", mock.Anything, mock.Anything).Return(nil, nil)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*models.User")).Return(nil)

	out, err := svc.Register(context.Background(), service.RegisterInput{
		Name:     "Token User",
		Email:    "tok@test.com",
		Password: "pass1234",
	})
	require.NoError(t, err)

	claims, err := svc.ParseToken(out.Token)
	require.NoError(t, err)
	assert.Equal(t, out.User.ID.String(), claims.Subject)
	assert.Equal(t, models.RoleCustomer, claims.Role)
}

func TestAuthService_ParseToken_Invalid(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthService(repo, "my-secret")

	_, err := svc.ParseToken("this.is.not.a.valid.token")
	assert.Error(t, err)
}

func TestAuthService_ParseToken_WrongSecret(t *testing.T) {
	repo := new(mockUserRepo)
	svc1 := service.NewAuthService(repo, "secret-A")
	svc2 := service.NewAuthService(repo, "secret-B")

	repo.On("FindByEmail", mock.Anything, mock.Anything).Return(nil, nil)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*models.User")).Return(nil)

	out, err := svc1.Register(context.Background(), service.RegisterInput{
		Name:     "X",
		Email:    "x@test.com",
		Password: "pass1234",
	})
	require.NoError(t, err)

	_, err = svc2.ParseToken(out.Token)
	assert.Error(t, err)
}
