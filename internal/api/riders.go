package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ebike-battery-backend/internal/auth"
	"ebike-battery-backend/internal/ds"
	"ebike-battery-backend/internal/repository"
	"ebike-battery-backend/internal/serializer"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLength = 6
	maxPasswordBytes  = 72
)

var loginPattern = regexp.MustCompile(`^[a-z0-9_]{3,50}$`)

type RiderRepository interface {
	CreateRider(login, passwordHash string) (ds.Rider, error)
	RiderByLogin(login string) (ds.Rider, error)
}

type RiderHandler struct {
	repository RiderRepository
	tokens     *auth.TokenManager
	blacklist  TokenBlacklist
}

func NewRiderHandler(repo RiderRepository, tokens *auth.TokenManager, blacklist TokenBlacklist) *RiderHandler {
	return &RiderHandler{repository: repo, tokens: tokens, blacklist: blacklist}
}

func riderJSON(rider ds.Rider) serializer.Rider {
	return serializer.Rider{ID: rider.ID, Login: rider.Login, IsBikeServiceEngineer: rider.IsBikeServiceEngineer}
}

func bindCredentials(c *gin.Context) (serializer.RiderCredentialsRequest, bool) {
	var request serializer.RiderCredentialsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, http.StatusBadRequest, "Тело запроса — JSON с полями login и password.")
		return request, false
	}
	request.Login = strings.TrimSpace(request.Login)
	if !ds.ValidText(request.Login) || !ds.ValidText(request.Password) {
		fail(c, http.StatusBadRequest, "Логин и пароль содержат недопустимые символы.")
		return request, false
	}
	return request, true
}

// @Summary Регистрация велосипедиста
// @Description Доступна гостю. Пароль хранится bcrypt-хешем.
// @Tags riders
// @Accept json
// @Produce json
// @Param credentials body serializer.RiderCredentialsRequest true "Логин и пароль"
// @Success 201 {object} serializer.Rider
// @Failure 400 {object} serializer.Message "Логин или пароль не подходят"
// @Failure 409 {object} serializer.Message "Логин уже занят"
// @Router /riders/register [post]
func (h *RiderHandler) RegisterRider(c *gin.Context) {
	request, ok := bindCredentials(c)
	if !ok {
		return
	}
	if !loginPattern.MatchString(request.Login) {
		fail(c, http.StatusBadRequest, "Логин — от 3 до 50 символов: строчные латинские буквы, цифры и подчёркивание.")
		return
	}
	if utf8.RuneCountInString(request.Password) < minPasswordLength || len(request.Password) > maxPasswordBytes {
		fail(c, http.StatusBadRequest, "Пароль — не короче 6 символов и не длиннее 72 байт.")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		serverError(c, err)
		return
	}
	rider, err := h.repository.CreateRider(request.Login, string(passwordHash))
	if errors.Is(err, repository.ErrLoginTaken) {
		fail(c, http.StatusConflict, "Логин уже занят.")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, riderJSON(rider))
}

// @Summary Аутентификация велосипедиста
// @Description Доступна гостю. Проверяет пароль и выдаёт JWT. Значение access_token передаётся в остальных запросах заголовком Authorization после слова Bearer.
// @Tags riders
// @Accept json
// @Produce json
// @Param credentials body serializer.RiderCredentialsRequest true "Логин и пароль"
// @Success 200 {object} serializer.RiderLoginResponse
// @Failure 400 {object} serializer.Message "Тело запроса не JSON с логином и паролем"
// @Failure 401 {object} serializer.Message "Неверный логин или пароль"
// @Router /riders/login [post]
func (h *RiderHandler) LoginRider(c *gin.Context) {
	request, ok := bindCredentials(c)
	if !ok {
		return
	}
	rider, err := h.repository.RiderByLogin(request.Login)
	if errors.Is(err, repository.ErrRiderNotFound) {
		fail(c, http.StatusUnauthorized, "Неверный логин или пароль.")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(rider.PasswordHash), []byte(request.Password)) != nil {
		fail(c, http.StatusUnauthorized, "Неверный логин или пароль.")
		return
	}
	accessToken, err := h.tokens.Issue(rider)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, serializer.RiderLoginResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(h.tokens.ExpiresIn().Seconds()),
		Rider:       riderJSON(rider),
	})
}

// @Summary Деавторизация велосипедиста
// @Description Только для авторизованного велосипедиста. JWT записывается в чёрный список Redis до истечения срока действия.
// @Tags riders
// @Produce json
// @Security BearerAuth
// @Success 200 {object} serializer.Message
// @Failure 401 {object} serializer.Message "Нет JWT, он недействителен, просрочен или отозван"
// @Router /riders/logout [post]
func (h *RiderHandler) LogoutRider(c *gin.Context) {
	session := currentSession(c)
	if session == nil {
		fail(c, http.StatusUnauthorized, "Метод доступен только авторизованному велосипедисту.")
		return
	}
	ttl := time.Until(session.claims.ExpiresAt.Time)
	if ttl > 0 {
		if err := h.blacklist.Add(c.Request.Context(), session.token, session.claims.Subject, ttl); err != nil {
			serverError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, serializer.Message{Message: "Выход выполнен, JWT добавлен в чёрный список."})
}
