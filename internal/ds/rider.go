package ds

type Rider struct {
	ID                    uint   `gorm:"primaryKey"`
	Login                 string `gorm:"type:varchar(50);not null;uniqueIndex"`
	PasswordHash          string `gorm:"type:varchar(100);not null"`
	IsBikeServiceEngineer bool   `gorm:"not null;default:false"`
}

func (Rider) TableName() string {
	return "riders"
}
