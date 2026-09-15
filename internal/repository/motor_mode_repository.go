package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"ebike-battery-backend/internal/ds"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var ErrMotorModeNotFound = errors.New("режим работы мотора не найден")

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
	return r.db.Where("status = ?", ds.StatusPublished).Order("id")
}

func (r *MotorModeRepository) PublishedModes(maxConsumption float64) ([]ds.MotorMode, error) {
	var motorModes []ds.MotorMode
	err := r.published().Where("consumption_wh_per_km <= ?", maxConsumption).Find(&motorModes).Error
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
	var motorMode ds.MotorMode
	err := r.published().Where("id > ?", afterID).First(&motorMode).Error
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
	return motorMode, err
}

type PublishFields struct {
	ModeName           string
	ShortDescription   string
	SupportPercent     int
	ConsumptionWhPerKm float64
}

func (r *MotorModeRepository) PublishDraft(riderID uint, fields PublishFields) (ds.MotorMode, error) {
	motorMode, err := r.DraftByRider(riderID)
	if err != nil {
		return ds.MotorMode{}, err
	}
	now := time.Now()
	motorMode.ModeName = fields.ModeName
	motorMode.ShortDescription = fields.ShortDescription
	motorMode.SupportPercent = fields.SupportPercent
	motorMode.ConsumptionWhPerKm = fields.ConsumptionWhPerKm
	motorMode.Status = ds.StatusPublished
	motorMode.PublishedAt = &now
	err = r.db.Save(&motorMode).Error
	return motorMode, err
}

func (r *MotorModeRepository) MarkDeletedBySQL(motorModeID uint) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	result, err := sqlDB.Exec(
		"UPDATE motor_modes SET status = $1 WHERE id = $2 AND status <> $1",
		string(ds.StatusDeleted), motorModeID,
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
