package routes

import (
    "mantra/src/controllers"
    "mantra/src/middleware"
    "github.com/labstack/echo/v4"
)

func PerusahaanRoutes(e *echo.Echo) {
    e.GET("/perusahaan", controllers.GetPerusahaanList, middleware.VerifyToken, middleware.AuthorizeRole(4))
}
