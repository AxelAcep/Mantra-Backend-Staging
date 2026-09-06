package controllers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"mantra/src/config"
	"mantra/src/models"
)

// ── Helpers ─────────────────────────────────────────────────────────────────

func preloadGaransi(trackingID string) (*models.Garansi, error) {
	var garansi models.Garansi
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
		First(&garansi).Error

	if err != nil {
		return nil, err
	}
	return &garansi, nil
}

// ── Get Detail (tracking status per bulan, logbook, dokumen pendukung) ───────

func GetDetailGaransi(c echo.Context) error {
	trackingID := c.Param("id")
	_, _, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	garansi, err := preloadGaransi(trackingID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Garansi tidak ditemukan."})
	}

	return c.JSON(http.StatusOK, garansi)
}

// ── Konfigurasi Timeline (input lama tahun + bulan/tahun mulai) ──────────────

func KonfigurasiGaransi(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		LamaTahun  int `json:"lamaTahun"`
		BulanMulai int `json:"bulanMulai"`
		TahunMulai int `json:"tahunMulai"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}
	if body.LamaTahun <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Lama tahun garansi wajib diisi."})
	}
	if body.BulanMulai < 1 || body.BulanMulai > 12 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Bulan mulai tidak valid."})
	}
	if body.TahunMulai <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Tahun mulai wajib diisi."})
	}

	var garansi models.Garansi
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&garansi).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Garansi tidak ditemukan."})
	}
	if garansi.Status != models.StatusGaransiBelumDikonfigurasi {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Garansi sudah dikonfigurasi sebelumnya."})
	}

	totalBulan := body.LamaTahun * 12

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		bulan := body.BulanMulai
		tahun := body.TahunMulai

		for i := 1; i <= totalBulan; i++ {
			month := models.GaransiMonth{
				ID:        uuid.New().String(),
				GaransiID: garansi.ID,
				BulanKe:   i,
				Bulan:     bulan,
				Tahun:     tahun,
				Status:    models.StatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := tx.Create(&month).Error; err != nil {
				return err
			}

			// Daily hanya dibuat untuk bulan ke-1, bulan berikutnya dibuat otomatis
			// lewat hook (AdvanceGaransiIfReady) setelah bulan berjalan selesai.
			if i == 1 {
				if err := models.CreateGaransiMonthActivity(tx, &month, garansi.PICID, pegawaiID, namaPegawai); err != nil {
					return err
				}
			}

			bulan++
			if bulan > 12 {
				bulan = 1
				tahun++
			}
		}

		garansi.LamaTahun = &body.LamaTahun
		garansi.BulanMulai = &body.BulanMulai
		garansi.TahunMulai = &body.TahunMulai
		garansi.Status = models.StatusGaransiOnProgress
		garansi.LogAktivitas = append(garansi.LogAktivitas, models.LogGaransi{
			Aksi:        "Konfigurasi Garansi",
			Keterangan:  fmt.Sprintf("Timeline garansi %d tahun (%d bulan) dikonfigurasi, mulai %02d/%d", body.LamaTahun, totalBulan, body.BulanMulai, body.TahunMulai),
			PegawaiID:   pegawaiID,
			NamaPegawai: namaPegawai,
			CreatedAt:   now,
		})
		garansi.UpdatedAt = now

		return tx.Model(&models.Garansi{}).Where("id = ?", garansi.ID).
			Select("lama_tahun", "bulan_mulai", "tahun_mulai", "status", "log_aktivitas", "updated_at").
			Updates(models.Garansi{
				LamaTahun:    garansi.LamaTahun,
				BulanMulai:   garansi.BulanMulai,
				TahunMulai:   garansi.TahunMulai,
				Status:       garansi.Status,
				LogAktivitas: garansi.LogAktivitas,
				UpdatedAt:    garansi.UpdatedAt,
			}).Error
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengkonfigurasi timeline Garansi."})
	}

	updated, _ := preloadGaransi(trackingID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Timeline Garansi berhasil dikonfigurasi.",
		"data":    updated,
	})
}

// ── Update Tanggal Kunjungan per Bulan ───────────────────────────────────────

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

	var garansi models.Garansi
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&garansi).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Garansi tidak ditemukan."})
	}

	var month models.GaransiMonth
	if err := config.DB.Where("id = ? AND garansi_id = ?", monthID, garansi.ID).First(&month).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Bulan Garansi tidak ditemukan."})
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

	updated, _ := preloadGaransi(trackingID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Tanggal kunjungan berhasil diperbarui.",
		"data":    updated,
	})
}
