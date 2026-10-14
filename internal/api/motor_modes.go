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

func (h *MotorModeHandler) mediaURL(key string) string {
	return h.mediaBaseURL + "/" + key
}

// @Summary Список опубликованных режимов работы мотора
// @Description Доступен гостю. Фильтр по расходу батареи выполняется в БД. Признак created_by_current_rider равен 1 только у режимов авторизованного велосипедиста, у гостя всегда 0.
// @Tags motor_modes
// @Produce json
// @Param max_consumption_wh_per_km query number false "Наибольший расход батареи, Вт·ч/км: больше 0 и меньше 1000"
// @Success 200 {object} serializer.MotorModeList
// @Failure 400 {object} serializer.Message "Фильтр не число или вне диапазона"
// @Router /motor_modes [get]
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

	riderID := currentRiderID(c)
	cards := make([]serializer.MotorModeCard, 0, len(motorModes))
	for _, motorMode := range motorModes {
		cards = append(cards, serializer.MotorModeCard{
			ID:                    motorMode.ID,
			ModeName:              motorMode.ModeName,
			ImageURL:              h.mediaURL(motorMode.ImageKey),
			SupportPercent:        motorMode.SupportPercent,
			ConsumptionWhPerKm:    motorMode.ConsumptionWhPerKm,
			LikeCount:             likeCounts[motorMode.ID],
			CreatedByCurrentRider: flag(riderID != 0 && motorMode.CreatorRiderID == riderID),
		})
	}

	c.JSON(http.StatusOK, serializer.MotorModeList{
		MotorModes:            cards,
		MaxConsumptionWhPerKm: selected,
		ConsumptionMin:        minConsumption,
		ConsumptionMax:        maxConsumption,
	})
}

// @Summary Лента: первый опубликованный режим
// @Description Доступен гостю. Признак liked_by_current_rider равен 1, если авторизованный велосипедист поставил лайк этому режиму.
// @Tags motor_modes
// @Produce json
// @Success 200 {object} serializer.MotorModeDetail
// @Failure 404 {object} serializer.Message "Опубликованных режимов нет"
// @Router /motor_modes/feed [get]
func (h *MotorModeHandler) GetMotorModeFeed(c *gin.Context) {
	motorMode, err := h.repository.FirstPublishedMode()
	h.respondFeed(c, motorMode, err)
}

// @Summary Лента: режим по идентификатору или следующий за ним
// @Description Доступен гостю. С параметром next=true возвращает следующий опубликованный режим после указанного, после последнего снова первый.
// @Tags motor_modes
// @Produce json
// @Param motor_mode_id path int true "Идентификатор режима работы мотора"
// @Param next query bool false "true — вернуть следующий опубликованный режим"
// @Success 200 {object} serializer.MotorModeDetail
// @Failure 400 {object} serializer.Message "Идентификатор не число"
// @Failure 404 {object} serializer.Message "Режима нет, он черновик или удалён"
// @Router /motor_modes/feed/{motor_mode_id} [get]
func (h *MotorModeHandler) GetMotorModeFeedByID(c *gin.Context) {
	id, ok := motorModeIDParam(c)
	if !ok {
		return
	}
	var (
		motorMode ds.MotorMode
		err       error
	)
	if c.Query("next") == "true" {
		motorMode, err = h.repository.NextPublishedMode(id)
	} else {
		motorMode, err = h.repository.PublishedModeByID(id)
	}
	h.respondFeed(c, motorMode, err)
}

func (h *MotorModeHandler) respondFeed(c *gin.Context, motorMode ds.MotorMode, err error) {
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

// @Summary Черновик режима текущего велосипедиста
// @Description Только для авторизованного велосипедиста. Идентификатор не указывается: владелец черновика берётся из JWT.
// @Tags motor_modes
// @Produce json
// @Security BearerAuth
// @Success 200 {object} serializer.MotorModeDetail
// @Failure 401 {object} serializer.Message "Нет JWT, он недействителен, просрочен или отозван"
// @Failure 404 {object} serializer.Message "У велосипедиста нет черновика"
// @Router /motor_modes/draft [get]
func (h *MotorModeHandler) GetMotorModeDraft(c *gin.Context) {
	draft, err := h.repository.DraftByRider(currentRiderID(c))
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

// @Summary Добавить режим работы мотора с фото и видео
// @Description Только для авторизованного велосипедиста. Создаёт черновик, создатель заполняется из JWT. Файлы сохраняются в Minio под латинскими именами вида motor_mode_15.jpg и motor_mode_15.mp4.
// @Tags motor_modes
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param mode_name formData string true "Название режима, до 50 символов"
// @Param image formData file true "Фото режима: JPEG, PNG или WebP до 10 МБ"
// @Param video formData file true "Короткое видео режима: MP4 до 50 МБ"
// @Success 201 {object} serializer.MotorModeDetail
// @Failure 400 {object} serializer.Message "Нет названия, фото или видео либо файл не того типа"
// @Failure 401 {object} serializer.Message "Нет JWT, он недействителен, просрочен или отозван"
// @Failure 409 {object} serializer.Message "У велосипедиста уже есть черновик"
// @Failure 413 {object} serializer.Message "Файл больше допустимого размера"
// @Failure 415 {object} serializer.Message "Тело запроса не multipart/form-data"
// @Failure 502 {object} serializer.Message "Minio недоступен"
// @Router /motor_modes [post]
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
	draft, err := h.repository.CreateDraftWithMedia(currentRiderID(c), modeName, func(motorModeID uint) (repository.MediaKeys, error) {
		var keys repository.MediaKeys
		for _, media := range []struct {
			file *mediaFile
			key  *string
		}{{image, &keys.ImageKey}, {video, &keys.VideoKey}} {
			key := fmt.Sprintf("motor_mode_%d%s", motorModeID, media.file.extension)
			uploaded = append(uploaded, key)
			if err := h.upload(uploadCtx, key, media.file); err != nil {
				return keys, err
			}
			*media.key = key
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

// @Summary Опубликовать свой черновик режима
// @Description Только для авторизованного велосипедиста. Меняет статус draft на published, дату публикации ставит сервер.
// @Tags motor_modes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param motor_mode_id path int true "Идентификатор черновика режима"
// @Param motor_mode body serializer.MotorModePublishRequest true "Краткое описание и два поля по теме"
// @Success 200 {object} serializer.MotorModeDetail
// @Failure 400 {object} serializer.Message "Поле не заполнено или вне диапазона"
// @Failure 401 {object} serializer.Message "Нет JWT, он недействителен, просрочен или отозван"
// @Failure 403 {object} serializer.Message "Режим создал другой велосипедист"
// @Failure 404 {object} serializer.Message "Режима нет или он удалён"
// @Failure 409 {object} serializer.Message "Режим уже опубликован"
// @Router /motor_modes/{motor_mode_id}/publish [put]
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
	riderID := currentRiderID(c)
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

// @Summary Удалить свой режим работы мотора
// @Description Только для авторизованного велосипедиста. Мягкое удаление: статус меняется на deleted, строка остаётся в БД.
// @Tags motor_modes
// @Produce json
// @Security BearerAuth
// @Param motor_mode_id path int true "Идентификатор режима работы мотора"
// @Success 200 {object} serializer.MotorModeDeleteResponse
// @Failure 400 {object} serializer.Message "Идентификатор не число"
// @Failure 401 {object} serializer.Message "Нет JWT, он недействителен, просрочен или отозван"
// @Failure 403 {object} serializer.Message "Режим создал другой велосипедист"
// @Failure 404 {object} serializer.Message "Режима нет или он уже удалён"
// @Router /motor_modes/{motor_mode_id} [delete]
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
	riderID := currentRiderID(c)
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

// @Summary Поставить или снять лайк режиму
// @Description Только для авторизованного велосипедиста. like=1 ставит лайк от велосипедиста из JWT, like=0 снимает.
// @Tags motor_modes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param motor_mode_id path int true "Идентификатор опубликованного режима"
// @Param like body serializer.MotorModeLikeRequest true "1 — поставить лайк, 0 — снять"
// @Success 200 {object} serializer.MotorModeLikeResponse
// @Failure 400 {object} serializer.Message "like не 0 и не 1"
// @Failure 401 {object} serializer.Message "Нет JWT, он недействителен, просрочен или отозван"
// @Failure 404 {object} serializer.Message "Режим не опубликован"
// @Router /motor_modes/{motor_mode_id}/like [post]
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

	riderID := currentRiderID(c)
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
	riderID := currentRiderID(c)
	likeCount, err := h.repository.LikeCount(motorMode.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	liked := false
	if riderID != 0 {
		liked, err = h.repository.IsLikedBy(riderID, motorMode.ID)
		if err != nil {
			serverError(c, err)
			return
		}
	}
	c.JSON(status, serializer.MotorModeDetail{
		ID:                    motorMode.ID,
		ModeName:              motorMode.ModeName,
		Status:                string(motorMode.Status),
		LikedByCurrentRider:   flag(liked),
		LikeCount:             likeCount,
		CreatedByCurrentRider: flag(riderID != 0 && motorMode.CreatorRiderID == riderID),
		CreatorRiderID:        motorMode.CreatorRiderID,
		ImageURL:              h.mediaURL(motorMode.ImageKey),
		VideoURL:              h.mediaURL(motorMode.VideoKey),
		SupportPercent:        motorMode.SupportPercent,
		ConsumptionWhPerKm:    motorMode.ConsumptionWhPerKm,
		RangeKm:               motorMode.RangeKm(),
		ShortDescription:      motorMode.ShortDescription,
		CreatedAt:             motorMode.CreatedAt,
		PublishedAt:           motorMode.PublishedAt,
	})
}
