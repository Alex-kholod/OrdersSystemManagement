// @title           Order Service API
// @version         1.0
// @description     Система управления заказами для интернет-магазина
// @termsOfService  http://swagger.io/terms/

// @contact.name   Холодков А.Д.
// @contact.email  kholodkov.a.d@edu.mirea.ru

// @license.name  MIT

// @host      localhost:8080
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and your JWT token.

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"order-service/docs"
	"order-service/internal/config"
	"order-service/internal/handler"
	"order-service/internal/middleware"
	repoImpl "order-service/internal/repository"
	"order-service/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	gin.SetMode(cfg.GinMode)

	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}

	// if err := db.AutoMigrate(
	// 	&models.User{},
	// 	&models.Category{},
	// 	&models.Product{},
	// 	&models.Order{},
	// 	&models.OrderItem{},
	// ); err != nil {
	// 	log.Fatalf("auto migrate: %v", err)
	// }

	userRepo := repoImpl.NewUserRepository(db)
	productRepo := repoImpl.NewProductRepository(db)
	orderRepo := repoImpl.NewOrderRepository(db)

	authSvc := service.NewAuthService(userRepo, cfg.JWTSecret)
	productSvc := service.NewProductService(productRepo)
	orderSvc := service.NewOrderService(orderRepo, productRepo, db)

	authHandler := handler.NewAuthHandler(authSvc)
	productHandler := handler.NewProductHandler(productSvc)
	orderHandler := handler.NewOrderHandler(orderSvc)

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Swagger
	docs.SwaggerInfo.BasePath = "/api/v1"
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	v1 := r.Group("/api/v1")
	{
		// Auth
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
		}

		// Products
		products := v1.Group("/products")
		{
			// Public
			products.GET("", productHandler.ListProducts)
			products.GET("/:id", productHandler.GetProduct)

			// Admin
			adminProducts := products.Group("")
			adminProducts.Use(middleware.AuthMiddleware(authSvc), middleware.AdminMiddleware())
			{
				adminProducts.POST("", productHandler.CreateProduct)
				adminProducts.PUT("/:id", productHandler.UpdateProduct)
				adminProducts.DELETE("/:id", productHandler.DeleteProduct)
			}
		}

		// Orders
		orders := v1.Group("/orders")
		orders.Use(middleware.AuthMiddleware(authSvc))
		{
			orders.POST("", orderHandler.CreateOrder)
			orders.GET("", orderHandler.ListOrders)
			orders.GET("/:id", orderHandler.GetOrder)
			orders.DELETE("/:id", orderHandler.CancelOrder)

			// Admin only
			adminOrders := orders.Group("")
			adminOrders.Use(middleware.AdminMiddleware())
			{
				adminOrders.PATCH("/:id/status", orderHandler.UpdateOrderStatus)
			}
		}

		// Admin namespace
		admin := v1.Group("/admin")
		admin.Use(middleware.AuthMiddleware(authSvc), middleware.AdminMiddleware())
		{
			admin.GET("/orders", orderHandler.AdminListOrders)
		}
	}

	addr := fmt.Sprintf(":%d", cfg.ServerPort)
	log.Printf("Starting server on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
