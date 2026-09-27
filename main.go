package main

import (
	"log"
	"os"

	"ebike-battery-backend/internal/api"
	"ebike-battery-backend/internal/dsn"
	"ebike-battery-backend/internal/handler"
	"ebike-battery-backend/internal/repository"
	"ebike-battery-backend/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

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

	motorModePages := handler.NewMotorModeHandler(motorModeRepository, mediaBaseURL)
	motorModeAPI := api.NewMotorModeHandler(motorModeRepository, mediaStorage, mediaBaseURL)
	riderAPI := api.NewRiderHandler(riderRepository)

	router := gin.Default()
	router.HandleMethodNotAllowed = true
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatal(err)
	}
	router.LoadHTMLGlob("templates/*.html")
	router.Static("/static", "./static")

	handler.RegisterRoutes(router, motorModePages)
	api.RegisterRoutes(router, motorModeAPI, riderAPI)
	router.NoRoute(api.NoRoute(motorModePages.NotFound))
	router.NoMethod(api.NoMethod(motorModePages.NotFound))

	log.Println("сервер режимов работы мотора запущен: страницы http://localhost:8080/motor_modes/feed, API http://localhost:8080/api/motor_modes")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
