package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ebike-battery-backend/internal/auth"
	"ebike-battery-backend/internal/ds"
	"ebike-battery-backend/internal/repository"
	"ebike-battery-backend/internal/serializer"

	"github.com/gin-gonic/gin"
)

type MotorModeRepository interface {
	PublishedModes(maxConsumption float64) ([]ds.MotorMode, error)
	ConsumptionBounds() (float64, float64, error)
	LikeCounts() (map[uint]int, error)
	LikeCount(motorModeID uint) (int, error)
	IsLikedBy(riderID, motorModeID uint) (bool, error)
	FirstPublishedMode() (ds.MotorMode, error)
	NextPublishedMode(afterID uint) (ds.MotorMode, error)
	PublishedModeByID(id uint) (ds.MotorMode, error)
	ActiveModeByID(id uint) (ds.MotorMode, error)
	DraftByRider(riderID uint) (ds.MotorMode, error)
	CreateDraftWithMedia(riderID uint, modeName string, attachMedia func(motorModeID uint) (repository.MediaKeys, error)) (ds.MotorMode, error)
	PublishDraft(riderID, draftID uint, fields repository.PublishFields) error
	MarkDeleted(riderID, motorModeID uint) error
	SetLike(riderID, motorModeID uint, liked bool) error
}

type MediaStorage interface {
	Put(ctx context.Context, key string, content io.Reader, size int64, contentType string) error
	Remove(ctx context.Context, key string) error
}

const (
	mediaUploadTimeout  = 2 * time.Minute
	mediaCleanupTimeout = 10 * time.Second
)

var maxCreateBodySize int64 = maxImageSize + maxVideoSize + 1<<20

type MotorModeHandler struct {
	repository   MotorModeRepository
	storage      MediaStorage
	mediaBaseURL string
}

func NewMotorModeHandler(repo MotorModeRepository, storage MediaStorage, mediaBaseURL string) *MotorModeHandler {
	return &MotorModeHandler{repository: repo, storage: storage, mediaBaseURL: mediaBaseURL}
}

func (h *MotorModeHandler) mediaURL(key *string) *string {
	if key == nil || *key == "" {
		return nil
	}
	url := h.mediaBaseURL + "/" + *key
	return &url
}

func (h *MotorModeHandler) ListMotorModes(c *gin.Context) {
	minConsumption, maxConsumption, err := h.repository.ConsumptionBounds()
	if err != nil {
		serverError(c, err)
		return
	}
	maxConsumption = math.Ceil(maxConsumption)

	selected := maxConsumption
	if raw := c.Query("max_consumption_wh_per_km"); raw != "" {
		value, parseErr := strconv.ParseFloat(raw, 64)
		value = ds.RoundConsumption(value)
		if parseErr != nil || math.IsNaN(value) || value <= 0 || value >= ds.MaxConsumptionWhPerKm {
			fail(c, http.StatusBadRequest, "Фильтр max_consumption_wh_per_km задаётся числом Вт·ч/км больше 0 и меньше 1000.")
			return
		}
		selected = value
	}

	motorModes, err := h.repository.PublishedModes(selected)
	if err != nil {
		serverError(c, err)
		return
	}
	likeCounts, err := h.repository.LikeCounts()
	if err != nil {
		serverError(c, err)
		return
	}

	riderID := auth.CurrentRiderID()
	cards := make([]serializer.MotorModeCard, 0, len(motorModes))
	for _, motorMode := range motorModes {
		cards = append(cards, serializer.MotorModeCard{
			ID:                    motorMode.ID,
			ModeName:              motorMode.ModeName,
			ImageURL:              h.mediaURL(motorMode.ImageKey),
			SupportPercent:        motorMode.SupportPercent,
			ConsumptionWhPerKm:    motorMode.ConsumptionWhPerKm,
			LikeCount:             likeCounts[motorMode.ID],
			CreatedByCurrentRider: flag(motorMode.CreatorRiderID == riderID),
		})
	}

	c.JSON(http.StatusOK, serializer.MotorModeList{
		MotorModes:            cards,
		MaxConsumptionWhPerKm: selected,
		ConsumptionMin:        minConsumption,
		ConsumptionMax:        maxConsumption,
	})
}

func (h *MotorModeHandler) GetMotorModeFeed(c *gin.Context) {
	var (
		motorMode ds.MotorMode
		err       error
	)
	if c.Param("motor_mode_id") == "" {
		motorMode, err = h.repository.FirstPublishedMode()
	} else {
		id, ok := motorModeIDParam(c)
		if !ok {
			return
		}
		if c.Query("next") == "true" {
			motorMode, err = h.repository.NextPublishedMode(id)
		} else {
			motorMode, err = h.repository.PublishedModeByID(id)
		}
	}
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		fail(c, http.StatusNotFound, "Опубликованный режим работы мотора не найден.")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	h.respondDetail(c, http.StatusOK, motorMode)
}

func (h *MotorModeHandler) GetMotorModeDraft(c *gin.Context) {
	draft, err := h.repository.DraftByRider(auth.CurrentRiderID())
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		fail(c, http.StatusNotFound, "У текущего велосипедиста нет черновика режима.")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	h.respondDetail(c, http.StatusOK, draft)
}

type storageError struct {
	err error
}

func (e storageError) Error() string {
	return e.err.Error()
}

func (h *MotorModeHandler) CreateMotorMode(c *gin.Context) {
	if !strings.EqualFold(c.ContentType(), "multipart/form-data") {
		fail(c, http.StatusUnsupportedMediaType, "Режим добавляется формой multipart/form-data: mode_name, image, video.")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCreateBodySize)
	if _, err := c.MultipartForm(); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(c, http.StatusRequestEntityTooLarge, "Запрос слишком большой: фото до 10 МБ и видео до 50 МБ.")
			return
		}
		fail(c, http.StatusBadRequest, "Не удалось разобрать форму multipart/form-data.")
		return
	}
	modeName := strings.TrimSpace(c.PostForm("mode_name"))
	if message := ds.ValidateModeName(modeName); message != "" {
		fail(c, http.StatusBadRequest, message)
		return
	}
	image, err := readMediaFile(c, imageRule)
	if err != nil {
		h.mediaFailure(c, err)
		return
	}
	video, err := readMediaFile(c, videoRule)
	if err != nil {
		h.mediaFailure(c, err)
		return
	}

	uploadCtx, cancel := context.WithTimeout(c.Request.Context(), mediaUploadTimeout)
	defer cancel()
	var uploaded []string
	draft, err := h.repository.CreateDraftWithMedia(auth.CurrentRiderID(), modeName, func(motorModeID uint) (repository.MediaKeys, error) {
		var keys repository.MediaKeys
		for _, media := range []struct {
			file *mediaFile
			key  **string
		}{{image, &keys.ImageKey}, {video, &keys.VideoKey}} {
			if media.file == nil {
				continue
			}
			key := fmt.Sprintf("motor_mode_%d%s", motorModeID, media.file.extension)
			uploaded = append(uploaded, key)
			if err := h.upload(uploadCtx, key, media.file); err != nil {
				return keys, err
			}
			*media.key = &key
		}
		return keys, nil
	})
	if err != nil {
		h.removeUploaded(c.Request.Context(), uploaded)
		var failedStorage storageError
		switch {
		case errors.Is(err, repository.ErrDraftAlreadyExists):
			fail(c, http.StatusConflict, "У текущего велосипедиста уже есть черновик режима: опубликуйте или удалите его.")
		case errors.As(err, &failedStorage):
			log.Printf("ошибка Minio: %v", err)
			fail(c, http.StatusBadGateway, "Не удалось сохранить файлы в Minio.")
		default:
			serverError(c, err)
		}
		return
	}

	c.Header("Location", "/api/motor_modes/draft")
	h.respondDetail(c, http.StatusCreated, draft)
}

func (h *MotorModeHandler) upload(ctx context.Context, key string, media *mediaFile) error {
	file, err := media.header.Open()
	if err != nil {
		return err
	}
	defer file.Close()
	if err := h.storage.Put(ctx, key, file, media.header.Size, media.contentType); err != nil {
		return storageError{err: err}
	}
	return nil
}

func (h *MotorModeHandler) removeUploaded(parent context.Context, keys []string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), mediaCleanupTimeout)
	defer cancel()
	for _, key := range keys {
		if err := h.storage.Remove(ctx, key); err != nil {
			log.Printf("не удалось убрать %s из Minio: %v", key, err)
		}
	}
}

func (h *MotorModeHandler) mediaFailure(c *gin.Context, err error) {
	var invalid mediaError
	if errors.As(err, &invalid) {
		fail(c, invalid.status, invalid.message)
		return
	}
	serverError(c, err)
}

func (h *MotorModeHandler) PublishMotorMode(c *gin.Context) {
	id, ok := motorModeIDParam(c)
	if !ok {
		return
	}
	var request serializer.MotorModePublishRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "Тело запроса — JSON с полями short_description, support_percent, consumption_wh_per_km.")
		return
	}

	motorMode, err := h.repository.ActiveModeByID(id)
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		fail(c, http.StatusNotFound, "Режим работы мотора не найден.")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	riderID := auth.CurrentRiderID()
	if motorMode.CreatorRiderID != riderID {
		fail(c, http.StatusForbidden, "Опубликовать можно только свой черновик.")
		return
	}
	if !motorMode.Status.CanChangeTo(ds.StatusPublished) {
		fail(c, http.StatusConflict, "Опубликовать можно только черновик, этот режим уже опубликован.")
		return
	}

	fields := repository.PublishFields{
		ModeName:         motorMode.ModeName,
		ShortDescription: strings.TrimSpace(request.ShortDescription),
	}
	if request.ModeName != nil {
		fields.ModeName = strings.TrimSpace(*request.ModeName)
	}
	var consumption *float64
	if request.ConsumptionWhPerKm != nil {
		rounded := ds.RoundConsumption(*request.ConsumptionWhPerKm)
		consumption = &rounded
	}
	message := ds.ValidateModeName(fields.ModeName)
	if message == "" {
		message = ds.ValidatePublication(fields.ShortDescription, request.SupportPercent, consumption)
	}
	if message != "" {
		fail(c, http.StatusBadRequest, message)
		return
	}
	fields.SupportPercent = *request.SupportPercent
	fields.ConsumptionWhPerKm = *consumption

	err = h.repository.PublishDraft(riderID, id, fields)
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		fail(c, http.StatusConflict, "Черновик уже опубликован или удалён.")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	published, err := h.repository.PublishedModeByID(id)
	if err != nil {
		serverError(c, err)
		return
	}
	h.respondDetail(c, http.StatusOK, published)
}

func (h *MotorModeHandler) DeleteMotorMode(c *gin.Context) {
	id, ok := motorModeIDParam(c)
	if !ok {
		return
	}
	motorMode, err := h.repository.ActiveModeByID(id)
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		fail(c, http.StatusNotFound, "Режим работы мотора не найден.")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	riderID := auth.CurrentRiderID()
	if motorMode.CreatorRiderID != riderID {
		fail(c, http.StatusForbidden, "Удалить можно только свой режим.")
		return
	}
	if !motorMode.Status.CanChangeTo(ds.StatusDeleted) {
		fail(c, http.StatusConflict, "Этот режим удалить нельзя.")
		return
	}

	err = h.repository.MarkDeleted(riderID, id)
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		fail(c, http.StatusNotFound, "Режим работы мотора не найден.")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, serializer.MotorModeDeleteResponse{ID: id, Status: string(ds.StatusDeleted)})
}

func (h *MotorModeHandler) LikeMotorMode(c *gin.Context) {
	id, ok := motorModeIDParam(c)
	if !ok {
		return
	}
	var request serializer.MotorModeLikeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, `Тело запроса: {"like": 1} ставит лайк, {"like": 0} отменяет.`)
		return
	}
	_, err := h.repository.PublishedModeByID(id)
	if errors.Is(err, repository.ErrMotorModeNotFound) {
		fail(c, http.StatusNotFound, "Лайк ставится только опубликованному режиму.")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	riderID := auth.CurrentRiderID()
	if err := h.repository.SetLike(riderID, id, *request.Like == 1); err != nil {
		serverError(c, err)
		return
	}
	likeCount, err := h.repository.LikeCount(id)
	if err != nil {
		serverError(c, err)
		return
	}
	liked, err := h.repository.IsLikedBy(riderID, id)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, serializer.MotorModeLikeResponse{
		MotorModeID:         id,
		LikeCount:           likeCount,
		LikedByCurrentRider: flag(liked),
	})
}

func (h *MotorModeHandler) respondDetail(c *gin.Context, status int, motorMode ds.MotorMode) {
	riderID := auth.CurrentRiderID()
	likeCount, err := h.repository.LikeCount(motorMode.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	liked, err := h.repository.IsLikedBy(riderID, motorMode.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(status, serializer.MotorModeDetail{
		ID:                    motorMode.ID,
		ModeName:              motorMode.ModeName,
		ShortDescription:      motorMode.ShortDescription,
		Status:                string(motorMode.Status),
		ImageURL:              h.mediaURL(motorMode.ImageKey),
		VideoURL:              h.mediaURL(motorMode.VideoKey),
		SupportPercent:        motorMode.SupportPercent,
		ConsumptionWhPerKm:    motorMode.ConsumptionWhPerKm,
		RangeKm:               motorMode.RangeKm(),
		CreatedAt:             motorMode.CreatedAt,
		PublishedAt:           motorMode.PublishedAt,
		CreatorRiderID:        motorMode.CreatorRiderID,
		CreatedByCurrentRider: flag(motorMode.CreatorRiderID == riderID),
		LikeCount:             likeCount,
		LikedByCurrentRider:   flag(liked),
	})
}
