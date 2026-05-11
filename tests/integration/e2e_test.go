package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"order-service/internal/domain/models"
	"order-service/internal/handler"
	"order-service/internal/middleware"
	repoImpl "order-service/internal/repository"
	"order-service/internal/service"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test Suite
// ─────────────────────────────────────────────────────────────────────────────

type E2ESuite struct {
	suite.Suite
	db          *gorm.DB
	router      *gin.Engine
	authSvc     *service.AuthService
	customerTok string
	adminTok    string
	categoryID  uuid.UUID
	productID   uuid.UUID
	orderID     uuid.UUID
}

func TestE2ESuite(t *testing.T) {
	suite.Run(t, new(E2ESuite))
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup / Teardown
// ─────────────────────────────────────────────────────────────────────────────

func (s *E2ESuite) SetupSuite() {
	gin.SetMode(gin.TestMode)

	dsn := dsn()
	fmt.Println("DSN:", dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(s.T(), err, "connect to test DB")
	s.db = db

	// Auto-migrate test schema
	require.NoError(s.T(), db.AutoMigrate(
		&models.User{},
		&models.Category{},
		&models.Product{},
		&models.Order{},
		&models.OrderItem{},
	))

	// Seed a category (required FK for products)
	cat := models.Category{ID: uuid.New(), Name: "Electronics", Description: "Gadgets"}
	require.NoError(s.T(), db.Create(&cat).Error)
	s.categoryID = cat.ID

	// Seed an admin user
	s.buildRouter()
	s.seedAdmin()
}

func (s *E2ESuite) TearDownSuite() {
	// Clean up in FK order
	s.db.Exec("DELETE FROM order_items")
	s.db.Exec("DELETE FROM orders")
	s.db.Exec("DELETE FROM products")
	s.db.Exec("DELETE FROM categories")
	s.db.Exec("DELETE FROM users")

	sqlDB, _ := s.db.DB()
	_ = sqlDB.Close()
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func dsn() string {
	host := getEnv("TEST_DB_HOST", "localhost")
	port := getEnv("TEST_DB_PORT", "5434")
	name := getEnv("TEST_DB_NAME", "orders_db")
	user := getEnv("TEST_DB_USER", "postgres")
	pass := getEnv("TEST_DB_PASSWORD", "secret")
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		host, port, user, pass, name)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (s *E2ESuite) buildRouter() {
	jwtSecret := "test-secret-key"

	userRepo := repoImpl.NewUserRepository(s.db)
	productRepo := repoImpl.NewProductRepository(s.db)
	orderRepo := repoImpl.NewOrderRepository(s.db)

	s.authSvc = service.NewAuthService(userRepo, jwtSecret)
	productSvc := service.NewProductService(productRepo)
	orderSvc := service.NewOrderService(orderRepo, productRepo, s.db)

	authH := handler.NewAuthHandler(s.authSvc)
	productH := handler.NewProductHandler(productSvc)
	orderH := handler.NewOrderHandler(orderSvc)

	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")

	// Auth
	v1.POST("/auth/register", authH.Register)
	v1.POST("/auth/login", authH.Login)

	// Products — public
	v1.GET("/products", productH.ListProducts)
	v1.GET("/products/:id", productH.GetProduct)

	// Products — admin
	adminP := v1.Group("/products")
	adminP.Use(middleware.AuthMiddleware(s.authSvc), middleware.AdminMiddleware())
	adminP.POST("", productH.CreateProduct)
	adminP.PUT("/:id", productH.UpdateProduct)
	adminP.DELETE("/:id", productH.DeleteProduct)

	// Orders — customer
	ord := v1.Group("/orders")
	ord.Use(middleware.AuthMiddleware(s.authSvc))
	ord.POST("", orderH.CreateOrder)
	ord.GET("", orderH.ListOrders)
	ord.GET("/:id", orderH.GetOrder)
	ord.DELETE("/:id", orderH.CancelOrder)

	// Orders — admin status
	adminO := ord.Group("")
	adminO.Use(middleware.AdminMiddleware())
	adminO.PATCH("/:id/status", orderH.UpdateOrderStatus)

	// Admin
	adm := v1.Group("/admin")
	adm.Use(middleware.AuthMiddleware(s.authSvc), middleware.AdminMiddleware())
	adm.GET("/orders", orderH.AdminListOrders)

	s.router = r
}

func (s *E2ESuite) seedAdmin() {
	ctx := context.Background()
	out, err := s.authSvc.Register(ctx, service.RegisterInput{
		Name:     "Admin User",
		Email:    fmt.Sprintf("admin-%d@test.com", time.Now().UnixNano()),
		Password: "admin1234",
	})
	require.NoError(s.T(), err)

	// Promote to admin directly in DB
	require.NoError(s.T(), s.db.Model(&models.User{}).
		Where("id = ?", out.User.ID).
		Update("role", models.RoleAdmin).Error)

	// Re-login to get admin token
	loginOut, err := s.authSvc.Login(ctx, service.LoginInput{
		Email:    out.User.Email,
		Password: "admin1234",
	})
	require.NoError(s.T(), err)
	s.adminTok = loginOut.Token
}

// do sends an HTTP request and returns the response recorder.
func (s *E2ESuite) do(method, path, token string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		require.NoError(s.T(), json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

func (s *E2ESuite) decode(w *httptest.ResponseRecorder, dst interface{}) {
	require.NoError(s.T(), json.NewDecoder(w.Body).Decode(dst))
}

// ─────────────────────────────────────────────────────────────────────────────
// 1. Health check
// ─────────────────────────────────────────────────────────────────────────────

func (s *E2ESuite) Test01_HealthCheck() {
	w := s.do(http.MethodGet, "/health", "", nil)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]string
	s.decode(w, &resp)
	assert.Equal(s.T(), "ok", resp["status"])
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. Auth — Register
// ─────────────────────────────────────────────────────────────────────────────

func (s *E2ESuite) Test02_Register_Success() {
	w := s.do(http.MethodPost, "/api/v1/auth/register", "", map[string]interface{}{
		"name":     "Ivan Petrov",
		"email":    fmt.Sprintf("ivan-%d@test.com", time.Now().UnixNano()),
		"password": "secret123",
	})
	assert.Equal(s.T(), http.StatusCreated, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	assert.NotEmpty(s.T(), resp["token"])
	assert.NotNil(s.T(), resp["user"])

	// Save token for subsequent tests
	s.customerTok = resp["token"].(string)
}

func (s *E2ESuite) Test03_Register_DuplicateEmail() {
	email := fmt.Sprintf("dup-%d@test.com", time.Now().UnixNano())
	body := map[string]interface{}{"name": "A", "email": email, "password": "pass123"}

	w1 := s.do(http.MethodPost, "/api/v1/auth/register", "", body)
	assert.Equal(s.T(), http.StatusCreated, w1.Code)

	w2 := s.do(http.MethodPost, "/api/v1/auth/register", "", body)
	assert.Equal(s.T(), http.StatusConflict, w2.Code)
}

func (s *E2ESuite) Test04_Register_InvalidPayload() {
	w := s.do(http.MethodPost, "/api/v1/auth/register", "", map[string]interface{}{
		"name": "No Email",
	})
	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// 3. Auth — Login
// ─────────────────────────────────────────────────────────────────────────────

func (s *E2ESuite) Test05_Login_Success() {
	email := fmt.Sprintf("login-%d@test.com", time.Now().UnixNano())

	// Register first
	s.do(http.MethodPost, "/api/v1/auth/register", "", map[string]interface{}{
		"name": "Login User", "email": email, "password": "pass1234",
	})

	w := s.do(http.MethodPost, "/api/v1/auth/login", "", map[string]interface{}{
		"email": email, "password": "pass1234",
	})
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	assert.NotEmpty(s.T(), resp["token"])
}

func (s *E2ESuite) Test06_Login_WrongPassword() {
	w := s.do(http.MethodPost, "/api/v1/auth/login", "", map[string]interface{}{
		"email": "nobody@test.com", "password": "wrong",
	})
	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// 4. Products — CRUD
// ─────────────────────────────────────────────────────────────────────────────

func (s *E2ESuite) Test07_CreateProduct_AsAdmin() {
	w := s.do(http.MethodPost, "/api/v1/products", s.adminTok, map[string]interface{}{
		"name":        "Laptop Pro",
		"description": "Powerful laptop",
		"price":       1299.99,
		"stock":       50,
		"category_id": s.categoryID,
	})
	require.Equal(s.T(), http.StatusCreated, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	assert.Equal(s.T(), "Laptop Pro", resp["name"])
	assert.Equal(s.T(), 1299.99, resp["price"])

	id, err := uuid.Parse(resp["id"].(string))
	require.NoError(s.T(), err)
	s.productID = id
}

func (s *E2ESuite) Test08_CreateProduct_AsCustomer_Forbidden() {
	w := s.do(http.MethodPost, "/api/v1/products", s.customerTok, map[string]interface{}{
		"name": "Hack", "price": 1.0, "stock": 1, "category_id": s.categoryID,
	})
	assert.Equal(s.T(), http.StatusForbidden, w.Code)
}

func (s *E2ESuite) Test09_CreateProduct_Unauthorized() {
	w := s.do(http.MethodPost, "/api/v1/products", "", map[string]interface{}{
		"name": "Hack", "price": 1.0, "stock": 1, "category_id": s.categoryID,
	})
	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

func (s *E2ESuite) Test10_ListProducts_Public() {
	w := s.do(http.MethodGet, "/api/v1/products", "", nil)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	assert.NotNil(s.T(), resp["items"])
	assert.NotNil(s.T(), resp["total"])
}

func (s *E2ESuite) Test11_ListProducts_WithFilters() {
	w := s.do(http.MethodGet,
		fmt.Sprintf("/api/v1/products?search=Laptop&min_price=100&max_price=2000&category_id=%s", s.categoryID),
		"", nil)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	items := resp["items"].([]interface{})
	assert.GreaterOrEqual(s.T(), len(items), 1)
}

func (s *E2ESuite) Test12_GetProduct_Public() {
	w := s.do(http.MethodGet, "/api/v1/products/"+s.productID.String(), "", nil)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	assert.Equal(s.T(), "Laptop Pro", resp["name"])
}

func (s *E2ESuite) Test13_GetProduct_NotFound() {
	w := s.do(http.MethodGet, "/api/v1/products/"+uuid.New().String(), "", nil)
	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *E2ESuite) Test14_UpdateProduct_AsAdmin() {
	w := s.do(http.MethodPut, "/api/v1/products/"+s.productID.String(), s.adminTok,
		map[string]interface{}{
			"name":        "Laptop Pro Max",
			"description": "Even more powerful",
			"price":       1499.99,
			"stock":       45,
			"category_id": s.categoryID,
		})
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	assert.Equal(s.T(), "Laptop Pro Max", resp["name"])
	assert.Equal(s.T(), 1499.99, resp["price"])
}

func (s *E2ESuite) Test15_UpdateProduct_NotFound() {
	w := s.do(http.MethodPut, "/api/v1/products/"+uuid.New().String(), s.adminTok,
		map[string]interface{}{
			"name": "Ghost", "price": 1.0, "stock": 1, "category_id": s.categoryID,
		})
	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// 5. Orders — CRUD
// ─────────────────────────────────────────────────────────────────────────────

func (s *E2ESuite) Test16_CreateOrder_Success() {
	w := s.do(http.MethodPost, "/api/v1/orders", s.customerTok, map[string]interface{}{
		"address": "ул. Ленина, д. 1, Москва",
		"items": []map[string]interface{}{
			{"product_id": s.productID, "quantity": 2},
		},
	})
	require.Equal(s.T(), http.StatusCreated, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	assert.Equal(s.T(), "new", resp["status"])
	assert.NotNil(s.T(), resp["items"])

	id, err := uuid.Parse(resp["id"].(string))
	require.NoError(s.T(), err)
	s.orderID = id

	// Verify total: 2 × 1499.99 = 2999.98
	assert.InDelta(s.T(), 2999.98, resp["total_amount"], 0.01)
}

func (s *E2ESuite) Test17_CreateOrder_InsufficientStock() {
	w := s.do(http.MethodPost, "/api/v1/orders", s.customerTok, map[string]interface{}{
		"address": "Somewhere",
		"items": []map[string]interface{}{
			{"product_id": s.productID, "quantity": 99999},
		},
	})
	assert.Equal(s.T(), http.StatusUnprocessableEntity, w.Code)
}

func (s *E2ESuite) Test18_CreateOrder_Unauthorized() {
	w := s.do(http.MethodPost, "/api/v1/orders", "", map[string]interface{}{
		"address": "X",
		"items":   []map[string]interface{}{{"product_id": s.productID, "quantity": 1}},
	})
	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

func (s *E2ESuite) Test19_CreateOrder_NonExistentProduct() {
	w := s.do(http.MethodPost, "/api/v1/orders", s.customerTok, map[string]interface{}{
		"address": "Somewhere",
		"items": []map[string]interface{}{
			{"product_id": uuid.New().String(), "quantity": 1},
		},
	})
	assert.Equal(s.T(), http.StatusInternalServerError, w.Code)
}

func (s *E2ESuite) Test20_ListOrders_CustomerSeesOwn() {
	w := s.do(http.MethodGet, "/api/v1/orders", s.customerTok, nil)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	items := resp["items"].([]interface{})
	assert.GreaterOrEqual(s.T(), len(items), 1)
}

func (s *E2ESuite) Test21_ListOrders_AdminSeesAll() {
	w := s.do(http.MethodGet, "/api/v1/admin/orders", s.adminTok, nil)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	assert.NotNil(s.T(), resp["items"])
}

func (s *E2ESuite) Test22_GetOrder_Success() {
	w := s.do(http.MethodGet, "/api/v1/orders/"+s.orderID.String(), s.customerTok, nil)
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	assert.Equal(s.T(), s.orderID.String(), resp["id"])
}

func (s *E2ESuite) Test23_GetOrder_NotFound() {
	w := s.do(http.MethodGet, "/api/v1/orders/"+uuid.New().String(), s.customerTok, nil)
	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *E2ESuite) Test24_GetOrder_ForbiddenForOtherUser() {
	// Register another customer
	email := fmt.Sprintf("other-%d@test.com", time.Now().UnixNano())
	w := s.do(http.MethodPost, "/api/v1/auth/register", "", map[string]interface{}{
		"name": "Other", "email": email, "password": "pass1234",
	})
	require.Equal(s.T(), http.StatusCreated, w.Code)
	var reg map[string]interface{}
	s.decode(w, &reg)
	otherTok := reg["token"].(string)

	// Try to read first customer's order
	w2 := s.do(http.MethodGet, "/api/v1/orders/"+s.orderID.String(), otherTok, nil)
	assert.Equal(s.T(), http.StatusForbidden, w2.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// 6. Order status transitions (admin)
// ─────────────────────────────────────────────────────────────────────────────

func (s *E2ESuite) Test25_UpdateStatus_NewToConfirmed() {
	w := s.do(http.MethodPatch, "/api/v1/orders/"+s.orderID.String()+"/status", s.adminTok,
		map[string]interface{}{"status": "confirmed"})
	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	s.decode(w, &resp)
	assert.Equal(s.T(), "confirmed", resp["status"])
}

func (s *E2ESuite) Test26_UpdateStatus_InvalidTransition() {
	// confirmed → shipped is not allowed (must go through paid → processing first)
	w := s.do(http.MethodPatch, "/api/v1/orders/"+s.orderID.String()+"/status", s.adminTok,
		map[string]interface{}{"status": "shipped"})
	assert.Equal(s.T(), http.StatusUnprocessableEntity, w.Code)
}

func (s *E2ESuite) Test27_UpdateStatus_ConfirmedToPaid() {
	w := s.do(http.MethodPatch, "/api/v1/orders/"+s.orderID.String()+"/status", s.adminTok,
		map[string]interface{}{"status": "paid"})
	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *E2ESuite) Test28_UpdateStatus_PaidToProcessing() {
	w := s.do(http.MethodPatch, "/api/v1/orders/"+s.orderID.String()+"/status", s.adminTok,
		map[string]interface{}{"status": "processing"})
	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *E2ESuite) Test29_UpdateStatus_ProcessingToShipped() {
	w := s.do(http.MethodPatch, "/api/v1/orders/"+s.orderID.String()+"/status", s.adminTok,
		map[string]interface{}{"status": "shipped"})
	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *E2ESuite) Test30_UpdateStatus_ShippedToDelivered() {
	w := s.do(http.MethodPatch, "/api/v1/orders/"+s.orderID.String()+"/status", s.adminTok,
		map[string]interface{}{"status": "delivered"})
	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *E2ESuite) Test31_CancelDeliveredOrder_Forbidden() {
	// Delivered order cannot be cancelled
	w := s.do(http.MethodDelete, "/api/v1/orders/"+s.orderID.String(), s.customerTok, nil)
	assert.Equal(s.T(), http.StatusUnprocessableEntity, w.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// 7. Cancel order — stock is restored
// ─────────────────────────────────────────────────────────────────────────────

func (s *E2ESuite) Test32_CancelOrder_StockRestored() {
	// Get stock before
	var productBefore models.Product
	require.NoError(s.T(), s.db.First(&productBefore, "id = ?", s.productID).Error)
	stockBefore := productBefore.Stock

	// Create a new order
	w := s.do(http.MethodPost, "/api/v1/orders", s.customerTok, map[string]interface{}{
		"address": "Cancel Test Address",
		"items":   []map[string]interface{}{{"product_id": s.productID, "quantity": 3}},
	})
	require.Equal(s.T(), http.StatusCreated, w.Code)
	var created map[string]interface{}
	s.decode(w, &created)
	newOrderID := created["id"].(string)

	// Stock should have decreased by 3
	var productAfterOrder models.Product
	require.NoError(s.T(), s.db.First(&productAfterOrder, "id = ?", s.productID).Error)
	assert.Equal(s.T(), stockBefore-3, productAfterOrder.Stock)

	// Cancel the order
	wc := s.do(http.MethodDelete, "/api/v1/orders/"+newOrderID, s.customerTok, nil)
	assert.Equal(s.T(), http.StatusOK, wc.Code)

	var cancelled map[string]interface{}
	json.NewDecoder(wc.Body).Decode(&cancelled)
	assert.Equal(s.T(), "cancelled", cancelled["status"])

	// Stock should be restored
	var productAfterCancel models.Product
	require.NoError(s.T(), s.db.First(&productAfterCancel, "id = ?", s.productID).Error)
	assert.Equal(s.T(), stockBefore, productAfterCancel.Stock)
}

// ─────────────────────────────────────────────────────────────────────────────
// 8. Delete product (admin)
// ─────────────────────────────────────────────────────────────────────────────

func (s *E2ESuite) Test33_DeleteProduct_AsAdmin() {
	// Create a fresh product to delete
	w := s.do(http.MethodPost, "/api/v1/products", s.adminTok, map[string]interface{}{
		"name": "ToDelete", "price": 9.99, "stock": 5, "category_id": s.categoryID,
	})
	require.Equal(s.T(), http.StatusCreated, w.Code)
	var p map[string]interface{}
	s.decode(w, &p)
	pid := p["id"].(string)

	wd := s.do(http.MethodDelete, "/api/v1/products/"+pid, s.adminTok, nil)
	assert.Equal(s.T(), http.StatusNoContent, wd.Code)

	// Verify it's gone
	wg := s.do(http.MethodGet, "/api/v1/products/"+pid, "", nil)
	assert.Equal(s.T(), http.StatusNotFound, wg.Code)
}

func (s *E2ESuite) Test34_DeleteProduct_NotFound() {
	w := s.do(http.MethodDelete, "/api/v1/products/"+uuid.New().String(), s.adminTok, nil)
	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

func (s *E2ESuite) Test35_DeleteProduct_AsCustomer_Forbidden() {
	w := s.do(http.MethodDelete, "/api/v1/products/"+s.productID.String(), s.customerTok, nil)
	assert.Equal(s.T(), http.StatusForbidden, w.Code)
}
