package controllers

import (
	"fmt"
	"mantra/src/config"
	"mantra/src/models"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func preloadPersetujuanManajemen(trackingID string) (models.PersetujuanManajemen, error) {
    var persetujuan models.PersetujuanManajemen
    err := config.DB.
        Where("tracking_penawaran_id = ?", trackingID).
        Preload("TrackingPenawaran.Perusahaan").
        Preload("TrackingPenawaran.Marketing").
        Preload("Dokumen").
		Preload("ActivityAdmin").         // ← tambahkan
        Preload("ActivityAdmin.Pegawai"). // ← tambahkan
		Preload("ActivityAdmin.Dokumen").
		Preload("ActivityAdmin.Dokumen.Pegawai").
        First(&persetujuan).Error
    return persetujuan, err
}

// ── Get Detail ─────────────────────────────────────────────────────────────

func GetDetailPersetujuanManajemen(c echo.Context) error {
    trackingID := c.Param("id")

    _, _, roleStr, divisiStr, ok := getReviewClaims(c)
    if !ok {
        return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
    }
    if !canViewStep(models.StepPersetujuanManajemen, roleStr, divisiStr) {
        return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak."})
    }

    persetujuan, err := preloadPersetujuanManajemen(trackingID)
    if err != nil {
        return c.JSON(http.StatusNotFound, map[string]string{"error": "Persetujuan Manajemen tidak ditemukan."})
    }

    return c.JSON(http.StatusOK, persetujuan)
}

// ── Update Status ──────────────────────────────────────────────────────────

func UpdateStatusPersetujuanManajemen(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, roleStr, divisiStr, ok := getReviewClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		Status string `json:"status"`
		Alasan string `json:"alasanPenolakan"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	var persetujuan models.PersetujuanManajemen
	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		First(&persetujuan).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Persetujuan Manajemen tidak ditemukan."})
	}

	isDirekturKomisaris := divisiStr == "DIREKTUR" || divisiStr == "KOMISARIS"
	isSalesPresalesSupervisi :=
		divisiStr == "SALES" ||
		divisiStr == "PRESALES" ||
		divisiStr == "MANAGER_OPERASIONAL" ||
		(roleStr == "SUPERVISI" && divisiStr == "SALES")

	switch body.Status {

	case "SELESAI":
		if !isDirekturKomisaris {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Hanya Direktur atau Komisaris yang bisa acc."})
		}
		if persetujuan.Status == models.StatusPerluTindakan {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Status perlu tindakan, harus dikonfirmasi ulang dulu."})
		}

		persetujuan.AccDirekturKomisaris = true
		persetujuan.Status = models.StatusSelesai

		appendPersetujuanManajemenLog(
			&persetujuan,
			"Approve Direktur/Komisaris",
			"Persetujuan Manajemen disetujui oleh Direktur/Komisaris",
			pegawaiID,
			namaPegawai,
		)

		var tracking models.TrackingPenawaran
		config.DB.Preload("Perusahaan").First(&tracking, "id = ?", trackingID)

		// Notifikasi Tahap 4 (target_hari_ini.md): Approval Direktur →
		// MO + Admin Sekertaris. Dibuat SEBELUM step_saat_ini diupdate ke
		// FOLLOW_UP supaya "Tahap Proses Pengadaan" tampil benar:
		// "Persetujuan Manajemen" (bukan Follow Up).
		var adminSekEvent models.Pegawai
		adminSekID := ""
		if err := config.DB.Where("divisi = ?", models.DivisiAdminSekertaris).First(&adminSekEvent).Error; err == nil {
			adminSekID = adminSekEvent.ID
		}
		models.NotifPengadaanEvent(config.DB,
			"Persetujuan Manajemen Disetujui - "+tracking.Perusahaan.Nama,
			"Direktur/Komisaris menyetujui persetujuan manajemen penawaran #"+tracking.NomorPenawaran+". Proses lanjut ke Follow Up (pengiriman dokumen ke klien).",
			trackingID, tracking.NomorPenawaran, tracking.Perusahaan.Nama, tracking.LokasiProyek,
			persetujuan.ActivityAdminID,
			adminSekID,
		)

		config.DB.Model(&models.TrackingPenawaran{}).
			Where("id = ?", trackingID).
			Updates(map[string]interface{}{
				"step_saat_ini": models.StepFollowUp,
				"status":        models.StatusOnProgress,
			})

		// --- Backfill daily step 4 untuk Admin Sekertaris ---
		// Daily pengingat approval kini dibuat otomatis saat tracking MASUK
		// step 4 (TryFinalizeReviewInternal). Blok ini hanya fallback untuk
		// tracking lama yang sudah berada di step 4 sebelum perubahan itu.
		if persetujuan.ActivityAdminID == nil {
			var adminPegawai models.Pegawai
			if err := config.DB.Where("divisi = ?", models.DivisiAdminSekertaris).First(&adminPegawai).Error; err != nil {
				// fallback ke pegawai yang sedang login
				adminPegawai.ID = pegawaiID
				adminPegawai.Nama = namaPegawai
			}

			namaPerusahaan := tracking.Perusahaan.Nama

			now := time.Now()
			deadline := time.Date(now.Year(), now.Month(), now.Day(), 17, 0, 0, 0, now.Location())
			if now.After(deadline) {
				deadline = deadline.Add(24 * time.Hour)
			}

			activityID := generateActivityID()
			dailyAdmin := models.Activity{
				ID:            activityID,
				PegawaiID:     adminPegawai.ID,
				TerkaitPO:     &tracking.NomorPenawaran,
				Perusahaan:    &namaPerusahaan,
				Kategori:      models.KategoriQuotation,
				Judul:         "Pengecekan Persetujuan Manajemen - " + namaPerusahaan,
				Deskripsi:     "Activity otomatis saat pengadaan masuk tahap Persetujuan Manajemen. Mengingatkan Direktur/Komisaris untuk melakukan approval dan mengunggah dokumen yang sudah ditandatangani untuk penawaran #" + tracking.NomorPenawaran,
				WaktuMulai:    time.Now(),
				TargetSelesai: deadline,
				Status:        models.StatusOnProgress,
			}
			if err := config.DB.Create(&dailyAdmin).Error; err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal membuat daily activity."})
			}

			persetujuan.ActivityAdminID = &activityID
			config.DB.Save(&persetujuan)
		}

		// --- Inisialisasi FollowUp ---
		// Pengiriman dokumen penawaran lengkap ke klien ditugaskan ke Admin
		// Sekertaris (DivisiAdminSekertaris), bukan Admin Sekertariat.
		var existingFollowUp models.FollowUp
		followUpExists := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&existingFollowUp).Error == nil
		if !followUpExists {
			var adminFollowUp models.Pegawai
			var adminID string
			var adminNama string
			if err := config.DB.Where("divisi = ?", models.DivisiAdminSekertaris).First(&adminFollowUp).Error; err == nil {
				adminID = adminFollowUp.ID
				adminNama = adminFollowUp.Nama
			} else {
				adminID = pegawaiID
				adminNama = namaPegawai
			}

			followUpActivityID := generateActivityID()
			perusahaanNama := tracking.Perusahaan.Nama
			dailyFollowUp := models.Activity{
				ID:            followUpActivityID,
				PegawaiID:     adminID,
				TerkaitPO:     &tracking.NomorPenawaran,
				Perusahaan:    &perusahaanNama,
				Kategori:      models.KategoriQuotation,
				Judul:         "Kirim Dokumen Penawaran Lengkap - " + perusahaanNama,
				Deskripsi:     "Mengirimkan dokumen penawaran lengkap via email ke klien. Kontak: " + tracking.CustomerName + " (" + tracking.CustomerEmail + " / " + tracking.CustomerPhone + ")",
				WaktuMulai:    time.Now(),
				TargetSelesai: time.Now().Add(24 * time.Hour),
				Status:        models.StatusOnProgress,
			}
			if err := config.DB.Create(&dailyFollowUp).Error; err == nil {
				followUp := models.FollowUp{
					ID:                  uuid.New().String(),
					TrackingPenawaranID: trackingID,
					AdminID:             &adminID,
					ActivityAdminID:     &followUpActivityID,
					SalesID:             &tracking.MarketingID,
					ActivitySalesID:     nil,
					Status:              models.StatusOnProgress,
					Stage:               1,
					LogAktivitas: []models.LogFollowUp{
						{
							Aksi:        "Follow Up Dimulai",
							Keterangan:  "Inisialisasi proses follow up. Tugas kirim penawaran ditugaskan ke Admin Sekertaris: " + adminNama,
							PegawaiID:   pegawaiID,
							NamaPegawai: namaPegawai,
							CreatedAt:   time.Now(),
						},
					},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				config.DB.Create(&followUp)

				// Notifikasi Tahap 4→5 (target_hari_ini.md): Daily kirim
				// dokumen ke Admin Sekertaris (pemilik) + MO. Sales TIDAK
				// dinotifikasi di sini — Sales mendapat daily "Follow up
				// Feedback" sendiri di stage 1→2.
				models.NotifTugasPengadaan(config.DB, &dailyFollowUp, trackingID, tracking.LokasiProyek)
			}
		}

	case "PERLU_TINDAKAN":
		if !isDirekturKomisaris {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Hanya Direktur atau Komisaris yang bisa menolak."})
		}
		if body.Alasan == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Alasan wajib diisi."})
		}

		persetujuan.Status = models.StatusPerluTindakan

		appendPersetujuanManajemenLog(
			&persetujuan,
			"Perlu Tindakan",
			body.Alasan,
			pegawaiID,
			namaPegawai,
		)

		config.DB.Model(&models.TrackingPenawaran{}).
			Where("id = ?", trackingID).
			Update("status", models.StatusPerluTindakan)

		// Notifikasi Tahap 4 (target_hari_ini.md): Ditolak Direktur →
		// MO + Admin Sekertaris.
		var trackingTolak models.TrackingPenawaran
		if err := config.DB.Preload("Perusahaan").First(&trackingTolak, "id = ?", trackingID).Error; err == nil {
			var adminSekTolak models.Pegawai
			adminSekTolakID := ""
			if errSek := config.DB.Where("divisi = ?", models.DivisiAdminSekertaris).First(&adminSekTolak).Error; errSek == nil {
				adminSekTolakID = adminSekTolak.ID
			}
			models.NotifPengadaanEvent(config.DB,
				"Persetujuan Manajemen Ditolak - "+trackingTolak.Perusahaan.Nama,
				"Direktur/Komisaris menolak persetujuan manajemen penawaran #"+trackingTolak.NomorPenawaran+". Alasan: "+body.Alasan+". Sales/Presales perlu konfirmasi ulang.",
				trackingID, trackingTolak.NomorPenawaran, trackingTolak.Perusahaan.Nama, trackingTolak.LokasiProyek,
				persetujuan.ActivityAdminID,
				adminSekTolakID,
			)
		}

	case "ON_PROGRESS":
		if !isSalesPresalesSupervisi {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Hanya Sales, Presales, Manager Operasional, atau Supervisi yang bisa konfirmasi ulang."})
		}
		if persetujuan.Status != models.StatusPerluTindakan {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Status bukan Perlu Tindakan."})
		}

		persetujuan.Status = models.StatusOnProgress

		appendPersetujuanManajemenLog(
			&persetujuan,
			"Konfirmasi Ulang",
			"Persetujuan Manajemen dikonfirmasi ulang dan diproses kembali",
			pegawaiID,
			namaPegawai,
		)

		config.DB.Model(&models.TrackingPenawaran{}).
			Where("id = ?", trackingID).
			Update("status", models.StatusOnProgress)

	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Status tidak valid."})
	}

	updated, err := preloadPersetujuanManajemen(trackingID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data terbaru."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Status berhasil diupdate.",
		"data":    updated,
	})
}

func UploadDokumenPersetujuanManajemen(c echo.Context) error {
	trackingID := c.Param("id")

	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)
	namaPegawai, _ := pegawaiMap["nama"].(string)

	var persetujuan models.PersetujuanManajemen
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&persetujuan).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"message": "Data Persetujuan Manajemen tidak ditemukan untuk penawaran ini",
		})
	}

	// Pastikan daily admin sudah ada
	if persetujuan.ActivityAdminID == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Activity Admin belum tersedia. Harap proses persetujuan terlebih dahulu.",
		})
	}

	if err := c.Request().ParseMultipartForm(10 << 20); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Gagal parse form: " + err.Error(),
		})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "File tidak ditemukan: " + err.Error(),
		})
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
		ActivityID: *persetujuan.ActivityAdminID,
		CreatedAt:  time.Now(),
	}
	if err := config.DB.Create(&dokumen).Error; err != nil {
		os.Remove(destPath)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal menyimpan data dokumen Persetujuan Manajemen",
		})
	}

	appendPersetujuanManajemenLog(&persetujuan, "Upload Dokumen", "Menambahkan dokumen **"+file.Filename+"**", pegawaiID, namaPegawai)

	config.DB.Preload("Pegawai").First(&dokumen, "id = ?", dokumen.ID)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "File Persetujuan Manajemen berhasil diunggah",
		"data":    dokumen,
	})
}

func DeleteDokumenPersetujuanManajemen(c echo.Context) error {
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

	var persetujuan models.PersetujuanManajemen
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&persetujuan).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"message": "Data Persetujuan Manajemen tidak ditemukan",
		})
	}

	if persetujuan.ActivityAdminID == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Activity Admin tidak tersedia.",
		})
	}

	// Cari di ActivityDokumen
	var dokumen models.ActivityDokumen
	if err := config.DB.Where("id = ? AND activity_id = ?", dokumenID, *persetujuan.ActivityAdminID).First(&dokumen).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"message": "Dokumen Persetujuan Manajemen tidak ditemukan",
		})
	}

	if dokumen.UploadedBy != pegawaiID {
		return c.JSON(http.StatusForbidden, map[string]string{
			"message": "Anda tidak berhak menghapus dokumen ini karena diunggah oleh orang lain",
		})
	}

	uploadDir := getUploadDir()
	filename := strings.TrimPrefix(dokumen.Path, "/uploads/")
	filePath := filepath.Join(uploadDir, filename)
	os.Remove(filePath)

	if err := config.DB.Delete(&dokumen).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal menghapus dokumen Persetujuan Manajemen",
		})
	}

	appendPersetujuanManajemenLog(&persetujuan, "Hapus Dokumen", "Menghapus dokumen **"+dokumen.NamaFile+"**", pegawaiID, namaPegawai)

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Dokumen Persetujuan Manajemen berhasil dihapus",
	})
}

func appendPersetujuanManajemenLog(persetujuan *models.PersetujuanManajemen, aksi, keterangan, pegawaiID, namaPegawai string) {
    log := models.LogPersetujuanManajemen{
        Aksi:        aksi,
        Keterangan:  keterangan,
        PegawaiID:   pegawaiID,
        NamaPegawai: namaPegawai,
        CreatedAt:   time.Now(),
    }
    persetujuan.LogAktivitas = append(persetujuan.LogAktivitas, log)
    config.DB.Save(persetujuan)
}