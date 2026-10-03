package routes

import (
	"mantra/src/controllers"
	"mantra/src/middleware"

	"github.com/labstack/echo/v4"
)

func BarangRoutes(e *echo.Echo) {
	g := e.Group("/barang")
	// Level 4: master barang adalah data referensi non-sensitif (noBarang,
	// deskripsi, satuan) yang dibutuhkan autocomplete "Tambah Barang" di
	// step implementasi — karyawan PROCUREMENT_GA yang baru kami buka
	// aksesnya ke tabel pembelian ikut butuh ini. Write tetap level 3.
	g.GET("", controllers.GetBarangList, middleware.VerifyToken, middleware.AuthorizeRole(4))
	g.POST("", controllers.CreateBarang, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.PUT("/:id", controllers.UpdateBarang, middleware.VerifyToken, middleware.AuthorizeRole(3))
	g.DELETE("/:id", controllers.DeleteBarang, middleware.VerifyToken, middleware.AuthorizeRole(3))
}
