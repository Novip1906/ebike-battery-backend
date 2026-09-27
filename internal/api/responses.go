package api

import (
	"log"
	"net/http"
	"strconv"

	"ebike-battery-backend/internal/serializer"

	"github.com/gin-gonic/gin"
)

func fail(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, serializer.Message{Status: "fail", Message: message})
}

func serverError(c *gin.Context, err error) {
	log.Printf("ошибка API %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	c.AbortWithStatusJSON(http.StatusInternalServerError, serializer.Message{Status: "error", Message: "Внутренняя ошибка сервера."})
}

func motorModeIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("motor_mode_id"), 10, 32)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "Идентификатор режима должен быть положительным целым числом.")
		return 0, false
	}
	return uint(id), true
}

func flag(value bool) int {
	if value {
		return 1
	}
	return 0
}
