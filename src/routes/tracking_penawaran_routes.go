package routes

import (
	"mantra/src/controllers"
	"mantra/src/middleware"

	"github.com/labstack/echo/v4"
)

func TrackingPenawaranRoutes(e *echo.Echo) {
	g := e.Group("/tracking-penawaran")

	// ── 1. STATIC ROUTES ─────────────────────────────────────────────────────
	g.GET("", controllers.GetTrackingPenawaranList, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.GET("/aktif", controllers.GetTrackingPenawaranAktif, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.POST("", controllers.CreateTrackingPenawaran, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.GET("/mo/all", controllers.GetTrackingPenawaranMO, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/pegawai", controllers.GetPegawaiByDivisi, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// ── 2. CHAT STATIC ───────────────────────────────────────────────────────
	g.GET("/chat/unread-total", controllers.GetTotalUnreadPenawaranChatCount, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/chat/:chatId", controllers.UpdatePenawaranChat, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// ── 3. DOKUMEN STATIC ────────────────────────────────────────────────────
	g.POST("/dokumen/upload", controllers.UploadPenawaranDokumen, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.DELETE("/dokumen/:dokumenId", controllers.DeletePenawaranDokumen, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// ── 4. PERMINTAAN MASUK ──────────────────────────────────────────────────
	g.POST("/permintaan-masuk/:id/dokumen", controllers.UploadPenawaranDokumen, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.DELETE("/permintaan-masuk/:id/dokumen/:dokumenId", controllers.DeletePenawaranDokumen, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// ── 5. DYNAMIC NESTED ROUTES (Prioritaskan di atas /:id) ──────────────────
	// Chat Dynamic
	g.GET("/:id/chat", controllers.GetPenawaranChat, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.POST("/:id/chat", controllers.KirimPenawaranChat, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/chat/read", controllers.ReadPenawaranChat, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.GET("/:id/chat/unread", controllers.GetUnreadPenawaranChatCount, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/marketing", controllers.AssignMarketing, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// BOQ
	g.GET("/:id/boq", controllers.GetDetailBoQ, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.PATCH("/:id/boq/subtotal", controllers.UpdateSubTotalBoQ, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.POST("/:id/boq/dokumen", controllers.UploadDokumenBoQ, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.DELETE("/:id/boq/dokumen/:dokumenId", controllers.DeleteDokumenBoQ, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/boq/status", controllers.UpdateStatusBoQ, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// Review Internal
	g.GET("/:id/review-internal", controllers.GetDetailReviewInternal, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.POST("/:id/review-internal/dokumen", controllers.UploadDokumenReviewInternal, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.DELETE("/:id/review-internal/dokumen/:dokumenId", controllers.DeleteDokumenReviewInternal, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/review-internal/status", controllers.UpdateStatusReviewInternal, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// Persetujuan Manajemen
	g.GET("/:id/persetujuan-manajemen", controllers.GetDetailPersetujuanManajemen, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.POST("/:id/persetujuan-manajemen/dokumen", controllers.UploadDokumenPersetujuanManajemen, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.DELETE("/:id/persetujuan-manajemen/dokumen/:dokumenId", controllers.DeleteDokumenPersetujuanManajemen, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/persetujuan-manajemen/status", controllers.UpdateStatusPersetujuanManajemen, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// Follow Up (Step 5)
	g.GET("/:id/follow-up", controllers.GetDetailFollowUp, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.POST("/:id/follow-up/dokumen", controllers.UploadDokumenFollowUp, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.DELETE("/:id/follow-up/dokumen/:dokumenId", controllers.DeleteDokumenFollowUp, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/follow-up/status", controllers.UpdateStatusFollowUp, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// Accounting
	g.GET("/:id/accounting", controllers.GetAccounting, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.POST("/:id/accounting", controllers.CreateAccounting, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/accounting", controllers.UpdateAccounting, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/accounting/item/:itemId/bayar", controllers.BayarItemTermin, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// Implementasi (Step 6)
	g.GET("/:id/implementasi", controllers.GetDetailImplementasi, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/implementasi", controllers.UpdateDetailImplementasi, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.POST("/:id/implementasi/barang", controllers.AddBarangImplementasi, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/implementasi/barang/:barangId", controllers.UpdateBarangImplementasi, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.DELETE("/:id/implementasi/barang/:barangId", controllers.DeleteBarangImplementasi, middleware.VerifyToken, middleware.AuthorizeRole(3))

	// ── 6. DYNAMIC GENERAL /:id (Taruh paling bawah di grup ini) ────────────
	g.GET("/:id", controllers.GetDetailTrackingPenawaran, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/detail", controllers.UpdateDetailTrackingPenawaran, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PATCH("/:id/presales", controllers.AssignPreSales, middleware.VerifyToken, middleware.AuthorizeRole(2))
	g.PATCH("/:id/status", controllers.UpdateStatusPermintaanMasuk, middleware.VerifyToken, middleware.AuthorizeRole(3))
}
