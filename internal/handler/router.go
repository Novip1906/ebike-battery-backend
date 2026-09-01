package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, motorModeHandler *MotorModeHandler) {
	router.GET("/motor-modes/feed", motorModeHandler.MotorModeFeed)
	router.GET("/motor-modes/feed/:motor_mode_id", motorModeHandler.MotorModeFeed)
	router.GET("/motor-modes/draft", motorModeHandler.MotorModeDraft)
	router.GET("/motor-modes", motorModeHandler.MotorModeGrid)

	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/motor-modes/feed")
	})
}
