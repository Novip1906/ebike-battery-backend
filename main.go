package main

import (
	"context"
	"log"
	"net/http"
	"os"

	_ "ebike-battery-backend/docs"
	"ebike-battery-backend/internal/api"
	"ebike-battery-backend/internal/auth"
	"ebike-battery-backend/internal/blacklist"
	"ebike-battery-backend/internal/dsn"
	"ebike-battery-backend/internal/handler"
	"ebike-battery-backend/internal/repository"
	"ebike-battery-backend/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title Режимы работы мотора электровелосипеда
// @version 1.0
// @description Веб-сервис заявочной системы «Оценка времени разряда аккумулятора электровелосипеда». Чтение списка и ленты режимов доступно гостю, методы создателя режимов — авторизованному велосипедисту.
// @BasePath /api
// @tag.name motor_modes
// @tag.description Режимы работы мотора
// @tag.name riders
// @tag.description Велосипедисты: регистрация, аутентификация, деавторизация
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Введите слово Bearer, пробел и access_token из ответа POST /riders/login
func main() {
	_ = godotenv.Load()

	mediaBaseURL := os.Getenv("MINIO_PUBLIC_URL")
	if mediaBaseURL == "" {
		mediaBaseURL = "http://localhost:9000/ebike-motor-media"
	}

	db, err := repository.Open(dsn.FromEnv())
	if err != nil {
		log.Fatalf("не удалось подключиться к базе данных: %v", err)
	}
	motorModeRepository := repository.NewMotorModeRepository(db)
	riderRepository := repository.NewRiderRepository(db)

	mediaStorage, err := storage.NewMinioStorage(storage.MinioConfigFromEnv())
	if err != nil {
		log.Fatalf("не удалось настроить клиент Minio: %v", err)
	}

	tokens, err := auth.TokenManagerFromEnv()
	if err != nil {
		log.Fatalf("не удалось настроить JWT: %v", err)
	}
	tokenBlacklist, err := blacklist.New(context.Background(), blacklist.ConfigFromEnv())
	if err != nil {
		log.Fatalf("не удалось подключиться к Redis: %v", err)
	}

	motorModePages := handler.NewMotorModeHandler(motorModeRepository, mediaBaseURL)
	motorModeAPI := api.NewMotorModeHandler(motorModeRepository, mediaStorage, mediaBaseURL)
	riderAPI := api.NewRiderHandler(riderRepository, tokens, tokenBlacklist)
	authMiddleware := api.NewAuthMiddleware(tokens, tokenBlacklist)

	router := gin.Default()
	router.HandleMethodNotAllowed = true
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatal(err)
	}
	router.LoadHTMLGlob("templates/*.html")
	router.Static("/static", "./static")

	handler.RegisterRoutes(router, motorModePages)
	api.RegisterRoutes(router, motorModeAPI, riderAPI, authMiddleware)
	router.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/swagger/index.html")
	})
	swaggerUI := ginSwagger.WrapHandler(swaggerFiles.Handler)
	router.GET("/swagger/*any", func(c *gin.Context) {
		if c.Param("any") == "/" {
			c.Redirect(http.StatusFound, "/swagger/index.html")
			return
		}
		swaggerUI(c)
	})
	router.NoRoute(api.NoRoute(motorModePages.NotFound))
	router.NoMethod(api.NoMethod(motorModePages.NotFound))

	log.Println("сервер режимов работы мотора запущен: страницы http://localhost:8080/motor_modes/feed, API http://localhost:8080/api/motor_modes, swagger http://localhost:8080/swagger/index.html")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
