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

// ── Helpers ────────────────────────────────────────────────────────────────

func getFollowUpClaims(c echo.Context) (pegawaiID, namaPegawai, roleStr, divisiStr string, ok bool) {
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

func preloadFollowUp(trackingID string) (models.FollowUp, error) {
	var followUp models.FollowUp
	err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Preload("TrackingPenawaran.Perusahaan").
		Preload("TrackingPenawaran.Marketing").
		Preload("Admin").
		Preload("Sales").
		Preload("ActivityAdmin.Pegawai").
		Preload("ActivitySales.Pegawai").
		Preload("Dokumen").
		Preload("Dokumen.Pegawai").
		First(&followUp).Error
	return followUp, err
}

// ── Get Detail ─────────────────────────────────────────────────────────────

func GetDetailFollowUp(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, _, _, ok := getFollowUpClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	followUp, err := preloadFollowUp(trackingID)
	if err != nil {
		// Auto-initialize FollowUp for compatibility with older records
		var tracking models.TrackingPenawaran
		if errDb := config.DB.Preload("Perusahaan").First(&tracking, "id = ?", trackingID).Error; errDb == nil {
			var adminPegawai models.Pegawai
			var adminID string
			var adminNama string
			if errAdmin := config.DB.Where("divisi = ?", models.DivisiAdminSekertariat).First(&adminPegawai).Error; errAdmin == nil {
				adminID = adminPegawai.ID
				adminNama = adminPegawai.Nama
			} else {
				adminID = pegawaiID
				adminNama = namaPegawai
			}

			activityID := generateActivityID()
			perusahaanNama := ""
			if tracking.Perusahaan.Nama != "" {
				perusahaanNama = tracking.Perusahaan.Nama
			}
			dailyAdmin := models.Activity{
				ID:            activityID,
				PegawaiID:     adminID,
				TerkaitPO:     &tracking.NomorPenawaran,
				Perusahaan:    &perusahaanNama,
				Kategori:      models.KategoriQuotation,
				Judul:         "Kirim Dokumen Penawaran Lengkap - " + perusahaanNama,
				Deskripsi:     "Mengirimkan dokumen penawaran lengkap via email ke klien. Kontak: " + tracking.CustomerName + " (" + tracking.CustomerEmail + " / " + tracking.CustomerPhone + ")",
				WaktuMulai:    time.Now(),
				TargetSelesai: time.Now().Add(24 * time.Hour), // Deadline 1 hari
				Status:        models.StatusOnProgress,
			}
			if errAct := config.DB.Create(&dailyAdmin).Error; errAct == nil {
				newFollowUp := models.FollowUp{
					ID:                  uuid.New().String(),
					TrackingPenawaranID: trackingID,
					AdminID:             &adminID,
					ActivityAdminID:     &activityID,
					SalesID:             &tracking.MarketingID,
					ActivitySalesID:     nil,
					Status:              models.StatusOnProgress,
					Stage:               1,
					LogAktivitas: []models.LogFollowUp{
						{
							Aksi:        "Follow Up Dimulai",
							Keterangan:  "Inisialisasi otomatis proses follow up (Data lama/Fallback). Tugas kirim penawaran ditugaskan ke Admin: " + adminNama,
							PegawaiID:   pegawaiID,
							NamaPegawai: namaPegawai,
							CreatedAt:   time.Now(),
						},
					},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				if errCreate := config.DB.Create(&newFollowUp).Error; errCreate == nil {
					followUp, _ = preloadFollowUp(trackingID)
					return c.JSON(http.StatusOK, followUp)
				}
			}
		}
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Follow Up tidak ditemukan."})
	}

	return c.JSON(http.StatusOK, followUp)
}

// ── Update Status/Stage ───────────────────────────────────────────────────

func UpdateStatusFollowUp(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, roleStr, divisiStr, ok := getFollowUpClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		Stage  *int   `json:"stage,omitempty"`
		Status string `json:"status,omitempty"`
		Alasan string `json:"alasanPenolakan,omitempty"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	var followUp models.FollowUp
	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Preload("TrackingPenawaran.Perusahaan").
		Preload("TrackingPenawaran.Marketing").
		First(&followUp).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Follow Up tidak ditemukan."})
	}

	isManagerOps := divisiStr == "MANAGER_OPERASIONAL"
	isMaster := roleStr == "MASTER"

	// If stage is provided (legacy or Stage 2 transition by Admin Sekretariat)
	if body.Status != "" {
		// Handle status/approval changes
		switch body.Status {
		case "KONFIRMASI_SELESAI":
			// Sales submits feedback completion -> Awaiting MO approval
			isSalesPIC := pegawaiID == followUp.TrackingPenawaran.MarketingID
			if !isSalesPIC && !isManagerOps && !isMaster {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "Hanya Sales pembuat penawaran, Manager Operasional, atau Master yang bisa mengajukan feedback.",
				})
			}
			if followUp.Stage < 2 {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Harap selesaikan pengiriman dokumen penawaran terlebih dahulu.",
				})
			}
			if followUp.Stage >= 3 {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Tahap follow up sudah selesai.",
				})
			}

			followUp.Status = models.StatusKonfirmasiSelesai
			appendFollowUpLog(
				&followUp,
				"Feedback Customer Diajukan",
				"Sales mengajukan konfirmasi bahwa feedback customer telah diterima. Menunggu persetujuan Manager Operasional.",
				pegawaiID,
				namaPegawai,
			)

			// Update parent tracking status
			config.DB.Model(&models.TrackingPenawaran{}).
				Where("id = ?", trackingID).
				Update("status", models.StatusKonfirmasiSelesai)

		case "SELESAI":
			// Manager Operasional approves the feedback
			if !isManagerOps && !isMaster {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "Hanya Manager Operasional atau Master yang bisa menyetujui feedback ini.",
				})
			}
			if followUp.Stage < 2 {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Harap selesaikan pengiriman dokumen penawaran terlebih dahulu.",
				})
			}
			if followUp.Stage >= 3 {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Tahap follow up sudah selesai.",
				})
			}

			// Update activity sales ke DITERIMA (Selesai)
			if followUp.ActivitySalesID != nil {
				now := time.Now()
				config.DB.Model(&models.Activity{}).
					Where("id = ?", *followUp.ActivitySalesID).
					Updates(map[string]interface{}{
						"status":       models.StatusDiterima,
						"waktu_submit": &now,
					})
			}

			followUp.Stage = 3
			followUp.Status = models.StatusSelesai
			appendFollowUpLog(
				&followUp,
				"Feedback Customer Disetujui",
				"Persetujuan diberikan oleh Manager Operasional. Feedback customer dikonfirmasi valid.",
				pegawaiID,
				namaPegawai,
			)

			// Lanjut ke Step 6 (IMPLEMENTASI)
			config.DB.Model(&models.TrackingPenawaran{}).
				Where("id = ?", trackingID).
				Updates(map[string]interface{}{
					"step_saat_ini": models.StepImplementasi,
					"status":        models.StatusOnProgress,
				})

		case "PERLU_TINDAKAN":
			// Manager Operasional rejects the feedback
			if !isManagerOps && !isMaster {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "Hanya Manager Operasional atau Master yang bisa menolak feedback ini.",
				})
			}
			if body.Alasan == "" {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Alasan penolakan wajib diisi.",
				})
			}
			if followUp.Stage >= 3 {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Tahap follow up sudah selesai, tidak bisa ditolak.",
				})
			}

			followUp.Status = models.StatusPerluTindakan
			appendFollowUpLog(
				&followUp,
				"Feedback Customer Perlu Tindakan: "+body.Alasan,
				body.Alasan,
				pegawaiID,
				namaPegawai,
			)

			// Update parent tracking status to PERLU_TINDAKAN
			config.DB.Model(&models.TrackingPenawaran{}).
				Where("id = ?", trackingID).
				Update("status", models.StatusPerluTindakan)

		case "ON_PROGRESS":
			// Sales PIC re-confirms (konfirmasi ulang)
			isSalesPIC := pegawaiID == followUp.TrackingPenawaran.MarketingID
			if !isSalesPIC && !isManagerOps && !isMaster {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "Hanya Sales pembuat penawaran, Manager Operasional, atau Master yang bisa konfirmasi ulang.",
				})
			}
			if followUp.Status != models.StatusPerluTindakan {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Status bukan Perlu Tindakan.",
				})
			}

			followUp.Status = models.StatusOnProgress
			appendFollowUpLog(
				&followUp,
				"Konfirmasi Ulang Follow Up",
				"Sales mengonfirmasi ulang dan memproses kembali follow up.",
				pegawaiID,
				namaPegawai,
			)

			// Update parent tracking status to ON_PROGRESS
			config.DB.Model(&models.TrackingPenawaran{}).
				Where("id = ?", trackingID).
				Update("status", models.StatusOnProgress)

		default:
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Status tidak valid."})
		}
	} else {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Stage atau Status wajib diisi."})
	}

	updated, err := preloadFollowUp(trackingID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data terbaru."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Status follow up berhasil diupdate.",
		"data":    updated,
	})
}

// ── Upload Dokumen ─────────────────────────────────────────────────────────

func UploadDokumenFollowUp(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, roleStr, divisiStr, ok := getFollowUpClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var followUp models.FollowUp
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&followUp).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"message": "Data Follow Up tidak ditemukan untuk penawaran ini",
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
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Tipe file tidak diizinkan",
		})
	}

	const maxSize = 10 << 20
	if file.Size > maxSize {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Ukuran file maksimal 10MB",
		})
	}

	uploadDir := getUploadDir()
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal membuat folder upload",
		})
	}

	uniqueID := uuid.New().String()
	safeOriginal := sanitizeFilename(strings.TrimSuffix(file.Filename, ext))
	newFilename := fmt.Sprintf("%s_%s%s", uniqueID, safeOriginal, ext)
	destPath := filepath.Join(uploadDir, newFilename)

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal membuka file",
		})
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal menyimpan file",
		})
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
	kategori := c.FormValue("kategori")

	if kategori == "DOKUMEN_PO_PGA" || kategori == "DOKUMEN_PO_FINANCE" {
		if divisiStr != "MAINTENANCE_PAC" && divisiStr != "MAINTENANCE_FIRE" && roleStr != "MASTER" && divisiStr != "MANAGER_OPERASIONAL" {
			os.Remove(destPath)
			return c.JSON(http.StatusForbidden, map[string]string{
				"message": "Anda tidak memiliki izin untuk mengunggah dokumen PO Khusus.",
			})
		}
	}

	dokumen := models.PenawaranDokumen{
		ID:         uuid.New().String(),
		NamaFile:   file.Filename,
		Path:       filePath,
		Kategori:   kategori,
		UploadedBy: pegawaiID,
		FollowUpID: &followUp.ID,
		CreatedAt:  time.Now(),
	}
	if err := config.DB.Create(&dokumen).Error; err != nil {
		os.Remove(destPath)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal menyimpan data dokumen",
		})
	}

	appendFollowUpLog(&followUp, "Upload Dokumen", "Menambahkan dokumen **"+file.Filename+"**", pegawaiID, namaPegawai)

	config.DB.Preload("Pegawai").First(&dokumen, "id = ?", dokumen.ID)

	// Auto-transition to StepImplementasi if both Admin Proyek PO documents are uploaded
	if followUp.Stage == 3 && (kategori == "DOKUMEN_PO_PGA" || kategori == "DOKUMEN_PO_FINANCE") {
		var allDocs []models.PenawaranDokumen
		config.DB.Where("follow_up_id = ?", followUp.ID).Find(&allDocs)

		hasPGA := false
		hasFinance := false
		for _, d := range allDocs {
			if d.Kategori == "DOKUMEN_PO_PGA" {
				hasPGA = true
			}
			if d.Kategori == "DOKUMEN_PO_FINANCE" {
				hasFinance = true
			}
		}

		if hasPGA && hasFinance {
			followUp.Status = models.StatusSelesai
			appendFollowUpLog(&followUp, "Follow Up Selesai", "Semua dokumen PO telah diunggah oleh Admin Proyek. Melanjutkan ke tahap Implementasi.", "system", "System")
			config.DB.Save(&followUp)

			// Update TrackingPenawaran step
			config.DB.Model(&models.TrackingPenawaran{}).Where("id = ?", followUp.TrackingPenawaranID).
				Update("step_saat_ini", models.StepImplementasi)

			// Create Implementasi entry
			implementasi := models.Implementasi{
				ID:                  uuid.New().String(),
				TrackingPenawaranID: followUp.TrackingPenawaranID,
				Status:              models.StatusOnProgress,
			}
			config.DB.Create(&implementasi)

			// Update ActivityAdminProyek status
			if followUp.ActivityAdminProyekID != nil {
				config.DB.Model(&models.Activity{}).Where("id = ?", *followUp.ActivityAdminProyekID).
					Updates(map[string]interface{}{
						"status": models.StatusSelesai,
						"kpi":    "BAIK",
					})
			}
		}
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "File berhasil diunggah",
		"data":    dokumen,
	})
}

// ── Delete Dokumen ─────────────────────────────────────────────────────────

func DeleteDokumenFollowUp(c echo.Context) error {
	trackingID := c.Param("id")
	dokumenID := c.Param("dokumenId")

	pegawaiID, namaPegawai, _, _, ok := getFollowUpClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var followUp models.FollowUp
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&followUp).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"message": "Data Follow Up tidak ditemukan",
		})
	}

	var dokumen models.PenawaranDokumen
	if err := config.DB.Where("id = ? AND follow_up_id = ?", dokumenID, followUp.ID).First(&dokumen).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"message": "Dokumen tidak ditemukan",
		})
	}

	// Hanya pengupload asli yang boleh menghapus dokumen
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
			"message": "Gagal menghapus dokumen",
		})
	}

	appendFollowUpLog(&followUp, "Hapus Dokumen", "Menghapus dokumen **"+dokumen.NamaFile+"**", pegawaiID, namaPegawai)

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Dokumen berhasil dihapus",
	})
}

func appendFollowUpLog(followUp *models.FollowUp, aksi, keterangan, pegawaiID, namaPegawai string) {
	log := models.LogFollowUp{
		Aksi:        aksi,
		Keterangan:  keterangan,
		PegawaiID:   pegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   time.Now(),
	}
	followUp.LogAktivitas = append(followUp.LogAktivitas, log)
	config.DB.Save(followUp)
}
