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

// Satu tracking bisa punya sampai 2 Garansi (PAC & FIRE, tergantung Bast yang
// men-trigger-nya — lihat models.DetectBastKategori), jadi endpoint di file
// ini selalu kerja dengan LIST Garansi, bukan satu Garansi tunggal.

// ── Helpers ─────────────────────────────────────────────────────────────────

func preloadGaransiList(trackingID string) ([]models.Garansi, error) {
	var garansis []models.Garansi
	err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Preload("TrackingPenawaran.Perusahaan").
		Preload("TrackingPenawaran.Marketing").
		Preload("Bast").
		Preload("PIC").
		Preload("Months", func(db *gorm.DB) *gorm.DB {
			return db.Order("bulan_ke ASC")
		}).
		Preload("Months.Activity.Pegawai").
		Preload("Months.Activity.Dokumen").
		Preload("Months.Activity.Dokumen.Pegawai").
		Order("created_at ASC").
		Find(&garansis).Error

	if err != nil {
		return nil, err
	}
	return garansis, nil
}

// findGaransiForTarget nentuin Garansi mana yang dituju: kalau tracking cuma
// punya 1 Garansi, langsung dipakai; kalau ada 2 (PAC & FIRE), kategoriBast
// wajib disebut.
func findGaransiForTarget(garansis []models.Garansi, kategoriBastParam string) (*models.Garansi, error) {
	if len(garansis) == 0 {
		return nil, fmt.Errorf("Data Garansi tidak ditemukan.")
	}
	if len(garansis) == 1 {
		return &garansis[0], nil
	}
	if kategoriBastParam == "" {
		return nil, fmt.Errorf("Tracking ini punya lebih dari 1 Garansi (PAC & FIRE) — sebutkan kategori Garansi-nya.")
	}
	for i := range garansis {
		if string(garansis[i].KategoriBast) == strings.ToUpper(kategoriBastParam) {
			return &garansis[i], nil
		}
	}
	return nil, fmt.Errorf("Garansi kategori %s tidak ditemukan.", kategoriBastParam)
}

// ── Get Detail (tracking status per bulan, logbook, dokumen pendukung) ───────
// Balikin SEMUA Garansi tracking ini (1 kalau UMUM/cuma PAC/cuma FIRE, 2
// kalau PAC & FIRE dua-duanya).

func GetDetailGaransi(c echo.Context) error {
	trackingID := c.Param("id")
	pegawaiID, _, roleStr, divisiStr, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	if !canViewStepForTracking(models.StepGaransi, roleStr, divisiStr, pegawaiID, trackingID) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak."})
	}

	garansis, err := preloadGaransiList(trackingID)
	if err != nil || len(garansis) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Garansi tidak ditemukan."})
	}

	return c.JSON(http.StatusOK, garansis)
}

// ── Konfigurasi Timeline (input kategori dalam/luar kota + lama tahun + bulan/tahun mulai) ──

func KonfigurasiGaransi(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		KategoriBast    string                 `json:"kategoriBast"` // wajib kalau tracking punya 2 Garansi
		KategoriGaransi models.KategoriGaransi `json:"kategoriGaransi"`
		LamaTahun       int                    `json:"lamaTahun"`
		BulanMulai      int                    `json:"bulanMulai"`
		TahunMulai      int                    `json:"tahunMulai"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	garansis, err := preloadGaransiList(trackingID)
	if err != nil || len(garansis) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Garansi tidak ditemukan."})
	}

	garansi, err := findGaransiForTarget(garansis, body.KategoriBast)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Opsi kategori garansi dibatasin sesuai KategoriBast Garansi ini (PAC ->
	// dalam/luar kota PAC, FIRE -> dalam/luar kota FIRE, UMUM -> generik) —
	// "Tidak Ada" selalu boleh dipilih di kategori manapun sebagai override.
	allowed := models.AllowedKategoriGaransi(garansi.KategoriBast)
	isValid := false
	for _, k := range allowed {
		if k == body.KategoriGaransi {
			isValid = true
			break
		}
	}
	if !isValid {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Kategori garansi tidak valid untuk Bast kategori ini."})
	}

	if body.KategoriGaransi != models.KategoriTidakAda {
		if body.LamaTahun <= 0 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Lama tahun garansi wajib diisi."})
		}
		if body.BulanMulai < 1 || body.BulanMulai > 12 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Bulan mulai tidak valid."})
		}
		if body.TahunMulai <= 0 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Tahun mulai wajib diisi."})
		}
	}

	if garansi.Status != models.StatusGaransiBelumDikonfigurasi {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Garansi sudah dikonfigurasi sebelumnya."})
	}

	if body.KategoriGaransi == models.KategoriTidakAda {
		err := config.DB.Transaction(func(tx *gorm.DB) error {
			now := time.Now()
			garansi.KategoriGaransi = models.KategoriTidakAda
			garansi.Status = models.StatusGaransiSelesai
			garansi.LogAktivitas = append(garansi.LogAktivitas, models.LogGaransi{
				Aksi:        "Tidak Ada Garansi",
				Keterangan:  "Pengadaan ini tidak memiliki garansi.",
				PegawaiID:   pegawaiID,
				NamaPegawai: namaPegawai,
				CreatedAt:   now,
			})
			garansi.UpdatedAt = now
			return tx.Model(&models.Garansi{}).Where("id = ?", garansi.ID).
				Select("kategori_garansi", "status", "log_aktivitas", "updated_at").
				Updates(garansi).Error
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengkonfigurasi garansi."})
		}
		updated, _ := preloadGaransiList(trackingID)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"message": "Garansi dikonfigurasi sebagai tidak ada garansi.",
			"data":    updated,
		})
	}

	_, bulanOffsets := models.KategoriSlotPattern(body.KategoriGaransi)

	err = config.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		bulanKe := 0

		for tahun := 0; tahun < body.LamaTahun; tahun++ {
			for _, offset := range bulanOffsets {
				bulanKe++
				bulan := body.BulanMulai + offset - 1 + tahun*12
				tahunHitung := body.TahunMulai
				for bulan > 12 {
					bulan -= 12
					tahunHitung++
				}

				month := models.GaransiMonth{
					ID:        uuid.New().String(),
					GaransiID: garansi.ID,
					BulanKe:   bulanKe,
					Bulan:     bulan,
					Tahun:     tahunHitung,
					Status:    models.StatusPending,
					CreatedAt: now,
					UpdatedAt: now,
				}
				if err := tx.Create(&month).Error; err != nil {
					return err
				}

				if bulanKe == 1 {
					if err := models.CreateGaransiMonthActivity(tx, &month, garansi.PICID, pegawaiID, namaPegawai); err != nil {
						return err
					}
				}
			}
		}

		garansi.KategoriGaransi = body.KategoriGaransi
		garansi.LamaTahun = &body.LamaTahun
		garansi.BulanMulai = &body.BulanMulai
		garansi.TahunMulai = &body.TahunMulai
		garansi.Status = models.StatusGaransiOnProgress
		garansi.LogAktivitas = append(garansi.LogAktivitas, models.LogGaransi{
			Aksi:        "Konfigurasi Garansi",
			Keterangan:  fmt.Sprintf("Kategori %s, timeline %d tahun (%d kunjungan) dikonfigurasi, mulai %02d/%d", body.KategoriGaransi, body.LamaTahun, bulanKe, body.BulanMulai, body.TahunMulai),
			PegawaiID:   pegawaiID,
			NamaPegawai: namaPegawai,
			CreatedAt:   now,
		})
		garansi.UpdatedAt = now

		return tx.Model(&models.Garansi{}).Where("id = ?", garansi.ID).
			Select("kategori_garansi", "lama_tahun", "bulan_mulai", "tahun_mulai", "status", "log_aktivitas", "updated_at").
			Updates(garansi).Error
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengkonfigurasi timeline Garansi."})
	}

	updated, _ := preloadGaransiList(trackingID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Timeline Garansi berhasil dikonfigurasi.",
		"data":    updated,
	})
}

// ── Update Tanggal Kunjungan per Bulan ───────────────────────────────────────
// monthId udah cukup buat nemuin Garansi mana (GaransiMonth->GaransiID), jadi
// gak perlu parameter kategoriBast di sini.

func UpdateTanggalKunjunganGaransi(c echo.Context) error {
	trackingID := c.Param("id")
	monthID := c.Param("monthId")

	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		TanggalKunjungan string `json:"tanggalKunjungan"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	tanggal := parseDate(body.TanggalKunjungan)
	if tanggal == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Tanggal kunjungan tidak valid."})
	}

	var month models.GaransiMonth
	if err := config.DB.Where("id = ?", monthID).First(&month).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Bulan Garansi tidak ditemukan."})
	}

	var garansi models.Garansi
	if err := config.DB.Where("id = ? AND tracking_penawaran_id = ?", month.GaransiID, trackingID).First(&garansi).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Garansi tidak ditemukan."})
	}

	// Tanggal kunjungan cuma bisa diubah selama daily bulan ini masih berjalan —
	// begitu daily-nya disetujui selesai, tanggal terkunci.
	if month.ActivitySelesai {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Daily bulan ini sudah selesai, tanggal kunjungan tidak bisa diubah lagi."})
	}

	oldTanggal := month.TanggalKunjungan

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		month.TanggalKunjungan = tanggal

		keterangan := fmt.Sprintf("Tanggal kunjungan bulan ke-%d diisi %s", month.BulanKe, tanggal.Format("2006-01-02"))
		if oldTanggal != nil {
			keterangan = fmt.Sprintf(
				"Tanggal kunjungan bulan ke-%d diubah dari %s menjadi %s",
				month.BulanKe, oldTanggal.Format("2006-01-02"), tanggal.Format("2006-01-02"),
			)
		}
		logEntry := models.LogGaransi{
			Aksi:        "Update Tanggal Kunjungan",
			Keterangan:  keterangan,
			PegawaiID:   pegawaiID,
			NamaPegawai: namaPegawai,
			CreatedAt:   now,
		}
		month.LogAktivitas = append(month.LogAktivitas, logEntry)

		if month.ActivitySelesai {
			month.Status = models.StatusDiterima
		}

		if err := tx.Model(&models.GaransiMonth{}).Where("id = ?", month.ID).
			Select("tanggal_kunjungan", "status", "log_aktivitas").
			Updates(models.GaransiMonth{
				TanggalKunjungan: month.TanggalKunjungan,
				Status:           month.Status,
				LogAktivitas:     month.LogAktivitas,
			}).Error; err != nil {
			return err
		}

		// Tampil juga di Log Aktivitas Garansi (level atas, yang ditampilkan
		// di sidebar FE) — bukan cuma di log per-bulan. Pakai Select+Updates
		// (bukan Update kolom tunggal) supaya serializer:json ke-apply.
		garansi.LogAktivitas = append(garansi.LogAktivitas, logEntry)
		if err := tx.Model(&models.Garansi{}).Where("id = ?", garansi.ID).
			Select("log_aktivitas").
			Updates(models.Garansi{LogAktivitas: garansi.LogAktivitas}).Error; err != nil {
			return err
		}

		return models.AdvanceGaransiIfReady(tx, &month, pegawaiID, namaPegawai)
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengupdate tanggal kunjungan."})
	}

	updated, _ := preloadGaransiList(trackingID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Tanggal kunjungan berhasil diperbarui.",
		"data":    updated,
	})
}
