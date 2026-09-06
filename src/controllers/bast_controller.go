package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

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
		Preload("Entries", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Preload("Entries.ActivityAdminProyek.Pegawai").
		Preload("Entries.ActivityAdminProyek.Dokumen").
		Preload("Entries.ActivityAdminProyek.Dokumen.Pegawai").
		Preload("Entries.ActivityAdminProyek.Children").
		Preload("Entries.ActivityAdminProyek.Children.Pegawai").
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
	// Pakai Select+Updates (bukan Update kolom tunggal) supaya serializer:json
	// ke-apply — Update(column, value) langsung ngirim slice mentah ke driver
	// dan bikin Postgres error "could not determine data type of parameter $1".
	config.DB.Model(bast).Select("log_aktivitas").Updates(models.Bast{LogAktivitas: bast.LogAktivitas})
}

// ── Get Detail ──────────────────────────────────────────────────────────────

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

// ── Create Entry ─────────────────────────────────────────────────────────────

func CreateBastEntry(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		NoReferensi        string `json:"noReferensi"`
		TanggalTerbit      string `json:"tanggalTerbit"`
		TanggalSerahTerima string `json:"tanggalSerahTerima"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	bast, err := preloadBast(trackingID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data BAST tidak ditemukan."})
	}

	entry := models.BastEntry{
		ID:                 uuid.New().String(),
		BastID:             bast.ID,
		NoReferensi:        body.NoReferensi,
		TanggalTerbit:      parseDate(body.TanggalTerbit),
		TanggalSerahTerima: parseDate(body.TanggalSerahTerima),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := config.DB.Create(&entry).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menambahkan entry BAST."})
	}

	appendBastLog(
		bast,
		"Tambah Entry BAST",
		fmt.Sprintf("Entry BAST baru ditambahkan dengan No. Referensi '%s'.", body.NoReferensi),
		pegawaiID,
		namaPegawai,
	)

	updated, _ := preloadBast(trackingID)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "Entry BAST berhasil ditambahkan.",
		"data":    updated,
	})
}

// ── Update Entry ──────────────────────────────────────────────────────────────

func UpdateDetailBast(c echo.Context) error {
	trackingID := c.Param("id")
	entryID := c.Param("entryId")

	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		NoReferensi        string `json:"noReferensi"`
		TanggalTerbit      string `json:"tanggalTerbit"`
		TanggalSerahTerima string `json:"tanggalSerahTerima"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	bast, err := preloadBast(trackingID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data BAST tidak ditemukan."})
	}

	var entry models.BastEntry
	if err := config.DB.
		Where("id = ? AND bast_id = ?", entryID, bast.ID).
		First(&entry).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Entry BAST tidak ditemukan."})
	}

	oldNoReferensi := entry.NoReferensi

	entry.NoReferensi = body.NoReferensi
	entry.TanggalTerbit = parseDate(body.TanggalTerbit)
	entry.TanggalSerahTerima = parseDate(body.TanggalSerahTerima)
	entry.UpdatedAt = time.Now()

	if err := config.DB.Save(&entry).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengupdate detail BAST."})
	}

	var changes []string
	if oldNoReferensi != body.NoReferensi {
		changes = append(changes, fmt.Sprintf(
			"No. Referensi diubah dari '%s' menjadi '%s'",
			oldNoReferensi, body.NoReferensi,
		))
	}

	keterangan := "Update Detail BAST"
	if len(changes) > 0 {
		keterangan = strings.Join(changes, ", ")
	}

	appendBastLog(bast, "Update Info BAST", keterangan, pegawaiID, namaPegawai)

	updated, _ := preloadBast(trackingID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Detail BAST berhasil diperbarui.",
		"data":    updated,
	})
}