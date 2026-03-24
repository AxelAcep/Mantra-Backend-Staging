package routes

import (
	"mantra/src/controllers"
	"mantra/src/middleware"

	"github.com/labstack/echo/v4"
)

func UserRoutes(e *echo.Echo) {
	g := e.Group("/user")

	g.POST("/register", controllers.Register, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.POST("/login", middleware.LoginRateLimiter(controllers.Login))

	g.GET("", controllers.GetAllUsers, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/:id", controllers.GetOneUser, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.PUT("/:id", controllers.EditUser, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.DELETE("/:id", controllers.DeleteUser, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/all", controllers.GetAllPegawai, middleware.VerifyToken, middleware.AuthorizeRole(4))
}
