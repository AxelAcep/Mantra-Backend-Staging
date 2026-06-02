package routes

import (
	"mantra/src/controllers"
	"mantra/src/middleware"

	"github.com/labstack/echo/v4"
)

func PerusahaanRoutes(e *echo.Echo) {
	e.GET("/perusahaan", controllers.GetPerusahaanList, middleware.VerifyToken, middleware.AuthorizeRole(4))
	e.GET("/perusahaan/:id", controllers.GetPerusahaanDetail, middleware.VerifyToken, middleware.AuthorizeRole(4))
	e.POST("/perusahaan", controllers.CreatePerusahaan, middleware.VerifyToken, middleware.AuthorizeRole(4))
	e.PUT("/perusahaan/:id", controllers.UpdatePerusahaan, middleware.VerifyToken, middleware.AuthorizeRole(4))
}
