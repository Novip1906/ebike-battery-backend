package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"ebike-battery-backend/internal/ds"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	ErrMotorModeNotFound  = errors.New("режим работы мотора не найден")
	ErrDraftAlreadyExists = errors.New("у велосипедиста уже есть черновик режима")
)

const pgUniqueViolation = "23505"

type MotorModeRepository struct {
	db *gorm.DB
}

func NewMotorModeRepository(dsn string) (*MotorModeRepository, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return &MotorModeRepository{db: db}, nil
}

func (r *MotorModeRepository) published() *gorm.DB {
	return r.db.Where("status = ?", ds.StatusPublished)
}

func (r *MotorModeRepository) PublishedModes(maxConsumption float64) ([]ds.MotorMode, error) {
	var motorModes []ds.MotorMode
	err := r.published().Where("consumption_wh_per_km <= ?", maxConsumption).Order("id").Find(&motorModes).Error
	return motorModes, err
}

func (r *MotorModeRepository) PublishedModeByID(id uint) (ds.MotorMode, error) {
	var motorMode ds.MotorMode
	err := r.published().Where("id = ?", id).First(&motorMode).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ds.MotorMode{}, ErrMotorModeNotFound
	}
	return motorMode, err
}

func (r *MotorModeRepository) FirstPublishedMode() (ds.MotorMode, error) {
	var motorMode ds.MotorMode
	err := r.published().First(&motorMode).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ds.MotorMode{}, ErrMotorModeNotFound
	}
	return motorMode, err
}

func (r *MotorModeRepository) NextPublishedMode(afterID uint) (ds.MotorMode, error) {
	if _, err := r.PublishedModeByID(afterID); err != nil {
		return ds.MotorMode{}, err
	}
	var motorMode ds.MotorMode
	err := r.published().Where("id > ?", afterID).Order("id").First(&motorMode).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.FirstPublishedMode()
	}
	return motorMode, err
}

func (r *MotorModeRepository) ConsumptionBounds() (float64, float64, error) {
	var bounds struct {
		Min sql.NullFloat64
		Max sql.NullFloat64
	}
	err := r.db.Model(&ds.MotorMode{}).
		Select("MIN(consumption_wh_per_km) AS min, MAX(consumption_wh_per_km) AS max").
		Where("status = ? AND consumption_wh_per_km > 0", ds.StatusPublished).
		Scan(&bounds).Error
	if err != nil || !bounds.Max.Valid {
		return 0, 0, err
	}
	return bounds.Min.Float64, bounds.Max.Float64, nil
}

func (r *MotorModeRepository) LikeCount(motorModeID uint) (int, error) {
	var count int64
	err := r.db.Model(&ds.MotorModeLike{}).Where("motor_mode_id = ?", motorModeID).Count(&count).Error
	return int(count), err
}

func (r *MotorModeRepository) LikeCounts() (map[uint]int, error) {
	var rows []struct {
		MotorModeID uint
		LikeCount   int
	}
	err := r.db.Model(&ds.MotorModeLike{}).
		Select("motor_mode_id, COUNT(*) AS like_count").
		Group("motor_mode_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[uint]int, len(rows))
	for _, row := range rows {
		counts[row.MotorModeID] = row.LikeCount
	}
	return counts, nil
}

func (r *MotorModeRepository) DraftByRider(riderID uint) (ds.MotorMode, error) {
	var motorMode ds.MotorMode
	err := r.db.Where("status = ? AND creator_rider_id = ?", ds.StatusDraft, riderID).First(&motorMode).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ds.MotorMode{}, ErrMotorModeNotFound
	}
	return motorMode, err
}

func (r *MotorModeRepository) CreateDraft(riderID uint, modeName string) (ds.MotorMode, error) {
	motorMode := ds.MotorMode{
		ModeName:       modeName,
		Status:         ds.StatusDraft,
		CreatedAt:      time.Now(),
		CreatorRiderID: riderID,
	}
	err := r.db.Create(&motorMode).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return ds.MotorMode{}, ErrDraftAlreadyExists
	}
	return motorMode, err
}

type PublishFields struct {
	ModeName           string
	ShortDescription   string
	SupportPercent     int
	ConsumptionWhPerKm float64
}

func (r *MotorModeRepository) PublishDraft(draftID uint, fields PublishFields) error {
	result := r.db.Model(&ds.MotorMode{}).
		Where("id = ? AND status = ?", draftID, ds.StatusDraft).
		Updates(map[string]any{
			"mode_name":             fields.ModeName,
			"short_description":     fields.ShortDescription,
			"support_percent":       fields.SupportPercent,
			"consumption_wh_per_km": fields.ConsumptionWhPerKm,
			"status":                ds.StatusPublished,
			"published_at":          time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrMotorModeNotFound
	}
	return nil
}

func (r *MotorModeRepository) MarkDeletedBySQL(motorModeID uint) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	result, err := sqlDB.Exec(
		"UPDATE motor_modes SET status = $1 WHERE id = $2 AND status = $3",
		string(ds.StatusDeleted), motorModeID, string(ds.StatusPublished),
	)
	if err != nil {
		return fmt.Errorf("удаление режима %d: %w", motorModeID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrMotorModeNotFound
	}
	return nil
}
