package dsn

import (
	"fmt"
	"os"
)

func FromEnv() string {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, os.Getenv("DB_USER"), os.Getenv("DB_PASS"), os.Getenv("DB_NAME"),
	)
}
