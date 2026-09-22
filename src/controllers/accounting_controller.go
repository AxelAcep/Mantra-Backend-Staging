package controllers

import (
	"math"
	"net/http"
	"time"

	"mantra/src/config"
	"mantra/src/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func hitungTotalPersentase(items []models.ItemTermin) float64 {
	total := 0.0
	for _, item := range items {
		total += item.Persentase
	}
	return total
}

// ─── GET /tracking-penawaran/:id/accounting ───────────────────────────────────

func GetAccounting(c echo.Context) error {
	trackingID := c.Param("id")

	_, roleStr, divisiStr, ok := getStepAccessClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	if !canViewStep(models.StepPembayaran, roleStr, divisiStr) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak."})
	}

	var termin models.TerminPembayaran
	err := config.DB.
		Preload("Items", func(db *gorm.DB) *gorm.DB {
			return db.Order("index ASC")
		}).
		Preload("Items.Activity.Pegawai").
		Preload("Items.Activity.Dokumen").
		Preload("Items.Activity.Dokumen.Pegawai").
		Where("tracking_penawaran_id = ?", trackingID).
		First(&termin).Error

	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data accounting tidak ditemukan"})
	}

	now := time.Now()
	oneWeek := now.Add(7 * 24 * time.Hour)
	twoWeeks := now.Add(14 * 24 * time.Hour)

	type ItemWithFlag struct {
		models.ItemTermin
		Flag string `json:"flag"` // "LEWAT", "1_MINGGU", "2_MINGGU", ""
	}

	enriched := make([]ItemWithFlag, len(termin.Items))
	for i, item := range termin.Items {
		flag := ""
		if item.Deadline != nil && !item.SudahDibayar {
			if item.Deadline.Before(now) {
				flag = "LEWAT"
			} else if item.Deadline.Before(oneWeek) {
				flag = "1_MINGGU"
			} else if item.Deadline.Before(twoWeeks) {
				flag = "2_MINGGU"
			}
		}
		enriched[i] = ItemWithFlag{ItemTermin: item, Flag: flag}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":                  termin.ID,
		"trackingPenawaranId": termin.TrackingPenawaranID,
		"status":              termin.Status,
		"items":               enriched,
		"createdAt":           termin.CreatedAt,
		"updatedAt":           termin.UpdatedAt,
	})
}

// ─── POST /tracking-penawaran/:id/accounting ──────────────────────────────────

type ItemTerminInput struct {
	NamaTermin string     `json:"namaTermin"`
	Persentase float64    `json:"persentase"`
	Keterangan *string    `json:"keterangan"`
	Deadline   *time.Time `json:"deadline"`
}

func CreateAccounting(c echo.Context) error {
	trackingID := c.Param("id")

	_, roleStr, divisiStr, ok := getStepAccessClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	if !canViewStep(models.StepPembayaran, roleStr, divisiStr) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak."})
	}

	// Cek persetujuan manajemen sudah DONE
	var persetujuan models.PersetujuanManajemen
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&persetujuan).Error; err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Persetujuan manajemen belum ada"})
	}
	if persetujuan.Status != models.StatusSelesai {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Persetujuan manajemen belum selesai"})
	}

	// Cek belum ada termin
	var existing models.TerminPembayaran
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&existing).Error; err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "Accounting sudah dibuat, gunakan endpoint edit"})
	}

	var input struct {
		Items []ItemTerminInput `json:"items"`
	}
	if err := c.Bind(&input); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	if len(input.Items) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Minimal 1 item termin"})
	}
	if len(input.Items) > 100 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Maksimal 100 item termin"})
	}

	total := 0.0
	for _, item := range input.Items {
		total += item.Persentase
	}
	if math.Abs(total-100.0) > 0.01 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Total persentase harus 100%"})
	}

	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)
	if pegawaiID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Pegawai tidak ditemukan"})
	}

	var tracking models.TrackingPenawaran
	if err := config.DB.Where("id = ?", trackingID).First(&tracking).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Tracking penawaran tidak ditemukan"})
	}

	termin := models.TerminPembayaran{
		ID:                  uuid.New().String(),
		TrackingPenawaranID: trackingID,
		CreatedBy:           pegawaiID,
		Status:              models.StatusOnProgress,
	}

	items := make([]models.ItemTermin, len(input.Items))
	for i, inp := range input.Items {
		items[i] = models.ItemTermin{
			ID:                 uuid.New().String(),
			TerminPembayaranID: termin.ID,
			Index:              i + 1,
			NamaTermin:         inp.NamaTermin,
			Persentase:         inp.Persentase,
			Keterangan:         inp.Keterangan,
			Deadline:           inp.Deadline,
		}
	}
	termin.Items = items

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&termin).Error; err != nil {
			return err
		}

		// Termin berjalan berurutan mirip bulan Garansi — cuma termin
		// pertama yang langsung dapet daily, termin berikutnya nyusul
		// otomatis (AdvanceTerminIfReady) setelah termin sebelumnya tuntas.
		return models.CreateItemTerminActivity(tx, &termin.Items[0], tracking.NomorPenawaran)
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, termin)
}

// ─── PATCH /tracking-penawaran/:id/accounting ────────────────────────────────
// Replace semua items (kirim ulang full list)

func UpdateAccounting(c echo.Context) error {
	trackingID := c.Param("id")

	_, roleStr, divisiStr, ok := getStepAccessClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	if !canViewStep(models.StepPembayaran, roleStr, divisiStr) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak."})
	}

	var termin models.TerminPembayaran
	if err := config.DB.
		Preload("Items").
		Where("tracking_penawaran_id = ?", trackingID).
		First(&termin).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data accounting tidak ditemukan"})
	}

	// Begitu ada termin yang beneran udah kelar (dibayar atau daily-nya
	// disetujui), gak boleh replace total daftar termin-nya lagi — bisa bikin
	// progress yang udah kejadian jadi nyangkut ke termin yang salah. Selama
	// belum ada progress beneran (termin-1 masih ON_PROGRESS daily-nya),
	// tetep boleh dikoreksi.
	for _, it := range termin.Items {
		if it.SudahDibayar || it.ActivitySelesai {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Ada termin yang sudah berjalan (dibayar/disetujui), daftar termin tidak bisa diubah lagi.",
			})
		}
	}

	var input struct {
		Items []ItemTerminInput `json:"items"`
	}
	if err := c.Bind(&input); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	if len(input.Items) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Minimal 1 item termin"})
	}
	if len(input.Items) > 100 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Maksimal 100 item termin"})
	}

	total := 0.0
	for _, item := range input.Items {
		total += item.Persentase
	}
	if math.Abs(total-100.0) > 0.01 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Total persentase harus 100%"})
	}

	// Daily termin-1 (kalau udah sempet dibuat pas create, masih ON_PROGRESS/
	// belum disetujui — dijamin sama guard di atas) dipertahankan biar gak
	// yatim piatu setelah list termin di-replace.
	var existingActivityIDTermin1 *string
	for _, it := range termin.Items {
		if it.Index == 1 {
			existingActivityIDTermin1 = it.ActivityID
			break
		}
	}

	// Hapus items lama, insert baru
	if err := config.DB.Where("termin_pembayaran_id = ?", termin.ID).Delete(&models.ItemTermin{}).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	newItems := make([]models.ItemTermin, len(input.Items))
	for i, inp := range input.Items {
		newItems[i] = models.ItemTermin{
			ID:                 uuid.New().String(),
			TerminPembayaranID: termin.ID,
			Index:              i + 1,
			NamaTermin:         inp.NamaTermin,
			Persentase:         inp.Persentase,
			Keterangan:         inp.Keterangan,
			Deadline:           inp.Deadline,
		}
		if i == 0 {
			newItems[i].ActivityID = existingActivityIDTermin1
		}
	}

	if err := config.DB.Create(&newItems).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	termin.Items = newItems
	return c.JSON(http.StatusOK, termin)
}

// ─── PATCH /tracking-penawaran/:id/accounting/item/:itemId/bayar ─────────────

func BayarItemTermin(c echo.Context) error {
	itemID := c.Param("itemId")

	_, roleStr, divisiStr, ok := getStepAccessClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	if !canViewStep(models.StepPembayaran, roleStr, divisiStr) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak."})
	}

	var item models.ItemTermin
	if err := config.DB.First(&item, "id = ?", itemID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Item tidak ditemukan"})
	}

	if item.SudahDibayar {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Termin ini sudah dibayar."})
	}

	// Belum ada daily-nya sama sekali = termin ini belum "mulai" (masih
	// nunggu termin sebelumnya tuntas) — belum bisa ditandai dibayar.
	if item.ActivityID == nil || *item.ActivityID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Termin ini belum berjalan — selesaikan termin sebelumnya (dibayar + daily disetujui) dulu.",
		})
	}

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		item.SudahDibayar = true
		item.TanggalDibayar = &now

		if err := tx.Model(&models.ItemTermin{}).Where("id = ?", item.ID).
			Select("sudah_dibayar", "tanggal_dibayar").
			Updates(models.ItemTermin{
				SudahDibayar:   item.SudahDibayar,
				TanggalDibayar: item.TanggalDibayar,
			}).Error; err != nil {
			return err
		}

		return models.AdvanceTerminIfReady(tx, &item)
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, item)
}
