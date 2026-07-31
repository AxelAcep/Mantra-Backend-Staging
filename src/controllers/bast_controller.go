package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"mantra/src/config"
	"mantra/src/models"
)

// ── Helpers ─────────────────────────────────────────────────────────────────

func preloadBast(trackingID string) (*models.Bast, error) {
	var bast models.Bast
	err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Preload("TrackingPenawaran.Perusahaan").
		Preload("TrackingPenawaran.Marketing").
		Preload("ActivityAdminProyek.Pegawai").
		Preload("ActivityAdminProyek.Dokumen").
		Preload("ActivityAdminProyek.Dokumen.Pegawai").
		Preload("ActivityAdminProyek.Children").
		Preload("ActivityAdminProyek.Children.Pegawai").
		First(&bast).Error

	if err != nil {
		return nil, err
	}
	return &bast, nil
}

func appendBastLog(bast *models.Bast, aksi, keterangan, pegawaiID, namaPegawai string) {
	log := models.LogBast{
		Aksi:        aksi,
		Keterangan:  keterangan,
		PegawaiID:   pegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   time.Now(),
	}
	bast.LogAktivitas = append(bast.LogAktivitas, log)
	config.DB.Model(bast).Update("log_aktivitas", bast.LogAktivitas)
}

// ──────────────────────────────────────────────────────────────────────────────────────────────────

func GetDetailBast(c echo.Context) error {
	trackingID := c.Param("id")
	_, _, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	bast, err := preloadBast(trackingID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data BAST tidak ditemukan."})
	}

	return c.JSON(http.StatusOK, bast)
}

// ──────────────────────────────────────────────────────────────────────────────────────────────────

func UpdateDetailBast(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Unauthorized.",
		})
	}

	var body struct {
		NoReferensi        string `json:"noReferensi"`
		TanggalTerbit      string `json:"tanggalTerbit"`
		TanggalSerahTerima string `json:"tanggalSerahTerima"`
	}

	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid body.",
		})
	}

	bast, err := preloadBast(trackingID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Data BAST tidak ditemukan.",
		})
	}

	oldNoReferensi := bast.NoReferensi

	// Update detail BAST
	bast.NoReferensi = body.NoReferensi
	bast.TanggalTerbit = parseDate(body.TanggalTerbit)
	bast.TanggalSerahTerima = parseDate(body.TanggalSerahTerima)
	bast.UpdatedAt = time.Now()

	// Save BAST
	if err := config.DB.Save(bast).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Gagal mengupdate detail BAST.",
		})
	}

	// Catat perubahan
	var changes []string

	if oldNoReferensi != body.NoReferensi {
		changes = append(
			changes,
			fmt.Sprintf(
				"No. Referensi diubah dari '%s' menjadi '%s'",
				oldNoReferensi,
				body.NoReferensi,
			),
		)
	}

	keterangan := "Update Detail BAST"
	if len(changes) > 0 {
		keterangan = strings.Join(changes, ", ")
	}

	appendBastLog(
		bast,
		"Update Info BAST",
		keterangan,
		pegawaiID,
		namaPegawai,
	)

	updated, _ := preloadBast(trackingID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Detail BAST berhasil diperbarui.",
		"data":    updated,
	})
}