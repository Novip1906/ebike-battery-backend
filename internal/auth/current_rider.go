package auth

import "sync"

const fixedCreatorRiderID uint = 1

var (
	currentRiderOnce sync.Once
	currentRiderID   uint
)

func CurrentRiderID() uint {
	currentRiderOnce.Do(func() {
		currentRiderID = fixedCreatorRiderID
	})
	return currentRiderID
}
