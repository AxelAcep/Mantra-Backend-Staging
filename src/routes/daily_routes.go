package routes

import (
	"mantra/src/controllers"
	"mantra/src/middleware"

	"github.com/labstack/echo/v4"
)

func ActivityRoutes(e *echo.Echo) {
	g := e.Group("/activity")

	g.POST("", controllers.CreateActivity, middleware.VerifyToken, middleware.AuthorizeRole(4))

	g.GET("/berjalan", controllers.GetActivityBerjalan, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.GET("/aktif", controllers.GetActivityAktif, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.GET("/pending", controllers.GetActivityPending, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.GET("/perlu-tindakan", controllers.GetActivityPerluTindakan, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.GET("/riwayat", controllers.GetActivityRiwayat, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.GET("/count", controllers.GetActivityCount, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.GET("/:id", controllers.GetDetailActivity, middleware.VerifyToken, middleware.AuthorizeRole(4))
	// update activity non-tanggal
	g.PATCH("/:id", controllers.UpdateActivity, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.POST("/:id/kolaborator", controllers.TambahKolaborator, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.POST("/:id/reschedule", controllers.PengajuanReschedule, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.PATCH("/reschedule/:rescheduleId/konfirmasi", controllers.KonfirmasiReschedule, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.POST("/:id/selesai", controllers.PengajuanSelesai, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.PATCH("/:id/konfirmasi-selesai", controllers.KonfirmasiSelesai, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.PATCH("/:id/kpi", controllers.UpdateActivityKPI, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/:id/chat", controllers.GetChat, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.POST("/:id/chat", controllers.KirimChat, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.PATCH("/:id/chat/read", controllers.ReadChat, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.GET("/:id/chat/unread", controllers.GetUnreadChatCount, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.GET("/chat/unread-total", controllers.GetTotalUnreadChatCount, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.GET("/chat/threads", controllers.GetChatThreads, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.GET("/konfirmasi-kolaborasi", controllers.GetActivityKonfirmasiKolaborasi, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.PATCH("/:id/konfirmasi-kolaborasi", controllers.KonfirmasiKolaborasi, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.POST("/:id/dokumen", controllers.UploadDokumen, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.DELETE("/:id/dokumen/:dokumenId", controllers.DeleteDokumen, middleware.VerifyToken, middleware.AuthorizeRole(4))

	g.GET("/master/reschedule", controllers.MasterGetReschedulePending, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/master/selesai", controllers.MasterGetActivitySelesai, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/master/aktif", controllers.MasterGetActivityAktif, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/master/riwayat", controllers.MasterGetActivitySemua, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/master/stats", controllers.MasterGetStats, middleware.VerifyToken, middleware.AuthorizeRole(1))

	g.GET("/master/:id", controllers.MasterGetActivityDetail, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/master/karyawan", controllers.MasterGetKaryawan, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/master/kpi/:pegawaiId", controllers.MasterGetDetailKPI, middleware.VerifyToken, middleware.AuthorizeRole(1))
	g.GET("/master/riwayat2", controllers.MasterGetActivityRiwayat, middleware.VerifyToken, middleware.AuthorizeRole(1))

	//	GET /master/reschedule          → MasterGetReschedulePending
	//
	// GET /master/activity/selesai    → MasterGetActivitySelesai
	// GET /master/activity/aktif      → MasterGetActivityAktif
	// GET /master/activity            → MasterGetActivitySemua
}
