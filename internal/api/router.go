package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const Prefix = "/api"

func RegisterRoutes(router *gin.Engine, motorModes *MotorModeHandler, riders *RiderHandler) {
	routes := router.Group(Prefix)

	routes.GET("/motor_modes", motorModes.ListMotorModes)
	routes.GET("/motor_modes/feed", motorModes.GetMotorModeFeed)
	routes.GET("/motor_modes/feed/:motor_mode_id", motorModes.GetMotorModeFeed)
	routes.GET("/motor_modes/draft", motorModes.GetMotorModeDraft)
	routes.POST("/motor_modes", motorModes.CreateMotorMode)
	routes.PUT("/motor_modes/:motor_mode_id/publish", motorModes.PublishMotorMode)
	routes.DELETE("/motor_modes/:motor_mode_id", motorModes.DeleteMotorMode)
	routes.POST("/motor_modes/:motor_mode_id/like", motorModes.LikeMotorMode)

	routes.POST("/riders/register", riders.RegisterRider)
	routes.POST("/riders/login", riders.LoginRider)
	routes.POST("/riders/logout", riders.LogoutRider)
}

func NoRoute(pages gin.HandlerFunc) gin.HandlerFunc {
	return apiOrPages(http.StatusNotFound, "Метод API не найден.", pages)
}

func NoMethod(pages gin.HandlerFunc) gin.HandlerFunc {
	return apiOrPages(http.StatusMethodNotAllowed, "Этот HTTP-метод не поддерживается по данному адресу.", pages)
}

func apiOrPages(status int, message string, pages gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == Prefix || strings.HasPrefix(path, Prefix+"/") {
			fail(c, status, message)
			return
		}
		pages(c)
	}
}
