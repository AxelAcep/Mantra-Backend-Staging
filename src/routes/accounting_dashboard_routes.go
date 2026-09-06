package routes

import (
	"mantra/src/controllers"
	"mantra/src/middleware"

	"github.com/labstack/echo/v4"
)

// Dashboard Accounting — menu sidebar terpisah, monitoring termin pembayaran
// lintas semua PO (bukan tab per-PO yang udah ada di /tracking-penawaran/:id/accounting).
func AccountingDashboardRoutes(e *echo.Echo) {
	g := e.Group("/accounting")

	g.GET("/summary", controllers.GetAccountingSummary, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.GET("/po", controllers.GetAccountingPOList, middleware.VerifyToken, middleware.AuthorizeRole(3))
}
