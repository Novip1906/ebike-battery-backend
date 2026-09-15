package main

import (
	"log"

	"ebike-battery-backend/internal/ds"
	"ebike-battery-backend/internal/dsn"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	_ = godotenv.Load()

	db, err := gorm.Open(postgres.Open(dsn.FromEnv()), &gorm.Config{})
	if err != nil {
		log.Fatalf("не удалось подключиться к базе данных: %v", err)
	}

	if err := db.AutoMigrate(&ds.Rider{}, &ds.MotorMode{}, &ds.MotorModeLike{}); err != nil {
		log.Fatalf("миграция не выполнена: %v", err)
	}

	log.Println("таблицы riders, motor_modes, motor_mode_likes созданы")
}
