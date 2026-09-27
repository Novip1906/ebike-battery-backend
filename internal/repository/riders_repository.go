package repository

import (
	"errors"

	"ebike-battery-backend/internal/ds"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var (
	ErrRiderNotFound = errors.New("велосипедист не найден")
	ErrLoginTaken    = errors.New("логин уже занят")
)

type RiderRepository struct {
	db *gorm.DB
}

func NewRiderRepository(db *gorm.DB) *RiderRepository {
	return &RiderRepository{db: db}
}

func (r *RiderRepository) CreateRider(login, passwordHash string) (ds.Rider, error) {
	rider := ds.Rider{Login: login, PasswordHash: passwordHash}
	err := r.db.Create(&rider).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == "idx_riders_login" {
		return ds.Rider{}, ErrLoginTaken
	}
	return rider, err
}

func (r *RiderRepository) RiderByLogin(login string) (ds.Rider, error) {
	var rider ds.Rider
	err := r.db.Where("login = ?", login).First(&rider).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ds.Rider{}, ErrRiderNotFound
	}
	return rider, err
}
