package handler

import (
	"net/http"
	"strconv"

	"ebike-battery-backend/internal/ds"
	"ebike-battery-backend/internal/repository"

	"github.com/gin-gonic/gin"
)

type MotorModeHandler struct {
	repository   *repository.MotorModeRepository
	mediaBaseURL string
}

func NewMotorModeHandler(repo *repository.MotorModeRepository, mediaBaseURL string) *MotorModeHandler {
	return &MotorModeHandler{repository: repo, mediaBaseURL: mediaBaseURL}
}

func (h *MotorModeHandler) MotorModeFeed(c *gin.Context) {
	rawID := c.Param("motor_mode_id")
	wantsNext := c.Query("next") == "true"

	var (
		motorMode ds.MotorMode
		found     bool
	)

	if rawID == "" {
		motorMode, found = h.repository.FirstMode()
	} else {
		requestedID, err := strconv.Atoi(rawID)
		if err != nil {
			c.HTML(http.StatusNotFound, "not_found.html", gin.H{"RequestedID": rawID})
			return
		}
		if wantsNext {
			motorMode, found = h.repository.NextMode(requestedID)
		} else {
			motorMode, found = h.repository.ModeByID(requestedID)
		}
	}

	if !found {
		c.HTML(http.StatusNotFound, "not_found.html", gin.H{"RequestedID": rawID})
		return
	}

	c.HTML(http.StatusOK, "motor_mode_feed.html", gin.H{
		"MotorMode":    motorMode,
		"LikeCount":    motorMode.LikeCount(),
		"RangeKm":      motorMode.RangeKm(),
		"MediaBaseURL": h.mediaBaseURL,
		"ActiveTab":    "feed",
	})
}

func (h *MotorModeHandler) MotorModeDraft(c *gin.Context) {
	motorMode, found := h.repository.DraftMode()
	if !found {
		c.HTML(http.StatusNotFound, "not_found.html", gin.H{"RequestedID": "черновик"})
		return
	}

	c.HTML(http.StatusOK, "motor_mode_draft.html", gin.H{
		"MotorMode":    motorMode,
		"MediaBaseURL": h.mediaBaseURL,
		"ActiveTab":    "draft",
	})
}

func (h *MotorModeHandler) MotorModeGrid(c *gin.Context) {
	rawFilter := c.Query("maxConsumptionWhPerKm")
	maxConsumption, err := strconv.ParseFloat(rawFilter, 64)
	if err != nil {
		maxConsumption = 0
	}

	motorModes := h.repository.FilterByConsumption(maxConsumption)
	likeCounts := make(map[int]int, len(motorModes))
	for _, motorMode := range motorModes {
		likeCounts[motorMode.ID] = motorMode.LikeCount()
	}

	c.HTML(http.StatusOK, "motor_mode_grid.html", gin.H{
		"MotorModes":            motorModes,
		"LikeCounts":            likeCounts,
		"MaxConsumptionWhPerKm": rawFilter,
		"MediaBaseURL":          h.mediaBaseURL,
		"ActiveTab":             "grid",
	})
}
