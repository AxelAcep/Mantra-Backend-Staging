package routes

import (
	"mantra/src/controllers"
	"mantra/src/middleware"

	"github.com/labstack/echo/v4"
)

func BarangRoutes(e *echo.Echo) {
	g := e.Group("/barang")
	g.GET("", controllers.GetBarangList, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.POST("", controllers.CreateBarang, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PUT("/:id", controllers.UpdateBarang, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.DELETE("/:id", controllers.DeleteBarang, middleware.VerifyToken, middleware.AuthorizeRole(3))
}
