package routes

import (
	"mantra/src/controllers"
	"mantra/src/middleware"

	"github.com/labstack/echo/v4"
)

func KPIRoutes(e *echo.Echo) {
	kpi := e.Group("/kpi")

	kpi.POST("/:pegawaiId", controllers.AddKPI, middleware.VerifyToken, middleware.AuthorizeRole(1))                    // 1
	kpi.GET("", controllers.GetAllKPIBulan, middleware.VerifyToken, middleware.AuthorizeRole(1))                        // 2
	kpi.GET("/yearly", controllers.GetAllKPIYearly, middleware.VerifyToken, middleware.AuthorizeRole(1))                // 3
	kpi.GET("/distribusi", controllers.GetDistribusiKPIBulan, middleware.VerifyToken, middleware.AuthorizeRole(1))      // 6
	kpi.GET("/:pegawaiId", controllers.GetKPIPegawaiBulan, middleware.VerifyToken, middleware.AuthorizeRole(1))         // 4
	kpi.GET("/:pegawaiId/yearly", controllers.GetKPIPegawaiYearly, middleware.VerifyToken, middleware.AuthorizeRole(1)) // 5
	kpi.GET("/overview/:pegawaiId", controllers.GetKPIOverview, middleware.VerifyToken, middleware.AuthorizeRole(1))

}
