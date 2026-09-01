package main

import (
	"log"
	"os"

	"ebike-battery-backend/internal/handler"
	"ebike-battery-backend/internal/repository"

	"github.com/gin-gonic/gin"
)

func main() {
	mediaBaseURL := os.Getenv("MINIO_PUBLIC_URL")
	if mediaBaseURL == "" {
		mediaBaseURL = "http://localhost:9000/ebike-motor-media"
	}

	motorModeRepository := repository.NewMotorModeRepository()
	motorModeHandler := handler.NewMotorModeHandler(motorModeRepository, mediaBaseURL)

	router := gin.Default()
	router.LoadHTMLGlob("templates/*.html")
	router.Static("/static", "./static")

	handler.RegisterRoutes(router, motorModeHandler)

	log.Println("сервер режимов работы мотора запущен на http://localhost:8080/motor-modes/feed")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
