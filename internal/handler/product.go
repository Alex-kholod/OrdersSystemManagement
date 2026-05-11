package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"order-service/internal/service"
)

type ProductHandler struct {
	productSvc *service.ProductService
}

func NewProductHandler(productSvc *service.ProductService) *ProductHandler {
	return &ProductHandler{productSvc: productSvc}
}

type createProductRequest struct {
	Name        string  `json:"name"        binding:"required"`
	Description string  `json:"description"`
	Price       float64 `json:"price"       binding:"required,gt=0"`
	Stock       int     `json:"stock"       binding:"min=0"`
}

type updateProductRequest struct {
	Name        string    `json:"name"        binding:"required"`
	Description string    `json:"description"`
	Price       float64   `json:"price"       binding:"required,gt=0"`
	Stock       int       `json:"stock"       binding:"min=0"`
	CategoryID  uuid.UUID `json:"category_id" binding:"required"`
}

type listProductsResponse struct {
	Items interface{} `json:"items"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Limit int         `json:"limit"`
}

// ListProducts godoc
// @Summary      List products
// @Description  Returns a paginated, filtered list of products
// @Tags         products
// @Produce      json
// @Param        page        query int    false "Page number"  default(1)
// @Param        limit       query int    false "Page size"    default(20)
// @Param        category_id query string false "Filter by category UUID"
// @Param        min_price   query number false "Minimum price"
// @Param        max_price   query number false "Maximum price"
// @Param        search      query string false "Search term"
// @Success      200 {object} listProductsResponse
// @Router       /products [get]
func (h *ProductHandler) ListProducts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	input := service.ListProductsInput{
		Search: c.Query("search"),
		Page:   page,
		Limit:  limit,
	}

	if catStr := c.Query("category_id"); catStr != "" {
		catID, err := uuid.Parse(catStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category_id"})
			return
		}
		input.CategoryID = &catID
	}
	if minStr := c.Query("min_price"); minStr != "" {
		v, err := strconv.ParseFloat(minStr, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid min_price"})
			return
		}
		input.MinPrice = &v
	}
	if maxStr := c.Query("max_price"); maxStr != "" {
		v, err := strconv.ParseFloat(maxStr, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid max_price"})
			return
		}
		input.MaxPrice = &v
	}

	out, err := h.productSvc.ListProducts(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, listProductsResponse{
		Items: out.Items,
		Total: out.Total,
		Page:  out.Page,
		Limit: out.Limit,
	})
}

// GetProduct godoc
// @Summary      Get a product by ID
// @Description  Returns a single product
// @Tags         products
// @Produce      json
// @Param        id path string true "Product UUID"
// @Success      200 {object} models.Product
// @Failure      404 {object} errorResponse
// @Router       /products/{id} [get]
func (h *ProductHandler) GetProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product id"})
		return
	}

	product, err := h.productSvc.GetProduct(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, product)
}

// CreateProduct godoc
// @Summary      Create a product
// @Description  Creates a new product (admin only)
// @Tags         products
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body createProductRequest true "Product payload"
// @Success      201 {object} models.Product
// @Failure      400 {object} errorResponse
// @Failure      401 {object} errorResponse
// @Failure      403 {object} errorResponse
// @Router       /products [post]
func (h *ProductHandler) CreateProduct(c *gin.Context) {
	var req createProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	product, err := h.productSvc.CreateProduct(c.Request.Context(), service.CreateProductInput{
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		Stock:       req.Stock,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusCreated, product)
}

// UpdateProduct godoc
// @Summary      Update a product
// @Description  Replaces a product's fields (admin only)
// @Tags         products
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string              true "Product UUID"
// @Param        body body updateProductRequest true "Product payload"
// @Success      200 {object} models.Product
// @Failure      400 {object} errorResponse
// @Failure      401 {object} errorResponse
// @Failure      403 {object} errorResponse
// @Failure      404 {object} errorResponse
// @Router       /products/{id} [put]
func (h *ProductHandler) UpdateProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product id"})
		return
	}

	var req updateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	product, err := h.productSvc.UpdateProduct(c.Request.Context(), id, service.UpdateProductInput{
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		Stock:       req.Stock,
		CategoryID:  req.CategoryID,
	})
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, product)
}

// DeleteProduct godoc
// @Summary      Delete a product
// @Description  Removes a product by ID (admin only)
// @Tags         products
// @Security     BearerAuth
// @Param        id path string true "Product UUID"
// @Success      204
// @Failure      401 {object} errorResponse
// @Failure      403 {object} errorResponse
// @Failure      404 {object} errorResponse
// @Router       /products/{id} [delete]
func (h *ProductHandler) DeleteProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product id"})
		return
	}

	if err := h.productSvc.DeleteProduct(c.Request.Context(), id); err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.Status(http.StatusNoContent)
}
