package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"order-service/internal/domain/models"
	"order-service/internal/middleware"
	"order-service/internal/service"
)

type OrderHandler struct {
	orderSvc *service.OrderService
}

func NewOrderHandler(orderSvc *service.OrderService) *OrderHandler {
	return &OrderHandler{orderSvc: orderSvc}
}

type createOrderItemRequest struct {
	ProductID uuid.UUID `json:"product_id" binding:"required"`
	Quantity  int       `json:"quantity"   binding:"required,min=1"`
}

type createOrderRequest struct {
	Address string                   `json:"address" binding:"required"`
	Items   []createOrderItemRequest `json:"items"   binding:"required,min=1,dive"`
}

type updateStatusRequest struct {
	Status models.OrderStatus `json:"status" binding:"required"`
}

type listOrdersResponse struct {
	Items interface{} `json:"items"`
	Total int64       `json:"total"`
}

// CreateOrder godoc
// @Summary      Place a new order
// @Description  Creates an order for the authenticated customer
// @Tags         orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body createOrderRequest true "Order payload"
// @Success      201 {object} models.Order
// @Failure      400 {object} errorResponse
// @Failure      401 {object} errorResponse
// @Failure      422 {object} errorResponse
// @Router       /orders [post]
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	userID := mustUserID(c)

	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	items := make([]service.CreateOrderItemInput, len(req.Items))
	for i, it := range req.Items {
		items[i] = service.CreateOrderItemInput{
			ProductID: it.ProductID,
			Quantity:  it.Quantity,
		}
	}

	order, err := h.orderSvc.CreateOrder(c.Request.Context(), service.CreateOrderInput{
		UserID:  userID,
		Address: req.Address,
		Items:   items,
	})
	if err != nil {
		if errors.Is(err, service.ErrInsufficientStock) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, order)
}

// ListOrders godoc
// @Summary      List orders
// @Description  Customers see their own orders; admins see all orders
// @Tags         orders
// @Produce      json
// @Security     BearerAuth
// @Param        page  query int false "Page number" default(1)
// @Param        limit query int false "Page size"   default(20)
// @Success      200 {object} listOrdersResponse
// @Failure      401 {object} errorResponse
// @Router       /orders [get]
func (h *OrderHandler) ListOrders(c *gin.Context) {
	userID := mustUserID(c)
	isAdmin := isAdminRole(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	input := service.ListOrdersInput{
		IsAdmin: isAdmin,
		Page:    page,
		Limit:   limit,
	}
	if !isAdmin {
		input.UserID = &userID
	}

	out, err := h.orderSvc.ListOrders(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, listOrdersResponse{Items: out.Items, Total: out.Total})
}

// GetOrder godoc
// @Summary      Get an order by ID
// @Description  Customers can only fetch their own orders
// @Tags         orders
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Order UUID"
// @Success      200 {object} models.Order
// @Failure      401 {object} errorResponse
// @Failure      403 {object} errorResponse
// @Failure      404 {object} errorResponse
// @Router       /orders/{id} [get]
func (h *OrderHandler) GetOrder(c *gin.Context) {
	userID := mustUserID(c)
	isAdmin := isAdminRole(c)

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	order, err := h.orderSvc.GetOrder(c.Request.Context(), orderID, userID, isAdmin)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrOrderForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}

	c.JSON(http.StatusOK, order)
}

// CancelOrder godoc
// @Summary      Cancel an order
// @Description  Customers cancel their own orders; stock is restored automatically
// @Tags         orders
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Order UUID"
// @Success      200 {object} models.Order
// @Failure      401 {object} errorResponse
// @Failure      403 {object} errorResponse
// @Failure      404 {object} errorResponse
// @Failure      422 {object} errorResponse
// @Router       /orders/{id} [delete]
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	userID := mustUserID(c)
	isAdmin := isAdminRole(c)

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	order, err := h.orderSvc.CancelOrder(c.Request.Context(), orderID, userID, isAdmin)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrOrderForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrOrderNotCancellable):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}

	c.JSON(http.StatusOK, order)
}

// UpdateOrderStatus godoc
// @Summary      Update order status
// @Description  Transitions an order to a new status (admin only)
// @Tags         orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string            true "Order UUID"
// @Param        body body updateStatusRequest true "New status"
// @Success      200 {object} models.Order
// @Failure      400 {object} errorResponse
// @Failure      401 {object} errorResponse
// @Failure      403 {object} errorResponse
// @Failure      404 {object} errorResponse
// @Failure      422 {object} errorResponse
// @Router       /orders/{id}/status [patch]
func (h *OrderHandler) UpdateOrderStatus(c *gin.Context) {
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order, err := h.orderSvc.UpdateOrderStatus(c.Request.Context(), orderID, req.Status)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrInvalidStatusTransition):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}

	c.JSON(http.StatusOK, order)
}

// AdminListOrders godoc
// @Summary      List all orders (admin)
// @Description  Returns all orders across all users
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        page  query int false "Page number" default(1)
// @Param        limit query int false "Page size"   default(20)
// @Success      200 {object} listOrdersResponse
// @Failure      401 {object} errorResponse
// @Failure      403 {object} errorResponse
// @Router       /admin/orders [get]
func (h *OrderHandler) AdminListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	out, err := h.orderSvc.ListOrders(c.Request.Context(), service.ListOrdersInput{
		IsAdmin: true,
		Page:    page,
		Limit:   limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, listOrdersResponse{Items: out.Items, Total: out.Total})
}

// mustUserID extracts the authenticated user's UUID from the Gin context.
// It panics if the middleware hasn't set the value (programming error).
func mustUserID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(middleware.ContextKeyUserID)
	id, _ := v.(uuid.UUID)
	return id
}

// isAdminRole returns true when the context role equals "admin".
func isAdminRole(c *gin.Context) bool {
	v, _ := c.Get(middleware.ContextKeyRole)
	role, _ := v.(string)
	return role == "admin"
}
