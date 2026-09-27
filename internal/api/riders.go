package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

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
}

func NewRiderHandler(repo RiderRepository) *RiderHandler {
	return &RiderHandler{repository: repo}
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
	c.JSON(http.StatusOK, serializer.RiderLoginResponse{
		Rider:   riderJSON(rider),
		Message: "Логин и пароль верны.",
	})
}

func (h *RiderHandler) LogoutRider(c *gin.Context) {
	c.JSON(http.StatusOK, serializer.Message{
		Status:  "success",
		Message: "Выход выполнен.",
	})
}
