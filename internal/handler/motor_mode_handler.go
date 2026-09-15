package handler

import (
	"math"
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
			c.HTML(http.StatusNotFound, "not_found.html", gin.H{"RequestedID": rawID, "ActiveTab": ""})
			return
		}
		if wantsNext {
			motorMode, found = h.repository.NextMode(requestedID)
		} else {
			motorMode, found = h.repository.ModeByID(requestedID)
		}
	}

	if !found {
		c.HTML(http.StatusNotFound, "not_found.html", gin.H{"RequestedID": rawID, "ActiveTab": ""})
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
		c.HTML(http.StatusNotFound, "not_found.html", gin.H{"RequestedID": "черновик", "ActiveTab": ""})
		return
	}

	c.HTML(http.StatusOK, "motor_mode_draft.html", gin.H{
		"MotorMode":    motorMode,
		"MediaBaseURL": h.mediaBaseURL,
		"ActiveTab":    "draft",
	})
}

func (h *MotorModeHandler) MotorModeGrid(c *gin.Context) {
	minConsumption, maxConsumption := h.repository.ConsumptionBounds()

	selected, err := strconv.ParseFloat(c.Query("maxConsumptionWhPerKm"), 64)
	selected = math.Round(selected*10) / 10
	if err != nil || math.IsNaN(selected) || selected < minConsumption || selected > maxConsumption {
		selected = maxConsumption
	}

	motorModes := h.repository.FilterByConsumption(selected)
	likeCounts := make(map[int]int, len(motorModes))
	for _, motorMode := range motorModes {
		likeCounts[motorMode.ID] = motorMode.LikeCount()
	}

	c.HTML(http.StatusOK, "motor_mode_grid.html", gin.H{
		"MotorModes":            motorModes,
		"LikeCounts":            likeCounts,
		"MaxConsumptionWhPerKm": formatConsumption(selected),
		"ConsumptionMin":        formatConsumption(minConsumption),
		"ConsumptionMax":        formatConsumption(maxConsumption),
		"ConsumptionTicks":      consumptionTicks(minConsumption, maxConsumption),
		"MediaBaseURL":          h.mediaBaseURL,
		"ActiveTab":             "grid",
	})
}

const consumptionTickStep = 2

type consumptionTick struct {
	Label   string
	Percent string
}

func formatConsumption(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func consumptionTicks(minConsumption, maxConsumption float64) []consumptionTick {
	if maxConsumption <= minConsumption {
		return nil
	}
	span := maxConsumption - minConsumption
	tick := func(value float64) consumptionTick {
		return consumptionTick{
			Label:   formatConsumption(value),
			Percent: strconv.FormatFloat((value-minConsumption)/span*100, 'f', 1, 64),
		}
	}
	ticks := []consumptionTick{tick(minConsumption)}
	for value := math.Ceil(minConsumption/consumptionTickStep) * consumptionTickStep; value < maxConsumption; value += consumptionTickStep {
		if value-minConsumption >= consumptionTickStep/2.0 {
			ticks = append(ticks, tick(value))
		}
	}
	return append(ticks, tick(maxConsumption))
}
