package serializer

import "time"

type MotorModeCard struct {
	ID                    uint     `json:"id"`
	ModeName              string   `json:"mode_name"`
	ImageURL              string   `json:"image_url"`
	SupportPercent        *int     `json:"support_percent"`
	ConsumptionWhPerKm    *float64 `json:"consumption_wh_per_km"`
	LikeCount             int      `json:"like_count"`
	CreatedByCurrentRider int      `json:"created_by_current_rider"`
}

type MotorModeList struct {
	MotorModes            []MotorModeCard `json:"motor_modes"`
	MaxConsumptionWhPerKm float64         `json:"max_consumption_wh_per_km"`
	ConsumptionMin        float64         `json:"consumption_min"`
	ConsumptionMax        float64         `json:"consumption_max"`
}

type MotorModeDetail struct {
	ID                    uint       `json:"id"`
	ModeName              string     `json:"mode_name"`
	Status                string     `json:"status"`
	LikedByCurrentRider   int        `json:"liked_by_current_rider"`
	LikeCount             int        `json:"like_count"`
	CreatedByCurrentRider int        `json:"created_by_current_rider"`
	CreatorRiderID        uint       `json:"creator_rider_id"`
	ImageURL              string     `json:"image_url"`
	VideoURL              string     `json:"video_url"`
	SupportPercent        *int       `json:"support_percent"`
	ConsumptionWhPerKm    *float64   `json:"consumption_wh_per_km"`
	RangeKm               int        `json:"range_km"`
	ShortDescription      *string    `json:"short_description"`
	CreatedAt             time.Time  `json:"created_at"`
	PublishedAt           *time.Time `json:"published_at"`
}

type MotorModePublishRequest struct {
	ModeName           *string  `json:"mode_name"`
	ShortDescription   string   `json:"short_description"`
	SupportPercent     *int     `json:"support_percent"`
	ConsumptionWhPerKm *float64 `json:"consumption_wh_per_km"`
}

type MotorModeLikeRequest struct {
	Like *int `json:"like" binding:"required,oneof=0 1"`
}

type MotorModeLikeResponse struct {
	MotorModeID         uint `json:"motor_mode_id"`
	LikeCount           int  `json:"like_count"`
	LikedByCurrentRider int  `json:"liked_by_current_rider"`
}

type MotorModeDeleteResponse struct {
	ID     uint   `json:"id"`
	Status string `json:"status"`
}
