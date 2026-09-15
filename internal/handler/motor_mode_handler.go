package handler

import (
	"errors"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"ebike-battery-backend/internal/ds"
	"ebike-battery-backend/internal/repository"

	"github.com/gin-gonic/gin"
)

const (
	DefaultImageURL = "/static/media/default_motor_mode.jpg"
	DefaultVideoURL = "/static/media/default_motor_mode.mp4"

	maxModeNameLength         = 50
	maxShortDescriptionLength = 600
)

type MotorModeHandler struct {
	repository     *repository.MotorModeRepository
	mediaBaseURL   string
	currentRiderID uint
}

func NewMotorModeHandler(repo *repository.MotorModeRepository, mediaBaseURL string, currentRiderID uint) *MotorModeHandler {
	return &MotorModeHandler{repository: repo, mediaBaseURL: mediaBaseURL, currentRiderID: currentRiderID}
}

type MotorModeView struct {
	ds.MotorMode
	ImageURL  string
	VideoURL  string
	LikeCount int
}

func (h *MotorModeHandler) view(motorMode ds.MotorMode, likeCount int) MotorModeView {
	view := MotorModeView{MotorMode: motorMode, ImageURL: DefaultImageURL, VideoURL: DefaultVideoURL, LikeCount: likeCount}
	if motorMode.ImageKey != "" {
		view.ImageURL = h.mediaBaseURL + "/" + motorMode.ImageKey
	}
	if motorMode.VideoKey != "" {
		view.VideoURL = h.mediaBaseURL + "/" + motorMode.VideoKey
	}
	return view
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
			h.notFound(c, rawID)
			return
		}
		if c.Query("next") == "true" {
			motorMode, err = h.repository.NextPublishedMode(uint(requestedID))
		} else {
			motorMode, err = h.repository.PublishedModeByID(uint(requestedID))
		}
	}

	if errors.Is(err, repository.ErrMotorModeNotFound) {
		h.notFound(c, rawID)
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

	c.HTML(http.StatusOK, "motor_mode_feed.html", gin.H{
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
		motorMode, err := h.repository.DraftByRider(h.currentRiderID)
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
	c.HTML(status, "motor_mode_draft.html", data)
}

func validateModeName(modeName string) string {
	switch {
	case modeName == "":
		return "Укажите название режима работы мотора."
	case utf8.RuneCountInString(modeName) > maxModeNameLength:
		return "Название режима не длиннее 50 символов."
	}
	return ""
}

func (h *MotorModeHandler) CreateMotorModeDraft(c *gin.Context) {
	modeName := strings.TrimSpace(c.PostForm("mode_name"))
	if errorMessage := validateModeName(modeName); errorMessage != "" {
		h.renderDraft(c, http.StatusUnprocessableEntity, nil, errorMessage)
		return
	}

	_, err := h.repository.CreateDraft(h.currentRiderID, modeName)
	if err != nil && !errors.Is(err, repository.ErrDraftAlreadyExists) {
		h.serverError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/motor-modes/draft")
}

func (h *MotorModeHandler) PublishMotorModeDraft(c *gin.Context) {
	draft, err := h.repository.DraftByRider(h.currentRiderID)
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		c.Redirect(http.StatusSeeOther, "/motor-modes/draft")
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

	errorMessage := validateModeName(fields.ModeName)
	switch {
	case errorMessage != "":
	case fields.ShortDescription == "":
		errorMessage = "Заполните краткое описание режима."
	case utf8.RuneCountInString(fields.ShortDescription) > maxShortDescriptionLength:
		errorMessage = "Краткое описание не длиннее 600 символов."
	case supportErr != nil || supportPercent < 0 || supportPercent > 1000:
		errorMessage = "Поддержка мотора задаётся целым числом процентов от 0 до 1000."
	case consumptionErr != nil || math.IsNaN(consumption) || math.IsInf(consumption, 0) || consumption <= 0 || consumption >= 1000:
		errorMessage = "Расход батареи задаётся положительным числом Вт·ч/км."
	}

	if errorMessage != "" {
		draft.ModeName = fields.ModeName
		draft.ShortDescription = fields.ShortDescription
		if supportErr == nil {
			draft.SupportPercent = supportPercent
		}
		if consumptionErr == nil {
			draft.ConsumptionWhPerKm = consumption
		}
		h.renderDraft(c, http.StatusUnprocessableEntity, &draft, errorMessage)
		return
	}

	fields.SupportPercent = supportPercent
	fields.ConsumptionWhPerKm = math.Round(consumption*10) / 10

	err = h.repository.PublishDraft(draft.ID, fields)
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		c.Redirect(http.StatusSeeOther, "/motor-modes/draft")
		return
	}
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/motor-modes/feed/"+strconv.FormatUint(uint64(draft.ID), 10))
}

func (h *MotorModeHandler) DeleteMotorMode(c *gin.Context) {
	rawID := c.Param("motor_mode_id")
	motorModeID, err := strconv.ParseUint(rawID, 10, 32)
	if err != nil {
		h.notFound(c, rawID)
		return
	}

	err = h.repository.MarkDeletedBySQL(uint(motorModeID))
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		h.notFound(c, rawID)
		return
	}
	if err != nil {
		h.serverError(c, err)
		return
	}

	target := "/motor-modes"
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

	c.HTML(http.StatusOK, "motor_mode_grid.html", gin.H{
		"MotorModes":            views,
		"MaxConsumptionWhPerKm": formatConsumption(selected),
		"ConsumptionMin":        formatConsumption(minConsumption),
		"ConsumptionMax":        formatConsumption(maxConsumption),
		"ConsumptionTicks":      consumptionTicks(minConsumption, maxConsumption),
		"ActiveTab":             "grid",
	})
}

func (h *MotorModeHandler) notFound(c *gin.Context, requestedID string) {
	c.HTML(http.StatusNotFound, "not_found.html", gin.H{"RequestedID": requestedID, "ActiveTab": ""})
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
