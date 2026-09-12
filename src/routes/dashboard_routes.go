package routes

import (
	"mantra/src/controllers"
	"mantra/src/middleware"

	"github.com/labstack/echo/v4"
)

func DashboardRoutes(e *echo.Echo) {
	g := e.Group("/dashboard")

	g.GET("/pengadaan-summary", controllers.GetPengadaanSummary, middleware.VerifyToken, middleware.AuthorizeRole(1))
}
