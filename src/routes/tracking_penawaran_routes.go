package routes

import (
	"mantra/src/controllers"
	"mantra/src/middleware"

	"github.com/labstack/echo/v4"
)

func TrackingPenawaranRoutes(e *echo.Echo) {
	g := e.Group("/tracking-penawaran")

	// ── 1. STATIC ROUTES — harus paling atas ─────────────────────────────────

	g.GET("", controllers.GetTrackingPenawaranList, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.POST("", controllers.CreateTrackingPenawaran, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.GET("/mo/all", controllers.GetTrackingPenawaranMO, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/pegawai", controllers.GetPegawaiByDivisi, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// ── 2. CHAT STATIC — harus sebelum /:id ──────────────────────────────────
	g.GET("/chat/unread-total", controllers.GetTotalUnreadPenawaranChatCount, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/chat/:chatId", controllers.UpdatePenawaranChat, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// ── 3. DOKUMEN STATIC — harus sebelum /:id ───────────────────────────────
	g.POST("/dokumen/upload", controllers.UploadPenawaranDokumen, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.DELETE("/dokumen/:dokumenId", controllers.DeletePenawaranDokumen, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// ── 4. PERMINTAAN MASUK DOKUMEN ───────────────────────────────────────────
	g.POST("/permintaan-masuk/:id/dokumen", controllers.UploadPenawaranDokumen, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.DELETE("/permintaan-masuk/:id/dokumen/:dokumenId", controllers.DeletePenawaranDokumen, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// ── 5. DYNAMIC /:id — harus paling bawah ─────────────────────────────────
	g.GET("/:id", controllers.GetDetailTrackingPenawaran, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/detail", controllers.UpdateDetailTrackingPenawaran, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/presales", controllers.AssignPreSales, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.PATCH("/:id/status", controllers.UpdateStatusPermintaanMasuk, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// ── 6. CHAT DYNAMIC — harus setelah static chat ───────────────────────────
	g.GET("/:id/chat", controllers.GetPenawaranChat, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.POST("/:id/chat", controllers.KirimPenawaranChat, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/chat/read", controllers.ReadPenawaranChat, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.GET("/:id/chat/unread", controllers.GetUnreadPenawaranChatCount, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/marketing", controllers.AssignMarketing, middleware.VerifyToken, middleware.AuthorizeRole(3))
}
