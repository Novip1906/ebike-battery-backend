package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const Prefix = "/api"

func RegisterRoutes(router *gin.Engine, motorModes *MotorModeHandler, riders *RiderHandler, authMiddleware *AuthMiddleware) {
	routes := router.Group(Prefix)
	routes.POST("/riders/register", riders.RegisterRider)
	routes.POST("/riders/login", riders.LoginRider)

	guest := routes.Group("", authMiddleware.OptionalRider)
	guest.GET("/motor_modes", motorModes.ListMotorModes)
	guest.GET("/motor_modes/feed", motorModes.GetMotorModeFeed)
	guest.GET("/motor_modes/feed/:motor_mode_id", motorModes.GetMotorModeFeedByID)

	rider := routes.Group("", authMiddleware.RequireRider)
	rider.GET("/motor_modes/draft", motorModes.GetMotorModeDraft)
	rider.POST("/motor_modes", motorModes.CreateMotorMode)
	rider.PUT("/motor_modes/:motor_mode_id/publish", motorModes.PublishMotorMode)
	rider.DELETE("/motor_modes/:motor_mode_id", motorModes.DeleteMotorMode)
	rider.POST("/motor_modes/:motor_mode_id/like", motorModes.LikeMotorMode)
	rider.POST("/riders/logout", riders.LogoutRider)
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
