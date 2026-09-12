package controllers

import (
	"mantra/src/config"
	"mantra/src/models"
	"net/http"

	"github.com/labstack/echo/v4"
)

type pengadaanSummaryResponse struct {
	RequestPenawaran   int `json:"requestPenawaran"`
	PenawaranApproval  int `json:"penawaranApproval"`
	PenawaranFinal     int `json:"penawaranFinal"`
	POAktif            int `json:"poAktif"`
	PenawaranPengadaan int `json:"penawaranPengadaan"`
	KonfirmasiSelesai  int `json:"konfirmasiSelesai"`
}

func GetPengadaanSummary(c echo.Context) error {
	db := config.DB
	var summary pengadaanSummaryResponse

	var count int64

	// 1. Request Penawaran → PERMINTAAN_MASUK
	db.Model(&models.TrackingPenawaran{}).
		Where(`"step_saat_ini" = ?`, models.StepPermintaanMasuk).
		Where(`"status" != ?`, models.StatusDibatalkan).
		Count(&count)
	summary.RequestPenawaran = int(count)

	// 2. Penawaran Approval → REVIEW_INTERNAL + PERSETUJUAN_MANAJEMEN
	db.Model(&models.TrackingPenawaran{}).
		Where(`"step_saat_ini" IN ?`, []string{
			string(models.StepReviewInternal),
			string(models.StepPersetujuanManajemen),
		}).
		Where(`"status" != ?`, models.StatusDibatalkan).
		Count(&count)
	summary.PenawaranApproval = int(count)

	// 3. Penawaran Final → FOLLOW_UP
	db.Model(&models.TrackingPenawaran{}).
		Where(`"step_saat_ini" = ?`, models.StepFollowUp).
		Where(`"status" != ?`, models.StatusDibatalkan).
		Count(&count)
	summary.PenawaranFinal = int(count)

	// 4. PO Aktif → IMPLEMENTASI
	db.Model(&models.TrackingPenawaran{}).
		Where(`"step_saat_ini" = ?`, models.StepImplementasi).
		Where(`"status" != ?`, models.StatusDibatalkan).
		Count(&count)
	summary.POAktif = int(count)

	// 5. Penawaran Pengadaan → BAST (reserved, return 0 for now)
	summary.PenawaranPengadaan = 0

	// 6. Konfirmasi Selesai → GARANSI (reserved, return 0 for now)
	summary.KonfirmasiSelesai = 0

	return c.JSON(http.StatusOK, summary)
}
