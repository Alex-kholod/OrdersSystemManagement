package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"order-service/internal/domain/models"
	domainrepo "order-service/internal/domain/repository"
)

var ErrProductNotFound = errors.New("product not found")

type ProductService struct {
	products domainrepo.ProductRepository
}

func NewProductService(products domainrepo.ProductRepository) *ProductService {
	return &ProductService{products: products}
}

type CreateProductInput struct {
	Name        string
	Description string
	Price       float64
	Stock       int
	CategoryID  uuid.UUID
}

type UpdateProductInput struct {
	Name        string
	Description string
	Price       float64
	Stock       int
	CategoryID  uuid.UUID
}

type ListProductsInput struct {
	CategoryID *uuid.UUID
	MinPrice   *float64
	MaxPrice   *float64
	Search     string
	Page       int
	Limit      int
}

type ListProductsOutput struct {
	Items []*models.Product
	Total int64
	Page  int
	Limit int
}

func (s *ProductService) CreateProduct(ctx context.Context, input CreateProductInput) (*models.Product, error) {
	product := &models.Product{
		ID:          uuid.New(),
		Name:        input.Name,
		Description: input.Description,
		Price:       input.Price,
		Stock:       input.Stock,
		CategoryID:  input.CategoryID,
	}
	if err := s.products.Create(ctx, product); err != nil {
		return nil, fmt.Errorf("create product: %w", err)
	}
	return product, nil
}

func (s *ProductService) GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	product, err := s.products.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if product == nil {
		return nil, ErrProductNotFound
	}
	return product, nil
}

func (s *ProductService) ListProducts(ctx context.Context, input ListProductsInput) (*ListProductsOutput, error) {
	filter := domainrepo.ProductFilter{
		CategoryID: input.CategoryID,
		MinPrice:   input.MinPrice,
		MaxPrice:   input.MaxPrice,
		Search:     input.Search,
		Page:       input.Page,
		Limit:      input.Limit,
	}
	items, total, err := s.products.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	return &ListProductsOutput{
		Items: items,
		Total: total,
		Page:  input.Page,
		Limit: input.Limit,
	}, nil
}

func (s *ProductService) UpdateProduct(ctx context.Context, id uuid.UUID, input UpdateProductInput) (*models.Product, error) {
	product, err := s.products.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find product: %w", err)
	}
	if product == nil {
		return nil, ErrProductNotFound
	}

	product.Name = input.Name
	product.Description = input.Description
	product.Price = input.Price
	product.Stock = input.Stock
	product.CategoryID = input.CategoryID

	if err := s.products.Update(ctx, product); err != nil {
		return nil, fmt.Errorf("update product: %w", err)
	}
	return product, nil
}

func (s *ProductService) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	product, err := s.products.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("find product: %w", err)
	}
	if product == nil {
		return ErrProductNotFound
	}
	if err := s.products.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	return nil
}
