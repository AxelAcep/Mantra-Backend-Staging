package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mantra/src/config"
	"mantra/src/models"
)

// ── Helpers ────────────────────────────────────────────────────────────────

func getImplementasiClaims(c echo.Context) (pegawaiID, namaPegawai, roleStr, divisiStr string, ok bool) {
	claims, valid := c.Get("user").(jwt.MapClaims)
	if !valid {
		return "", "", "", "", false
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ = pegawaiMap["id"].(string)
	namaPegawai, _ = pegawaiMap["nama"].(string)
	roleStr, _ = claims["role"].(string)
	divisiStr, _ = pegawaiMap["divisi"].(string)
	return pegawaiID, namaPegawai, roleStr, divisiStr, true
}

func parseDate(dStr string) *time.Time {
	if dStr == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", dStr)
	if err == nil {
		return &t
	}
	t2, err2 := time.Parse(time.RFC3339, dStr)
	if err2 == nil {
		return &t2
	}
	return nil
}

func formatNumber(val float64) string {
	isInteger := val == float64(int64(val))
	var s string
	if isInteger {
		s = fmt.Sprintf("%.0f", val)
	} else {
		s = fmt.Sprintf("%.2f", val)
	}

	parts := strings.Split(s, ".")
	intPart := parts[0]
	decPart := ""
	if len(parts) > 1 {
		decPart = "," + parts[1]
	}

	var result []string
	length := len(intPart)
	for i, c := range intPart {
		if i > 0 && (length-i)%3 == 0 {
			result = append(result, ".")
		}
		result = append(result, string(c))
	}
	return strings.Join(result, "") + decPart
}

func preloadImplementasi(trackingID string) (models.Implementasi, error) {
	var impl models.Implementasi
	err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Preload("TrackingPenawaran.Perusahaan").
		Preload("TrackingPenawaran.Marketing").
		Preload("Barang").
		Preload("Dokumen").
		Preload("Dokumen.Pegawai").
		Preload("ActivityPembelian.Pegawai").
		Preload("ActivityPengantaran.Pegawai").
		Preload("ActivityInstalasi.Pegawai").
		First(&impl).Error
	return impl, err
}

func appendImplementasiLog(impl *models.Implementasi, aksi, keterangan, pegawaiID, namaPegawai string) {
	log := models.LogImplementasi{
		Aksi:        aksi,
		Keterangan:  keterangan,
		PegawaiID:   pegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   time.Now(),
	}
	impl.LogAktivitas = append(impl.LogAktivitas, log)
	config.DB.Save(impl)
}

// ── GET /tracking-penawaran/:id/implementasi ──────────────────────────────────

func GetDetailImplementasi(c echo.Context) error {
	trackingID := c.Param("id")
	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	impl, err := preloadImplementasi(trackingID)
	if err != nil {
		// Auto-initialize Implementasi
		var tracking models.TrackingPenawaran
		if errDb := config.DB.Preload("Perusahaan").First(&tracking, "id = ?", trackingID).Error; errDb == nil {
			// Create Activities
			actPembelianID := uuid.New().String()
			actPembelian := models.Activity{
				ID:            actPembelianID,
				PegawaiID:     pegawaiID,
				TerkaitPO:     &tracking.NomorPenawaran,
				Perusahaan:    &tracking.Perusahaan.Nama,
				Kategori:      models.KategoriBillOfQuantity,
				Judul:         "Pembelian Barang Proyek - " + tracking.Perusahaan.Nama,
				Deskripsi:     "Melakukan pembelian barang proyek sesuai daftar untuk penawaran #" + tracking.NomorPenawaran,
				WaktuMulai:    time.Now(),
				TargetSelesai: time.Now().Add(7 * 24 * time.Hour),
				Status:        models.StatusOnProgress,
			}
			config.DB.Create(&actPembelian)

			actPengantaranID := uuid.New().String()
			actPengantaran := models.Activity{
				ID:            actPengantaranID,
				PegawaiID:     pegawaiID,
				TerkaitPO:     &tracking.NomorPenawaran,
				Perusahaan:    &tracking.Perusahaan.Nama,
				Kategori:      models.KategoriAkomodasiProject,
				Judul:         "Pengantaran Barang Proyek - " + tracking.Perusahaan.Nama,
				Deskripsi:     "Mengatur pengantaran logistik barang proyek untuk penawaran #" + tracking.NomorPenawaran,
				WaktuMulai:    time.Now(),
				TargetSelesai: time.Now().Add(7 * 24 * time.Hour),
				Status:        models.StatusOnProgress,
			}
			config.DB.Create(&actPengantaran)

			actInstalasiID := uuid.New().String()
			actInstalasi := models.Activity{
				ID:            actInstalasiID,
				PegawaiID:     pegawaiID,
				TerkaitPO:     &tracking.NomorPenawaran,
				Perusahaan:    &tracking.Perusahaan.Nama,
				Kategori:      models.KategoriMonitorProgress,
				Judul:         "Instalasi dan Uji Coba Proyek - " + tracking.Perusahaan.Nama,
				Deskripsi:     "Melakukan instalasi teknis dan uji coba perangkat di lokasi proyek untuk penawaran #" + tracking.NomorPenawaran,
				WaktuMulai:    time.Now(),
				TargetSelesai: time.Now().Add(7 * 24 * time.Hour),
				Status:        models.StatusOnProgress,
			}
			config.DB.Create(&actInstalasi)

			newImpl := models.Implementasi{
				ID:                     uuid.New().String(),
				TrackingPenawaranID:    trackingID,
				Status:                 models.StatusOnProgress,
				ActivityPembelianID:    &actPembelianID,
				ActivityPengantaranID:  &actPengantaranID,
				ActivityInstalasiID:    &actInstalasiID,
				LogAktivitas: []models.LogImplementasi{
					{
						Aksi:        "Implementasi Dimulai",
						Keterangan:  "Inisialisasi otomatis proses implementasi.",
						PegawaiID:   pegawaiID,
						NamaPegawai: namaPegawai,
						CreatedAt:   time.Now(),
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			if errCreate := config.DB.Create(&newImpl).Error; errCreate == nil {
				impl, _ = preloadImplementasi(trackingID)
				return c.JSON(http.StatusOK, impl)
			}
		}
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Implementasi tidak ditemukan."})
	}

	// Self-healing for existing records without Activities
	updated := false
	if impl.ActivityPembelianID == nil {
		actPembelianID := uuid.New().String()
		actPembelian := models.Activity{
			ID:            actPembelianID,
			PegawaiID:     pegawaiID,
			TerkaitPO:     &impl.TrackingPenawaran.NomorPenawaran,
			Perusahaan:    &impl.TrackingPenawaran.Perusahaan.Nama,
			Kategori:      models.KategoriBillOfQuantity,
			Judul:         "Pembelian Barang Proyek - " + impl.TrackingPenawaran.Perusahaan.Nama,
			Deskripsi:     "Melakukan pembelian barang proyek sesuai daftar untuk penawaran #" + impl.TrackingPenawaran.NomorPenawaran,
			WaktuMulai:    time.Now(),
			TargetSelesai: time.Now().Add(7 * 24 * time.Hour),
			Status:        models.StatusOnProgress,
		}
		config.DB.Create(&actPembelian)
		impl.ActivityPembelianID = &actPembelianID
		updated = true
	}
	if impl.ActivityPengantaranID == nil {
		actPengantaranID := uuid.New().String()
		actPengantaran := models.Activity{
			ID:            actPengantaranID,
			PegawaiID:     pegawaiID,
			TerkaitPO:     &impl.TrackingPenawaran.NomorPenawaran,
			Perusahaan:    &impl.TrackingPenawaran.Perusahaan.Nama,
			Kategori:      models.KategoriAkomodasiProject,
			Judul:         "Pengantaran Barang Proyek - " + impl.TrackingPenawaran.Perusahaan.Nama,
			Deskripsi:     "Mengatur pengantaran logistik barang proyek untuk penawaran #" + impl.TrackingPenawaran.NomorPenawaran,
			WaktuMulai:    time.Now(),
			TargetSelesai: time.Now().Add(7 * 24 * time.Hour),
			Status:        models.StatusOnProgress,
		}
		config.DB.Create(&actPengantaran)
		impl.ActivityPengantaranID = &actPengantaranID
		updated = true
	}
	if impl.ActivityInstalasiID == nil {
		actInstalasiID := uuid.New().String()
		actInstalasi := models.Activity{
			ID:            actInstalasiID,
			PegawaiID:     pegawaiID,
			TerkaitPO:     &impl.TrackingPenawaran.NomorPenawaran,
			Perusahaan:    &impl.TrackingPenawaran.Perusahaan.Nama,
			Kategori:      models.KategoriMonitorProgress,
			Judul:         "Instalasi dan Uji Coba Proyek - " + impl.TrackingPenawaran.Perusahaan.Nama,
			Deskripsi:     "Melakukan instalasi teknis dan uji coba perangkat di lokasi proyek untuk penawaran #" + impl.TrackingPenawaran.NomorPenawaran,
			WaktuMulai:    time.Now(),
			TargetSelesai: time.Now().Add(7 * 24 * time.Hour),
			Status:        models.StatusOnProgress,
		}
		config.DB.Create(&actInstalasi)
		impl.ActivityInstalasiID = &actInstalasiID
		updated = true
	}
	if updated {
		config.DB.Save(&impl)
		impl, _ = preloadImplementasi(trackingID)
	}

	return c.JSON(http.StatusOK, impl)
}

// ── PATCH /tracking-penawaran/:id/implementasi ───────────────────────────────

func UpdateDetailImplementasi(c echo.Context) error {
	trackingID := c.Param("id")
	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		NoPO      string `json:"noPO"`
		TanggalPO string `json:"tanggalPO"`
		NoWO      string `json:"noWO"`
		TanggalWO string `json:"tanggalWO"`
		NoDO      string `json:"noDO"`
		TanggalDO string `json:"tanggalDO"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	var impl models.Implementasi
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&impl).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Implementasi tidak ditemukan."})
	}

	oldNoPO := impl.NoPO
	oldNoWO := impl.NoWO
	oldNoDO := impl.NoDO

	impl.NoPO = body.NoPO
	impl.TanggalPO = parseDate(body.TanggalPO)
	impl.NoWO = body.NoWO
	impl.TanggalWO = parseDate(body.TanggalWO)
	impl.NoDO = body.NoDO
	impl.TanggalDO = parseDate(body.TanggalDO)
	impl.UpdatedAt = time.Now()

	if err := config.DB.Save(&impl).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengupdate detail implementasi."})
	}

	// Update parent TrackingPenawaran NomorPO also for consistency
	if body.NoPO != "" {
		config.DB.Model(&models.TrackingPenawaran{}).Where("id = ?", trackingID).Update("nomor_po", body.NoPO)
	}

	var changes []string
	if oldNoPO != body.NoPO {
		o := oldNoPO
		if o == "" {
			o = "kosong"
		}
		n := body.NoPO
		if n == "" {
			n = "kosong"
		}
		changes = append(changes, fmt.Sprintf("No. Purchase Order diubah dari '%s' menjadi '%s'", o, n))
	}
	if oldNoWO != body.NoWO {
		o := oldNoWO
		if o == "" {
			o = "kosong"
		}
		n := body.NoWO
		if n == "" {
			n = "kosong"
		}
		changes = append(changes, fmt.Sprintf("No. Work Order diubah dari '%s' menjadi '%s'", o, n))
	}
	if oldNoDO != body.NoDO {
		o := oldNoDO
		if o == "" {
			o = "kosong"
		}
		n := body.NoDO
		if n == "" {
			n = "kosong"
		}
		changes = append(changes, fmt.Sprintf("No. Delivery Order diubah dari '%s' menjadi '%s'", o, n))
	}

	keterangan := "Memperbarui data No PO/WO/DO"
	if len(changes) > 0 {
		keterangan = strings.Join(changes, ", ")
	}

	appendImplementasiLog(&impl, "Update Info Order", keterangan, pegawaiID, namaPegawai)

	updated, _ := preloadImplementasi(trackingID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Detail implementasi berhasil diperbarui.",
		"data":    updated,
	})
}

// ── POST /tracking-penawaran/:id/implementasi/barang ─────────────────────────

func AddBarangImplementasi(c echo.Context) error {
	trackingID := c.Param("id")
	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var impl models.Implementasi
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&impl).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Implementasi tidak ditemukan."})
	}

	var body struct {
		NamaBarang         string  `json:"namaBarang"`
		Status             string  `json:"status"`
		Qty                float64 `json:"qty"`
		Satuan             string  `json:"satuan"`
		HargaSatuan        float64 `json:"hargaSatuan"`
		Metode             string  `json:"metode"`
		EstimasiKedatangan string  `json:"estimasiKedatangan"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	barang := models.ImplementasiBarang{
		ID:                 uuid.New().String(),
		ImplementasiID:     impl.ID,
		NamaBarang:         body.NamaBarang,
		Status:             body.Status,
		Qty:                body.Qty,
		Satuan:             body.Satuan,
		HargaSatuan:        body.HargaSatuan,
		Metode:             body.Metode,
		EstimasiKedatangan: parseDate(body.EstimasiKedatangan),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := config.DB.Create(&barang).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menambahkan barang."})
	}

	appendImplementasiLog(&impl, "Tambah Barang", fmt.Sprintf("Menambahkan barang baru dengan nama '%s' (Status: %s, Qty: %s %s, Harga: Rp %s, Metode: %s)", body.NamaBarang, body.Status, formatNumber(body.Qty), body.Satuan, formatNumber(body.HargaSatuan), body.Metode), pegawaiID, namaPegawai)

	updated, _ := preloadImplementasi(trackingID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Barang berhasil ditambahkan.",
		"data":    updated,
	})
}

// ── PATCH /tracking-penawaran/:id/implementasi/barang/:barangId ──────────────

func UpdateBarangImplementasi(c echo.Context) error {
	trackingID := c.Param("id")
	barangID := c.Param("barangId")
	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var impl models.Implementasi
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&impl).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Implementasi tidak ditemukan."})
	}

	var barang models.ImplementasiBarang
	if err := config.DB.Where("id = ? AND implementasi_id = ?", barangID, impl.ID).First(&barang).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Barang tidak ditemukan."})
	}

	var body struct {
		NamaBarang         string  `json:"namaBarang"`
		Status             string  `json:"status"`
		Qty                float64 `json:"qty"`
		Satuan             string  `json:"satuan"`
		HargaSatuan        float64 `json:"hargaSatuan"`
		Metode             string  `json:"metode"`
		EstimasiKedatangan string  `json:"estimasiKedatangan"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	var changes []string
	if barang.NamaBarang != body.NamaBarang {
		changes = append(changes, fmt.Sprintf("nama barang dari '%s' menjadi '%s'", barang.NamaBarang, body.NamaBarang))
	}
	if barang.Status != body.Status {
		changes = append(changes, fmt.Sprintf("status dari '%s' menjadi '%s'", barang.Status, body.Status))
	}
	if barang.Qty != body.Qty {
		changes = append(changes, fmt.Sprintf("qty dari '%s' menjadi '%s'", formatNumber(barang.Qty), formatNumber(body.Qty)))
	}
	if barang.Satuan != body.Satuan {
		changes = append(changes, fmt.Sprintf("satuan dari '%s' menjadi '%s'", barang.Satuan, body.Satuan))
	}
	if barang.HargaSatuan != body.HargaSatuan {
		changes = append(changes, fmt.Sprintf("harga satuan dari 'Rp %s' menjadi 'Rp %s'", formatNumber(barang.HargaSatuan), formatNumber(body.HargaSatuan)))
	}
	if barang.Metode != body.Metode {
		changes = append(changes, fmt.Sprintf("metode dari '%s' menjadi '%s'", barang.Metode, body.Metode))
	}
	newEst := parseDate(body.EstimasiKedatangan)
	if (barang.EstimasiKedatangan == nil && newEst != nil) || (barang.EstimasiKedatangan != nil && newEst == nil) || (barang.EstimasiKedatangan != nil && newEst != nil && !barang.EstimasiKedatangan.Equal(*newEst)) {
		oldEstStr := "kosong"
		if barang.EstimasiKedatangan != nil {
			oldEstStr = barang.EstimasiKedatangan.Format("2006-01-02")
		}
		newEstStr := "kosong"
		if newEst != nil {
			newEstStr = newEst.Format("2006-01-02")
		}
		changes = append(changes, fmt.Sprintf("estimasi kedatangan dari '%s' menjadi '%s'", oldEstStr, newEstStr))
	}

	barang.NamaBarang = body.NamaBarang
	barang.Status = body.Status
	barang.Qty = body.Qty
	barang.Satuan = body.Satuan
	barang.HargaSatuan = body.HargaSatuan
	barang.Metode = body.Metode
	barang.EstimasiKedatangan = newEst
	barang.UpdatedAt = time.Now()

	if err := config.DB.Save(&barang).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal memperbarui barang."})
	}

	keterangan := fmt.Sprintf("Memperbarui info barang '%s'", barang.NamaBarang)
	if len(changes) > 0 {
		keterangan = fmt.Sprintf("Memperbarui barang '%s' (%s)", barang.NamaBarang, strings.Join(changes, ", "))
	}

	appendImplementasiLog(&impl, "Update Barang", keterangan, pegawaiID, namaPegawai)

	updated, _ := preloadImplementasi(trackingID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Barang berhasil diperbarui.",
		"data":    updated,
	})
}

// ── DELETE /tracking-penawaran/:id/implementasi/barang/:barangId ─────────────

func DeleteBarangImplementasi(c echo.Context) error {
	trackingID := c.Param("id")
	barangID := c.Param("barangId")
	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var impl models.Implementasi
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&impl).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Implementasi tidak ditemukan."})
	}

	var barang models.ImplementasiBarang
	if err := config.DB.Where("id = ? AND implementasi_id = ?", barangID, impl.ID).First(&barang).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Barang tidak ditemukan."})
	}

	namaBarang := barang.NamaBarang
	if err := config.DB.Delete(&barang).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menghapus barang."})
	}

	appendImplementasiLog(&impl, "Hapus Barang", fmt.Sprintf("Menghapus barang '%s' dari daftar pembelian barang proyek", namaBarang), pegawaiID, namaPegawai)

	updated, _ := preloadImplementasi(trackingID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Barang berhasil dihapus.",
		"data":    updated,
	})
}
