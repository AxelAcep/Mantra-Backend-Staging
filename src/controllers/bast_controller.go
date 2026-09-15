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

// Satu tracking bisa punya sampai 2 Bast (PAC & FIRE, tergantung Jenis
// Penawaran-nya — lihat models.DetectBastKategori), jadi endpoint di file ini
// selalu kerja dengan LIST Bast, bukan satu Bast tunggal kayak dulu.

// ── Helpers ─────────────────────────────────────────────────────────────────

func preloadBastList(trackingID string) ([]models.Bast, error) {
	var basts []models.Bast
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
		Order("created_at ASC").
		Find(&basts).Error

	if err != nil {
		return nil, err
	}
	return basts, nil
}

// findBastForEntryTarget nentuin Bast mana yang dituju buat operasi yang
// perlu tau kategori (mis. tambah entry baru): kalau tracking cuma punya 1
// Bast, langsung dipakai; kalau ada 2 (PAC & FIRE), kategori wajib disebut.
func findBastForEntryTarget(basts []models.Bast, kategoriParam string) (*models.Bast, error) {
	if len(basts) == 0 {
		return nil, fmt.Errorf("Data BAST tidak ditemukan.")
	}
	if len(basts) == 1 {
		return &basts[0], nil
	}
	if kategoriParam == "" {
		return nil, fmt.Errorf("Tracking ini punya lebih dari 1 BAST (PAC & FIRE) — sebutkan kategori BAST-nya.")
	}
	for i := range basts {
		if string(basts[i].Kategori) == strings.ToUpper(kategoriParam) {
			return &basts[i], nil
		}
	}
	return nil, fmt.Errorf("BAST kategori %s tidak ditemukan.", kategoriParam)
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
// Balikin SEMUA Bast tracking ini (1 kalau UMUM/cuma PAC/cuma FIRE, 2 kalau
// PAC & FIRE dua-duanya).

func GetDetailBast(c echo.Context) error {
	trackingID := c.Param("id")
	pegawaiID, _, roleStr, divisiStr, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	if !canViewStepForTracking(models.StepBAST, roleStr, divisiStr, pegawaiID, trackingID) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak."})
	}

	basts, err := preloadBastList(trackingID)
	if err != nil || len(basts) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data BAST tidak ditemukan."})
	}

	return c.JSON(http.StatusOK, basts)
}

// ── Create Entry ─────────────────────────────────────────────────────────────

func CreateBastEntry(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		Kategori           string `json:"kategori"` // wajib kalau tracking punya 2 BAST (PAC & FIRE)
		NoReferensi        string `json:"noReferensi"`
		TanggalTerbit      string `json:"tanggalTerbit"`
		TanggalSerahTerima string `json:"tanggalSerahTerima"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	basts, err := preloadBastList(trackingID)
	if err != nil || len(basts) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data BAST tidak ditemukan."})
	}

	bast, err := findBastForEntryTarget(basts, body.Kategori)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// No. Referensi auto-generated (BAST-[PAC/FIR-]YYMM-XXX-###) kalau gak
	// dikirim manual — biar konsisten walau entry ditambah manual dari FE.
	noReferensi := body.NoReferensi
	if strings.TrimSpace(noReferensi) == "" {
		kodePerusahaan := models.KodePerusahaanFromNama(bast.TrackingPenawaran.Perusahaan.Nama)
		now := time.Now()
		noReferensi = models.GenerateBastKode(config.DB, bast.Kategori, kodePerusahaan, now.Year(), int(now.Month()))
	}

	entry := models.BastEntry{
		ID:                 uuid.New().String(),
		BastID:             bast.ID,
		NoReferensi:        noReferensi,
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
		fmt.Sprintf("Entry BAST baru ditambahkan dengan No. Referensi '%s'.", noReferensi),
		pegawaiID,
		namaPegawai,
	)

	updated, _ := preloadBastList(trackingID)

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

	var entry models.BastEntry
	if err := config.DB.Where("id = ?", entryID).First(&entry).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Entry BAST tidak ditemukan."})
	}

	// Pastikan entry ini emang milik BAST punya tracking di URL (scoping/auth).
	var bast models.Bast
	if err := config.DB.
		Where("id = ? AND tracking_penawaran_id = ?", entry.BastID, trackingID).
		First(&bast).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data BAST tidak ditemukan."})
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

	appendBastLog(&bast, "Update Info BAST", keterangan, pegawaiID, namaPegawai)

	updated, _ := preloadBastList(trackingID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Detail BAST berhasil diperbarui.",
		"data":    updated,
	})
}
