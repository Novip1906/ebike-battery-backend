package handler

import (
	"errors"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"ebike-battery-backend/internal/auth"
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

type MotorModeView struct {
	ds.MotorMode
	ImageURL  string
	VideoURL  string
	LikeCount int
}

func (h *MotorModeHandler) view(motorMode ds.MotorMode, likeCount int) MotorModeView {
	return MotorModeView{
		MotorMode: motorMode,
		ImageURL:  h.mediaBaseURL + "/" + motorMode.ImageKey,
		VideoURL:  h.mediaBaseURL + "/" + motorMode.VideoKey,
		LikeCount: likeCount,
	}
}

func (h *MotorModeHandler) MotorModeFeed(c *gin.Context) {
	rawID := c.Param("motor_mode_id")

	var (
		motorMode ds.MotorMode
		err       error
	)

	if rawID == "" {
		motorMode, err = h.repository.FirstPublishedMode()
	} else {
		requestedID, parseErr := strconv.ParseUint(rawID, 10, 32)
		if parseErr != nil {
			h.NotFound(c)
			return
		}
		if c.Query("next") == "true" {
			motorMode, err = h.repository.NextPublishedMode(uint(requestedID))
		} else {
			motorMode, err = h.repository.PublishedModeByID(uint(requestedID))
		}
	}

	if errors.Is(err, repository.ErrMotorModeNotFound) {
		h.NotFound(c)
		return
	}
	if err != nil {
		h.serverError(c, err)
		return
	}

	likeCount, err := h.repository.LikeCount(motorMode.ID)
	if err != nil {
		h.serverError(c, err)
		return
	}

	c.HTML(http.StatusOK, "motor_modes_feed.html", gin.H{
		"MotorMode": h.view(motorMode, likeCount),
		"RangeKm":   motorMode.RangeKm(),
		"ActiveTab": "feed",
	})
}

func (h *MotorModeHandler) MotorModeDraft(c *gin.Context) {
	h.renderDraft(c, http.StatusOK, nil, "")
}

func (h *MotorModeHandler) renderDraft(c *gin.Context, status int, draft *ds.MotorMode, errorMessage string) {
	if draft == nil {
		motorMode, err := h.repository.DraftByRider(auth.CurrentRiderID())
		switch {
		case errors.Is(err, repository.ErrMotorModeNotFound):
			draft = nil
		case err != nil:
			h.serverError(c, err)
			return
		default:
			draft = &motorMode
		}
	}

	data := gin.H{
		"HasDraft":     draft != nil,
		"ErrorMessage": errorMessage,
		"ActiveTab":    "draft",
	}
	if draft != nil {
		data["MotorMode"] = h.view(*draft, 0)
	}
	c.HTML(status, "motor_modes_draft.html", data)
}

func (h *MotorModeHandler) CreateMotorModeDraft(c *gin.Context) {
	modeName := strings.TrimSpace(c.PostForm("mode_name"))
	if errorMessage := ds.ValidateModeName(modeName); errorMessage != "" {
		h.renderDraft(c, http.StatusUnprocessableEntity, nil, errorMessage)
		return
	}

	_, err := h.repository.CreateDraft(auth.CurrentRiderID(), modeName)
	if err != nil && !errors.Is(err, repository.ErrDraftAlreadyExists) {
		h.serverError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/motor_modes/draft")
}

func (h *MotorModeHandler) PublishMotorModeDraft(c *gin.Context) {
	draft, err := h.repository.DraftByRider(auth.CurrentRiderID())
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		c.Redirect(http.StatusSeeOther, "/motor_modes/draft")
		return
	}
	if err != nil {
		h.serverError(c, err)
		return
	}

	fields := repository.PublishFields{
		ModeName:         strings.TrimSpace(c.PostForm("mode_name")),
		ShortDescription: strings.TrimSpace(c.PostForm("short_description")),
	}
	supportPercent, supportErr := strconv.Atoi(strings.TrimSpace(c.PostForm("support_percent")))
	consumption, consumptionErr := strconv.ParseFloat(strings.TrimSpace(c.PostForm("consumption_wh_per_km")), 64)
	consumption = ds.RoundConsumption(consumption)

	var supportValue *int
	if supportErr == nil {
		supportValue = &supportPercent
	}
	var consumptionValue *float64
	if consumptionErr == nil {
		consumptionValue = &consumption
	}

	errorMessage := ds.ValidateModeName(fields.ModeName)
	if errorMessage == "" {
		errorMessage = ds.ValidatePublication(fields.ShortDescription, supportValue, consumptionValue)
	}

	if errorMessage != "" {
		draft.ModeName = fields.ModeName
		draft.ShortDescription = &fields.ShortDescription
		draft.SupportPercent = supportValue
		draft.ConsumptionWhPerKm = consumptionValue
		h.renderDraft(c, http.StatusUnprocessableEntity, &draft, errorMessage)
		return
	}

	fields.SupportPercent = supportPercent
	fields.ConsumptionWhPerKm = consumption

	err = h.repository.PublishDraft(auth.CurrentRiderID(), draft.ID, fields)
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		c.Redirect(http.StatusSeeOther, "/motor_modes/draft")
		return
	}
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/motor_modes/feed/"+strconv.FormatUint(uint64(draft.ID), 10))
}

func (h *MotorModeHandler) DeleteMotorMode(c *gin.Context) {
	motorModeID, err := strconv.ParseUint(c.Param("motor_mode_id"), 10, 32)
	if err != nil {
		h.NotFound(c)
		return
	}

	err = h.repository.MarkDeletedBySQL(uint(motorModeID))
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		h.NotFound(c)
		return
	}
	if err != nil {
		h.serverError(c, err)
		return
	}

	target := "/motor_modes"
	if filter := c.PostForm("maxConsumptionWhPerKm"); filter != "" {
		target += "?maxConsumptionWhPerKm=" + url.QueryEscape(filter)
	}
	c.Redirect(http.StatusSeeOther, target)
}

func (h *MotorModeHandler) MotorModeGrid(c *gin.Context) {
	minConsumption, maxConsumption, err := h.repository.ConsumptionBounds()
	if err != nil {
		h.serverError(c, err)
		return
	}
	maxConsumption = math.Ceil(maxConsumption)

	selected, parseErr := strconv.ParseFloat(c.Query("maxConsumptionWhPerKm"), 64)
	selected = math.Round(selected*10) / 10
	if parseErr != nil || math.IsNaN(selected) || selected < minConsumption || selected > maxConsumption {
		selected = maxConsumption
	}

	motorModes, err := h.repository.PublishedModes(selected)
	if err != nil {
		h.serverError(c, err)
		return
	}
	likeCounts, err := h.repository.LikeCounts()
	if err != nil {
		h.serverError(c, err)
		return
	}

	views := make([]MotorModeView, 0, len(motorModes))
	for _, motorMode := range motorModes {
		views = append(views, h.view(motorMode, likeCounts[motorMode.ID]))
	}

	c.HTML(http.StatusOK, "motor_modes_grid.html", gin.H{
		"MotorModes":            views,
		"MaxConsumptionWhPerKm": formatConsumption(selected),
		"ConsumptionMin":        formatConsumption(minConsumption),
		"ConsumptionMax":        formatConsumption(maxConsumption),
		"ConsumptionTicks":      consumptionTicks(minConsumption, maxConsumption),
		"ActiveTab":             "grid",
	})
}

func (h *MotorModeHandler) NotFound(c *gin.Context) {
	c.HTML(http.StatusNotFound, "not_found.html", gin.H{"ActiveTab": ""})
}

func (h *MotorModeHandler) serverError(c *gin.Context, err error) {
	log.Printf("ошибка обработки %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	c.HTML(http.StatusInternalServerError, "server_error.html", gin.H{"ActiveTab": ""})
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
