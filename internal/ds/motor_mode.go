package ds

import "time"

type MotorModeStatus string

const (
	StatusDraft     MotorModeStatus = "draft"
	StatusPublished MotorModeStatus = "published"
	StatusDeleted   MotorModeStatus = "deleted"
)

func (s MotorModeStatus) Label() string {
	switch s {
	case StatusDraft:
		return "черновик"
	case StatusPublished:
		return "опубликован"
	case StatusDeleted:
		return "удален"
	}
	return string(s)
}

type MotorMode struct {
	ID                 uint            `gorm:"primaryKey"`
	ModeName           string          `gorm:"type:varchar(50);not null"`
	ShortDescription   string          `gorm:"type:varchar(600);not null;default:''"`
	Status             MotorModeStatus `gorm:"type:varchar(20);not null;default:'draft'"`
	ImageKey           string          `gorm:"type:varchar(120);not null;default:''"`
	VideoKey           string          `gorm:"type:varchar(120);not null;default:''"`
	SupportPercent     int             `gorm:"type:integer;not null;default:0"`
	ConsumptionWhPerKm float64         `gorm:"type:numeric(5,1);not null;default:0"`
	CreatedAt          time.Time       `gorm:"not null"`
	CreatorRiderID     uint            `gorm:"not null;index:one_draft_per_rider,unique,where:status = 'draft'"`
	CreatorRider       Rider           `gorm:"foreignKey:CreatorRiderID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	PublishedAt        *time.Time
}

func (MotorMode) TableName() string {
	return "motor_modes"
}

const BatteryCapacityWh = 800

func (m MotorMode) RangeKm() int {
	if m.ConsumptionWhPerKm <= 0 {
		return 0
	}
	return int(BatteryCapacityWh / m.ConsumptionWhPerKm)
}
