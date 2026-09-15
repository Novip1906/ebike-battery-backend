package ds

import "time"

type MotorModeLike struct {
	ID          uint      `gorm:"primaryKey"`
	RiderID     uint      `gorm:"not null;uniqueIndex:rider_likes_mode_once"`
	MotorModeID uint      `gorm:"not null;uniqueIndex:rider_likes_mode_once"`
	LikedAt     time.Time `gorm:"not null"`
	Rider       Rider     `gorm:"foreignKey:RiderID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	MotorMode   MotorMode `gorm:"foreignKey:MotorModeID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
}

func (MotorModeLike) TableName() string {
	return "motor_mode_likes"
}
