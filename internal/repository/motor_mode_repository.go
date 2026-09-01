package repository

import "ebike-battery-backend/internal/ds"

type MotorModeRepository struct {
	motorModes []ds.MotorMode
}

func NewMotorModeRepository() *MotorModeRepository {
	return &MotorModeRepository{motorModes: motorModeCollection()}
}

func (r *MotorModeRepository) PublishedModes() []ds.MotorMode {
	result := make([]ds.MotorMode, 0, len(r.motorModes))
	for _, mode := range r.motorModes {
		if mode.Status == ds.StatusPublished {
			result = append(result, mode)
		}
	}
	return result
}

func (r *MotorModeRepository) FilterByConsumption(maxConsumption float64) []ds.MotorMode {
	published := r.PublishedModes()
	if maxConsumption <= 0 {
		return published
	}
	result := make([]ds.MotorMode, 0, len(published))
	for _, mode := range published {
		if mode.ConsumptionWhPerKm <= maxConsumption {
			result = append(result, mode)
		}
	}
	return result
}

func (r *MotorModeRepository) ModeByID(id int) (ds.MotorMode, bool) {
	for _, mode := range r.PublishedModes() {
		if mode.ID == id {
			return mode, true
		}
	}
	return ds.MotorMode{}, false
}

func (r *MotorModeRepository) NextMode(afterID int) (ds.MotorMode, bool) {
	published := r.PublishedModes()
	if len(published) == 0 {
		return ds.MotorMode{}, false
	}
	for i, mode := range published {
		if mode.ID == afterID {
			return published[(i+1)%len(published)], true
		}
	}
	return published[0], true
}

func (r *MotorModeRepository) FirstMode() (ds.MotorMode, bool) {
	published := r.PublishedModes()
	if len(published) == 0 {
		return ds.MotorMode{}, false
	}
	return published[0], true
}

func (r *MotorModeRepository) DraftMode() (ds.MotorMode, bool) {
	for _, mode := range r.motorModes {
		if mode.Status == ds.StatusDraft {
			return mode, true
		}
	}
	return ds.MotorMode{}, false
}
