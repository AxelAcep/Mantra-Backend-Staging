package controllers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mantra/src/config"
	"mantra/src/models"
)

// ── Helpers ────────────────────────────────────────────────────────────────

func getBoQClaims(c echo.Context) (pegawaiID, namaPegawai, roleStr, divisiStr string, ok bool) {
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

func canAccessBoQ(roleStr, divisiStr string) bool {
	if roleStr == "MASTER" {
		return true
	}
	if roleStr == "SUPERVISI" {
		return true
	}
	if divisiStr == string(models.DivisiPresales) {
		return true
	}
		if divisiStr == string(models.DivisiAdminSekertariat) {
		return true
	}
	return false
}

func appendBoQLog(boq *models.PenyusunanBoQ, aksi, keterangan, pegawaiID, namaPegawai string) {
    log := models.LogBoq{
        Aksi:        aksi,
        Keterangan:  keterangan,
        PegawaiID:   pegawaiID,
        NamaPegawai: namaPegawai,
        CreatedAt:   time.Now(),
    }
    boq.LogAktivitas = append(boq.LogAktivitas, log)
    config.DB.Save(boq) // ← save seluruh boq biar kolom logs ter-update
}

func recalcEstimasi(boq *models.PenyusunanBoQ) {
	var total float64
	if boq.Harga1 != nil {
		total += *boq.Harga1
	}
	if boq.Harga2 != nil {
		total += *boq.Harga2
	}
	if boq.Harga3 != nil {
		total += *boq.Harga3
	}
	boq.EstimasiHarga = &total
}

func preloadBoQ(trackingID string) (models.PenyusunanBoQ, error) {
    var updated models.PenyusunanBoQ
    
    err := config.DB.
        Where("tracking_penawaran_id = ?", trackingID).
        // 1. Preload relasi langsung dari PenyusunanBoQ
        Preload("Pembuat").
        Preload("Activity").
        Preload("Activity.Pegawai").
        Preload("Dokumen").
        Preload("Dokumen.Pegawai").
		Preload("Activity.Dokumen").
		Preload("Activity.Dokumen.Pegawai").
        Preload("TrackingPenawaran").
        // 2. Preload nested relasi yang ada di dalam TrackingPenawaran
        Preload("TrackingPenawaran.Perusahaan").
        Preload("TrackingPenawaran.Marketing").
        First(&updated).Error
        
    return updated, err
}

// ── 1. Get Detail ──────────────────────────────────────────────────────────

func GetDetailBoQ(c echo.Context) error {
	trackingID := c.Param("id")

	_, _, roleStr, divisiStr, ok := getBoQClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	if !canAccessBoQ(roleStr, divisiStr) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak."})
	}

	updated, err := preloadBoQ(trackingID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data tidak ditemukan."})
	}

	return c.JSON(http.StatusOK, updated)
}

// ── 2. Update Sub Total ────────────────────────────────────────────────────

func UpdateSubTotalBoQ(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, roleStr, divisiStr, ok := getBoQClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	if !canAccessBoQ(roleStr, divisiStr) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak."})
	}

	var body struct {
		Harga1 *float64 `json:"harga1"`
		Harga2 *float64 `json:"harga2"`
		Harga3 *float64 `json:"harga3"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	var boq models.PenyusunanBoQ
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&boq).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "BoQ tidak ditemukan."})
	}

	if body.Harga1 != nil {
		boq.Harga1 = body.Harga1
	}
	if body.Harga2 != nil {
		boq.Harga2 = body.Harga2
	}
	if body.Harga3 != nil {
		boq.Harga3 = body.Harga3
	}

	recalcEstimasi(&boq)
	boq.UpdatedAt = time.Now()
	config.DB.Save(&boq)

	appendBoQLog(&boq, "Update Sub Total", fmt.Sprintf("Harga diperbarui: Total Rp %.0f", *boq.EstimasiHarga), pegawaiID, namaPegawai)

	// ── Trigger ke step berikutnya ──────────────────────────────────────
	boq.Status = models.StatusSelesai
	appendBoQLog(&boq, "Konfirmasi Selesai Diterima", "BoQ disetujui, lanjut ke Review Internal", pegawaiID, namaPegawai)
	config.DB.Save(&boq)

	config.DB.Model(&models.TrackingPenawaran{}).
		Where("id = ?", trackingID).
		Updates(map[string]interface{}{
			"step_saat_ini": models.StepReviewInternal,
			"status":        models.StatusOnProgress,
		})

	// Tentukan Admin Sekertariat untuk daily
	var adminPegawai models.Pegawai
	if divisiStr == string(models.DivisiAdminSekertariat) {
		// Yang update adalah Admin Sekertariat, assign ke dirinya sendiri
		adminPegawai.ID = pegawaiID
		adminPegawai.Nama = namaPegawai
	} else {
		// Bukan Admin Sekertariat, cari Admin Sekertariat pertama
		if err := config.DB.Where("divisi = ?", models.DivisiAdminSekertariat).First(&adminPegawai).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Admin Sekertariat tidak ditemukan."})
		}
	}

	// Ambil nama perusahaan untuk judul daily
	var tracking models.TrackingPenawaran
	config.DB.Preload("Perusahaan").Where("id = ?", trackingID).First(&tracking)
	namaPerusahaan := tracking.Perusahaan.Nama

	// Hitung deadline hari ini jam 5 sore, kalau sudah lewat jadi besok
	now := time.Now()
	deadline := time.Date(now.Year(), now.Month(), now.Day(), 17, 0, 0, 0, now.Location())
	if now.After(deadline) {
		deadline = deadline.Add(24 * time.Hour)
	}

	// Buat Daily Activity untuk Admin Sekertariat
	activityAdminID := generateActivityID()
	dailyAdmin := models.Activity{
		ID:            activityAdminID,
		PegawaiID:     adminPegawai.ID,
		TerkaitPO:     tracking.NomorPO,
		Perusahaan:    &namaPerusahaan,
		Kategori:      models.KategoriQuotation,
		Judul:         "Pengecekan Penawaran " + namaPerusahaan,
		Deskripsi:     "Activity otomatis setelah update sub total BoQ untuk penawaran #" + tracking.NomorPenawaran,
		WaktuMulai:    time.Now(),
		TargetSelesai: deadline,
		Status:        models.StatusOnProgress,
	}
	if err := config.DB.Create(&dailyAdmin).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal membuat daily activity."})
	}

	// Buat Review Internal
	var existingReview models.ReviewInternal
	reviewExists := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&existingReview).Error == nil

	if !reviewExists {
		review := models.ReviewInternal{
			ID:                  uuid.New().String(),
			TrackingPenawaranID: trackingID,
			ActivityAdminID:     &activityAdminID,
			AccAdminDirektur:    false,
			AccManajerOps:       false,
			Status:              models.StatusOnProgress,
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		}
		if err := config.DB.Create(&review).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal membuat Review Internal."})
		}
	} else {
		// Update existing review dengan activity admin
		existingReview.ActivityAdminID = &activityAdminID
		config.DB.Save(&existingReview)
	}

	// ── End Trigger ─────────────────────────────────────────────────────

	updated, _ := preloadBoQ(trackingID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Sub total berhasil diupdate.",
		"data":    updated,
	})
}

// ── 3. Upload Dokumen BoQ ──────────────────────────────────────────────────

func UploadDokumenBoQ(c echo.Context) error {
	trackingID := c.Param("id")

	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)
	namaPegawai, _ := pegawaiMap["nama"].(string)

	var boq models.PenyusunanBoQ
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&boq).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"message": "Data Penyusunan BoQ tidak ditemukan"})
	}

	if boq.ActivityID == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Activity BoQ belum tersedia"})
	}

	if err := c.Request().ParseMultipartForm(10 << 20); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Gagal parse form: " + err.Error()})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "File tidak ditemukan: " + err.Error()})
	}

	allowedExt := map[string]bool{
		".pdf": true, ".doc": true, ".docx": true,
		".xls": true, ".xlsx": true,
		".ppt": true, ".pptx": true,
		".txt": true, ".csv": true,
		".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".webp": true, ".svg": true,
		".zip": true, ".rar": true,
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedExt[ext] {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Tipe file tidak diizinkan"})
	}

	const maxSize = 10 << 20
	if file.Size > maxSize {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Ukuran file maksimal 10MB"})
	}

	uploadDir := getUploadDir()
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal membuat folder upload"})
	}

	uniqueID := uuid.New().String()
	safeOriginal := sanitizeFilename(strings.TrimSuffix(file.Filename, ext))
	newFilename := fmt.Sprintf("%s_%s%s", uniqueID, safeOriginal, ext)
	destPath := filepath.Join(uploadDir, newFilename)

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal membuka file"})
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal menyimpan file"})
	}
	defer dst.Close()

	buf := make([]byte, 32*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			dst.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}

	filePath := "/uploads/" + newFilename

	// Simpan ke ActivityDokumen
	dokumen := models.ActivityDokumen{
		ID:         uuid.New().String(),
		NamaFile:   file.Filename,
		Path:       filePath,
		UploadedBy: pegawaiID,
		ActivityID: *boq.ActivityID,
		CreatedAt:  time.Now(),
	}
	if err := config.DB.Create(&dokumen).Error; err != nil {
		os.Remove(destPath)
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal menyimpan data dokumen BoQ"})
	}

	appendBoQLog(&boq, "Upload Dokumen", "Menambahkan dokumen **"+file.Filename+"**", pegawaiID, namaPegawai)

	config.DB.Preload("Pegawai").First(&dokumen, "id = ?", dokumen.ID)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "File BoQ berhasil diunggah",
		"data":    dokumen,
	})
}

func DeleteDokumenBoQ(c echo.Context) error {
	trackingID := c.Param("id")
	dokumenID := c.Param("dokumenId")

	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}
	pegawaiMap, ok2 := claims["pegawai"].(map[string]interface{})
	if !ok2 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}
	pegawaiID, _ := pegawaiMap["id"].(string)
	namaPegawai, _ := pegawaiMap["nama"].(string)

	var boq models.PenyusunanBoQ
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&boq).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"message": "Data Penyusunan BoQ tidak ditemukan"})
	}

	// Cari dokumen di ActivityDokumen
	var dokumen models.ActivityDokumen
	if err := config.DB.Where("id = ? AND activity_id = ?", dokumenID, boq.ActivityID).First(&dokumen).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"message": "Dokumen BoQ tidak ditemukan"})
	}

	if dokumen.UploadedBy != pegawaiID {
		return c.JSON(http.StatusForbidden, map[string]string{"message": "Anda tidak berhak menghapus dokumen ini"})
	}

	uploadDir := getUploadDir()
	filename := strings.TrimPrefix(dokumen.Path, "/uploads/")
	filePath := filepath.Join(uploadDir, filename)
	os.Remove(filePath)

	if err := config.DB.Delete(&dokumen).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal menghapus dokumen BoQ"})
	}

	appendBoQLog(&boq, "Hapus Dokumen", "Menghapus dokumen **"+dokumen.NamaFile+"**", pegawaiID, namaPegawai)

	return c.JSON(http.StatusOK, map[string]string{"message": "Dokumen BoQ berhasil dihapus"})
}

// ── 5. Update Status BoQ ───────────────────────────────────────────────────

func UpdateStatusBoQ(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, roleStr, divisiStr, ok := getBoQClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	if !canAccessBoQ(roleStr, divisiStr) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak."})
	}

	var body struct {
		Status string `json:"status"`
		Alasan string `json:"alasanPenolakan"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	var boq models.PenyusunanBoQ
	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Preload("TrackingPenawaran.Perusahaan").
		First(&boq).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "BoQ tidak ditemukan."})
	}

	switch body.Status {

	case "PERLU_TINDAKAN":
		if roleStr != "MASTER" {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Hanya Master yang bisa menolak."})
		}
		if body.Alasan == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Alasan wajib diisi."})
		}
		boq.Status = models.StatusPerluTindakan
		appendBoQLog(
			&boq,
			"Tolak: "+body.Alasan,
			body.Alasan,
			pegawaiID,
			namaPegawai,
		)
		config.DB.Save(&boq)

		config.DB.Model(&models.TrackingPenawaran{}).
			Where("id = ?", trackingID).
			Update("status", models.StatusPerluTindakan)

	case "KONFIRMASI_SELESAI":
		if roleStr == "MASTER" {
			boq.Status = models.StatusSelesai
			appendBoQLog(&boq, "Konfirmasi Selesai Diterima", "BoQ disetujui, lanjut ke Review Internal", pegawaiID, namaPegawai)
			config.DB.Save(&boq)

			config.DB.Model(&models.TrackingPenawaran{}).
				Where("id = ?", trackingID).
				Updates(map[string]interface{}{
					"step_saat_ini": models.StepReviewInternal,
					"status":        models.StatusOnProgress,
				})

			var existingReview models.ReviewInternal
			reviewExists := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&existingReview).Error == nil

			if !reviewExists {
				review := models.ReviewInternal{
					ID:                  uuid.New().String(),
					TrackingPenawaranID: trackingID,
					AccAdminDirektur:    false,
					AccManajerOps:       false,
					Status:              models.StatusOnProgress,
					CreatedAt:           time.Now(),
					UpdatedAt:           time.Now(),
				}
				if err := config.DB.Create(&review).Error; err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal membuat Review Internal."})
				}
			}

		} else {
			boq.Status = models.StatusKonfirmasiSelesai
			appendBoQLog(&boq, "Konfirmasi Selesai Diajukan", "Menunggu persetujuan Master", pegawaiID, namaPegawai)
			config.DB.Save(&boq)

			config.DB.Model(&models.TrackingPenawaran{}).
				Where("id = ?", trackingID).
				Update("status", models.StatusKonfirmasiSelesai)
		}

	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Status tidak valid."})
	}

	updated, err := preloadBoQ(trackingID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data terbaru."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Status berhasil diupdate.",
		"data":    updated,
	})
}