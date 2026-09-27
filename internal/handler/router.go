package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, motorModeHandler *MotorModeHandler) {
	router.GET("/motor_modes/feed", motorModeHandler.MotorModeFeed)
	router.GET("/motor_modes/feed/:motor_mode_id", motorModeHandler.MotorModeFeed)
	router.GET("/motor_modes/draft", motorModeHandler.MotorModeDraft)
	router.GET("/motor_modes", motorModeHandler.MotorModeGrid)

	router.POST("/motor_modes/draft", motorModeHandler.CreateMotorModeDraft)
	router.POST("/motor_modes/draft/publish", motorModeHandler.PublishMotorModeDraft)
	router.POST("/motor_modes/:motor_mode_id/delete", motorModeHandler.DeleteMotorMode)

	router.NoRoute(motorModeHandler.notFound)

	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/motor_modes/feed")
	})
}
