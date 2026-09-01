package ds

type MotorModeStatus string

const (
	StatusDraft     MotorModeStatus = "черновик"
	StatusPublished MotorModeStatus = "опубликован"
	StatusDeleted   MotorModeStatus = "удален"
)

type MotorMode struct {
	ID                 int
	ModeName           string
	ShortDescription   string
	Status             MotorModeStatus
	ImageKey           string
	VideoKey           string
	SupportPercent     int
	ConsumptionWhPerKm float64
	MaxTorqueNm        int
	DriveUnit          string
	SupportCharacter   string
	LikedByRiderIDs    []int
}

func (m MotorMode) LikeCount() int {
	return len(m.LikedByRiderIDs)
}

func (m MotorMode) RangeKm() int {
	if m.ConsumptionWhPerKm <= 0 {
		return 0
	}
	return int(BatteryCapacityWh / m.ConsumptionWhPerKm)
}

const BatteryCapacityWh = 800
