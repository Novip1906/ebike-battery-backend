package repository

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const pgUniqueViolation = "23505"

func Open(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}
