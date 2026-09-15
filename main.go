package main

import (
	"log"
	"os"
	"strconv"

	"ebike-battery-backend/internal/dsn"
	"ebike-battery-backend/internal/handler"
	"ebike-battery-backend/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	mediaBaseURL := os.Getenv("MINIO_PUBLIC_URL")
	if mediaBaseURL == "" {
		mediaBaseURL = "http://localhost:9000/ebike-motor-media"
	}

	currentRiderID, err := strconv.ParseUint(os.Getenv("CURRENT_RIDER_ID"), 10, 32)
	if err != nil || currentRiderID == 0 {
		currentRiderID = 1
	}

	motorModeRepository, err := repository.NewMotorModeRepository(dsn.FromEnv())
	if err != nil {
		log.Fatalf("не удалось подключиться к базе данных: %v", err)
	}
	motorModeHandler := handler.NewMotorModeHandler(motorModeRepository, mediaBaseURL, uint(currentRiderID))

	router := gin.Default()
	router.LoadHTMLGlob("templates/*.html")
	router.Static("/static", "./static")

	handler.RegisterRoutes(router, motorModeHandler)

	log.Println("сервер режимов работы мотора запущен на http://localhost:8080/motor-modes/feed")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
