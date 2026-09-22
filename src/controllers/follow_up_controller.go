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
		Preload("AdminProyek").
		Preload("ActivityAdmin.Pegawai").
		Preload("ActivitySales.Pegawai").
		Preload("ActivityAdminProyek.Pegawai").
		Preload("ActivityPengecekanAdminProyek.Pegawai").
		Preload("ActivityPengecekanFinance.Pegawai").
		Preload("Dokumen").
		Preload("Dokumen.Pegawai").
		First(&followUp).Error
	return followUp, err
}

// ── Get Detail ─────────────────────────────────────────────────────────────

func GetDetailFollowUp(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, _, roleStr, divisiStr, ok := getFollowUpClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	if !canViewStepForTracking(models.StepFollowUp, roleStr, divisiStr, pegawaiID, trackingID) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak."})
	}

	// Catatan: dulu ada fallback yang auto-bikin FollowUp + daily Admin
	// Sekretariat di sini kalau belum ketemu ("kompatibilitas data lama").
	// Ini GET handler dan gak boleh punya side effect nulis DB — kalau
	// dipanggil dobel (retry, dua tab, dsb) sebelum baris pertama commit,
	// dua-duanya lolos pengecekan "belum ada" dan masing-masing bikin daily
	// Admin Sekretariat sendiri -> duplikat. FollowUp sekarang selalu dibuat
	// di alur normal (UpdateStatusPersetujuanManajemen, case SELESAI) sebelum
	// step_saat_ini pindah ke StepFollowUp, jadi fallback ini gak perlu lagi.
	followUp, err := preloadFollowUp(trackingID)
	if err != nil {
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

	if followUp.Status == models.StatusDibatalkan {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Permintaan penawaran ini sudah dibatalkan, tidak bisa diproses lagi.",
		})
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

// ── Batalkan Permintaan Penawaran ──────────────────────────────────────────
// Tombol "Batalkan Permintaan Penawaran" di step Follow Up — cuma boleh
// diakses Manager Operasional, Direktur, atau Komisaris, dan cuma bisa
// dipencet selama FollowUp masih ON_PROGRESS. Setelah dibatalkan, tracking
// berhenti permanen di step ini (step_saat_ini gak pernah pindah lagi karena
// gak ada lagi aksi lanjutan yang bisa dilakukan atas FollowUp yang DIBATALKAN).

func BatalkanFollowUp(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, roleStr, divisiStr, ok := getFollowUpClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		Alasan string `json:"alasan"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}
	if strings.TrimSpace(body.Alasan) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Alasan pembatalan wajib diisi."})
	}

	isMaster := roleStr == "MASTER"
	isBerwenang := divisiStr == "MANAGER_OPERASIONAL" || divisiStr == "DIREKTUR" || divisiStr == "KOMISARIS"
	if !isBerwenang && !isMaster {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Hanya Manager Operasional, Direktur, atau Komisaris yang bisa membatalkan permintaan penawaran.",
		})
	}

	var followUp models.FollowUp
	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		First(&followUp).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Follow Up tidak ditemukan."})
	}

	if followUp.Status != models.StatusOnProgress {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Permintaan penawaran cuma bisa dibatalkan selagi Follow Up masih On Progress.",
		})
	}

	followUp.Status = models.StatusDibatalkan
	appendFollowUpLog(
		&followUp,
		"Case Closed",
		"Case Closed oleh "+namaPegawai+". Alasan: "+body.Alasan,
		pegawaiID,
		namaPegawai,
	)

	if err := config.DB.Save(&followUp).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal membatalkan permintaan penawaran."})
	}

	if err := config.DB.Model(&models.TrackingPenawaran{}).
		Where("id = ?", trackingID).
		Update("status", models.StatusDibatalkan).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengupdate status tracking."})
	}

	updated, err := preloadFollowUp(trackingID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data terbaru."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Permintaan penawaran berhasil dibatalkan.",
		"data":    updated,
	})
}

// ── Input Total BAST ─────────────────────────────────────────────────────

func InputBASTFollowup(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, roleStr, divisiStr, ok := getFollowUpClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		TotalBAST     *int `json:"total_bast"`
		TotalBastPAC  *int `json:"total_bast_pac"`
		TotalBastFire *int `json:"total_bast_fire"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	var tracking models.TrackingPenawaran
	if err := config.DB.Where("id = ?", trackingID).First(&tracking).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Tracking penawaran tidak ditemukan."})
	}

	// PAC & FIRE dua-duanya ada -> butuh 2 input terpisah. Kalau cuma salah
	// satu (atau gak ada dua-duanya) -> tetap 1 input generik (total_bast),
	// sama kayak sebelum ada pemisahan BAST.
	dualKategori := len(models.DetectBastKategori(tracking.JenisPenawaran)) == 2

	if dualKategori {
		if body.TotalBastPAC == nil && body.TotalBastFire == nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "total_bast_pac atau total_bast_fire wajib diisi (minimal salah satu, tracking ini punya PAC & FIRE)."})
		}
		if body.TotalBastPAC != nil && *body.TotalBastPAC < 0 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "total_bast_pac tidak boleh negatif."})
		}
		if body.TotalBastFire != nil && *body.TotalBastFire < 0 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "total_bast_fire tidak boleh negatif."})
		}
	} else {
		if body.TotalBAST == nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "total_bast wajib diisi."})
		}
		if *body.TotalBAST < 0 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "total_bast tidak boleh negatif."})
		}
	}

	var followUp models.FollowUp
	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		First(&followUp).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Follow Up tidak ditemukan."})
	}

	if followUp.Status == models.StatusDibatalkan {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Permintaan penawaran ini sudah dibatalkan, tidak bisa diproses lagi.",
		})
	}

	if followUp.ActivityAdminProyekID == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Dokumen PO belum dikonfirmasi Direktur/Komisaris. Tunggu konfirmasi dulu sebelum input Total BAST.",
		})
	}

	isManagerOps := divisiStr == "MANAGER_OPERASIONAL"
	isDirekturKomisaris := divisiStr == "DIREKTUR" || divisiStr == "KOMISARIS"
	isMaster := roleStr == "MASTER"
	isAdminProyek := isAssignedAdminProyek(pegawaiID, trackingID)

	if !isManagerOps && !isDirekturKomisaris && !isMaster && !isAdminProyek {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Hanya Admin Proyek, Manager Operasional, Direktur, Komisaris, atau Master yang bisa menginput Total BAST.",
		})
	}

	var changes []string
	if dualKategori {
		if body.TotalBastPAC != nil {
			followUp.TotalBastPAC = body.TotalBastPAC
			changes = append(changes, fmt.Sprintf("Total BAST PAC diperbarui menjadi %d", *body.TotalBastPAC))
		}
		if body.TotalBastFire != nil {
			followUp.TotalBastFire = body.TotalBastFire
			changes = append(changes, fmt.Sprintf("Total BAST FIRE diperbarui menjadi %d", *body.TotalBastFire))
		}
	} else {
		followUp.TotalBAST = body.TotalBAST
		changes = append(changes, fmt.Sprintf("Total BAST diperbarui menjadi %d", *body.TotalBAST))
	}

	appendFollowUpLog(
		&followUp,
		"Total BAST Diinput",
		strings.Join(changes, ", ")+".",
		pegawaiID,
		namaPegawai,
	)

	if err := config.DB.Save(&followUp).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menyimpan Total BAST."})
	}

	updated, err := preloadFollowUp(trackingID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data terbaru."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Total BAST berhasil diupdate.",
		"data":    updated,
	})
}

// ── Set Kondisi Pengantaran ────────────────────────────────────────────────

func SetKondisiPengantaran(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, roleStr, divisiStr, ok := getFollowUpClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		KondisiPengantaran string `json:"kondisiPengantaran"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	if body.KondisiPengantaran != "SEBELUM_DP" && body.KondisiPengantaran != "SESUDAH_DP" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "kondisiPengantaran harus 'SEBELUM_DP' atau 'SESUDAH_DP'.",
		})
	}

	var followUp models.FollowUp
	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		First(&followUp).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Follow Up tidak ditemukan."})
	}

	if followUp.Status == models.StatusDibatalkan {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Permintaan penawaran ini sudah dibatalkan, tidak bisa diproses lagi.",
		})
	}

	if followUp.ActivityAdminProyekID == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Admin Proyek belum ditugaskan. Tugaskan Admin Proyek dulu.",
		})
	}

	isManagerOps := divisiStr == "MANAGER_OPERASIONAL"
	isDirekturKomisaris := divisiStr == "DIREKTUR" || divisiStr == "KOMISARIS"
	isMaster := roleStr == "MASTER"
	isAdminProyek := isAssignedAdminProyek(pegawaiID, trackingID)

	if !isManagerOps && !isDirekturKomisaris && !isMaster && !isAdminProyek {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Hanya Admin Proyek, Manager Operasional, Direktur, Komisaris, atau Master yang bisa mengatur kondisi pengantaran.",
		})
	}

	followUp.KondisiPengantaran = &body.KondisiPengantaran

	kondisiLabel := "Diantar Sebelum DP"
	if body.KondisiPengantaran == "SESUDAH_DP" {
		kondisiLabel = "Diantar Setelah Klien DP"
	}

	appendFollowUpLog(
		&followUp,
		"Kondisi Pengantaran Diatur",
		"Kondisi pengantaran barang diubah menjadi: "+kondisiLabel+".",
		pegawaiID,
		namaPegawai,
	)

	if err := config.DB.Save(&followUp).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menyimpan kondisi pengantaran."})
	}

	updated, err := preloadFollowUp(trackingID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data terbaru."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Kondisi pengantaran berhasil diupdate.",
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
	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Preload("TrackingPenawaran").
		Preload("TrackingPenawaran.Perusahaan").
		First(&followUp).Error; err != nil {
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
		isManagerOps := divisiStr == "MANAGER_OPERASIONAL"
		isDirekturKomisaris := divisiStr == "DIREKTUR" || divisiStr == "KOMISARIS"
		isMaster := roleStr == "MASTER"
		isAdminProyek := isAssignedAdminProyek(pegawaiID, trackingID)

		if !isManagerOps && !isDirekturKomisaris && !isMaster && !isAdminProyek {
			os.Remove(destPath)
			return c.JSON(http.StatusForbidden, map[string]string{
				"message": "Hanya Admin Proyek, Manager Operasional, Direktur, Komisaris, atau Master yang bisa mengunggah dokumen PO.",
			})
		}

		if followUp.ActivityAdminProyekID == nil {
			os.Remove(destPath)
			return c.JSON(http.StatusBadRequest, map[string]string{
				"message": "Dokumen PO belum dikonfirmasi Direktur/Komisaris. Tunggu konfirmasi dan input Total BAST dulu sebelum upload dokumen PO.",
			})
		}

		var tracking models.TrackingPenawaran
		config.DB.Where("id = ?", trackingID).First(&tracking)
		dualKategori := len(models.DetectBastKategori(tracking.JenisPenawaran)) == 2
		bastFilled := followUp.TotalBAST != nil
		if dualKategori {
			bastFilled = followUp.TotalBastPAC != nil && followUp.TotalBastFire != nil
		}
		if !bastFilled {
			os.Remove(destPath)
			return c.JSON(http.StatusBadRequest, map[string]string{
				"message": "Total BAST belum diisi. Input Total BAST dulu sebelum upload dokumen PO.",
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
	if followUp.Stage == 6 && (kategori == "DOKUMEN_PO_PGA" || kategori == "DOKUMEN_PO_FINANCE") {
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

			appendFollowUpLog(
				&followUp,
				"Follow Up Selesai",
				"Semua dokumen PO telah diunggah oleh Admin Proyek. Melanjutkan ke tahap Implementasi.",
				"system",
				"System",
			)

			config.DB.Save(&followUp)

			// Update TrackingPenawaran step
			config.DB.
				Model(&models.TrackingPenawaran{}).
				Where("id = ?", followUp.TrackingPenawaranID).
				Update("step_saat_ini", models.StepImplementasi)

			// Cari Supervisor PROCUREMENT_GA pertama untuk auto-assign
			// Activity Pembelian Barang
			var pgaSupervisor models.Pegawai

			if err := config.DB.
				Joins(`JOIN "User" ON "User".pegawai_id = "Pegawai".id`).
				Where(
					`"Pegawai".divisi = ? AND "User".role = ?`,
					models.DivisiProcurementGA,
					models.RoleSupervisi,
				).
				First(&pgaSupervisor).Error; err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{
					"message": "Tidak ditemukan Supervisor Procurement GA, gagal membuat Implementasi",
				})
			}

			// 1. Create Implementasi terlebih dahulu
			implementasi := models.Implementasi{
				ID:                  uuid.New().String(),
				TrackingPenawaranID: followUp.TrackingPenawaranID,
				Status:              models.StatusOnProgress,
			}

			if err := config.DB.Create(&implementasi).Error; err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{
					"message": "Gagal membuat Implementasi: " + err.Error(),
				})
			}

			// 2. Setelah Implementasi berhasil dibuat,
			//    create Activity Pembelian Barang untuk Supervisor PGA
			activityPembelianID := uuid.NewString()

			nomorPO := followUp.TrackingPenawaran.NomorPenawaran
			if followUp.TrackingPenawaran.NomorPO != nil && *followUp.TrackingPenawaran.NomorPO != "" {
				nomorPO = *followUp.TrackingPenawaran.NomorPO
			}
			var namaPerusahaan *string
			if followUp.TrackingPenawaran.Perusahaan.Nama != "" {
				namaPerusahaan = &followUp.TrackingPenawaran.Perusahaan.Nama
			}

			activityPembelian := models.Activity{
				ID:            activityPembelianID,
				PegawaiID:     pgaSupervisor.ID,
				TerkaitPO:     &nomorPO,
				Perusahaan:    namaPerusahaan,
				Kategori:      models.KategoriAkomodasiProject,
				Judul:         "Pembelian Barang Implementasi",
				Deskripsi:     "Activity otomatis pembelian barang untuk tahap Implementasi",
				WaktuMulai:    time.Now(),
				TargetSelesai: time.Now().AddDate(0, 0, 2),
				Status:        models.StatusOnProgress,
			}

			if err := config.DB.Create(&activityPembelian).Error; err != nil {
				// Rollback Implementasi jika Activity gagal dibuat
				config.DB.Delete(&models.Implementasi{}, "id = ?", implementasi.ID)

				return c.JSON(http.StatusInternalServerError, map[string]string{
					"message": "Gagal membuat Activity Pembelian Barang: " + err.Error(),
				})
			}

			// 3. Simpan ActivityPembelianID ke Implementasi
			if err := config.DB.
				Model(&models.Implementasi{}).
				Where("id = ?", implementasi.ID).
				Update("activity_pembelian_id", activityPembelianID).Error; err != nil {

				// Rollback Activity + Implementasi jika gagal menyimpan relasi
				config.DB.Delete(&models.Activity{}, "id = ?", activityPembelianID)
				config.DB.Delete(&models.Implementasi{}, "id = ?", implementasi.ID)

				return c.JSON(http.StatusInternalServerError, map[string]string{
					"message": "Gagal menghubungkan Activity Pembelian dengan Implementasi: " + err.Error(),
				})
			}

			// 4. Update ActivityAdminProyek status
			if followUp.ActivityAdminProyekID != nil {
				config.DB.
					Model(&models.Activity{}).
					Where("id = ?", *followUp.ActivityAdminProyekID).
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

// ============================================
// 1. GET Pegawai (RoleSupervisi + Divisi Maintenance PAC/Fire)
// ============================================
func GetPegawaiSupervisiMaintenance(c echo.Context) error {
	type PegawaiSimple struct {
		PegawaiID string `json:"pegawaiId"`
		Nama      string `json:"nama"`
	}

	var pegawaiIDs []string
	if err := config.DB.
		Model(&models.User{}).
		Where("role = ?", models.RoleSupervisi).
		Pluck("pegawai_id", &pegawaiIDs).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data user."})
	}

	var result []PegawaiSimple
	if err := config.DB.
		Model(&models.Pegawai{}).
		Select("id as pegawai_id, nama as nama").
		Where("id IN ?", pegawaiIDs).
		Where("divisi IN ?", []models.Divisi{models.DivisiMaintenancePAC, models.DivisiMaintenanceFire}).
		Where("deleted_at IS NULL").
		Scan(&result).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data pegawai."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"data": result})
}

// ============================================
// 2. Pilih Admin Proyek -> bikin 2 daily "Pengecekan Dokumen PO"
//    (Admin Proyek yang dipilih + Finance Supervisi). Belum bikin daily
//    "Upload Dokumen PO" -- itu baru dibuat Direktur/Komisaris ACC (lihat
//    KonfirmasiDokumenPO).
// ============================================
func AssignAdminProyek(c echo.Context) error {
	pegawaiID, namaPegawai, roleStr, divisiStr, ok := getFollowUpClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	isBerwenang := roleStr == "MASTER" ||
		divisiStr == "MANAGER_OPERASIONAL" ||
		divisiStr == "DIREKTUR" ||
		divisiStr == "KOMISARIS"
	if !isBerwenang {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Hanya Manager Operasional, Direktur, Komisaris, atau Master yang bisa menugaskan Admin Proyek.",
		})
	}

	var body struct {
		FollowUpID string `json:"followUpId"`
		PegawaiID  string `json:"pegawaiId"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}
	if body.FollowUpID == "" || body.PegawaiID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "FollowUpID dan PegawaiID wajib diisi."})
	}

	var followUp models.FollowUp
	if err := config.DB.
		Preload("TrackingPenawaran").
		Where("id = ?", body.FollowUpID).
		First(&followUp).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Follow Up tidak ditemukan."})
	}

	if followUp.Status == models.StatusDibatalkan {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Permintaan penawaran ini sudah dibatalkan, tidak bisa diproses lagi.",
		})
	}

	// Admin Proyek gak boleh ditugaskan sebelum daily Sales (ActivitySalesID)
	// selesai/DITERIMA.
	if followUp.ActivitySalesID == nil || *followUp.ActivitySalesID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Admin Proyek belum bisa ditugaskan, daily Sales belum ada/belum berjalan.",
		})
	}
	var activitySales models.Activity
	if err := config.DB.Where("id = ?", *followUp.ActivitySalesID).First(&activitySales).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Daily Sales tidak ditemukan."})
	}
	if activitySales.Status != models.StatusDiterima {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Admin Proyek belum bisa ditugaskan, daily Sales harus selesai (disetujui) dulu.",
		})
	}

	// Admin Proyek cuma boleh dipilih/diganti selagi masih Stage 3 (belum
	// pernah dipilih) atau Stage 4 (dua daily pengecekan masih berjalan).
	// Begitu Stage udah 5 (nunggu Direktur) atau 6 (upload PO), gak bisa
	// diganti lewat endpoint ini lagi.
	if followUp.Stage < 3 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Follow Up belum sampai tahap pemilihan Admin Proyek.",
		})
	}
	if followUp.Stage > 4 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Admin Proyek untuk tahap ini sudah dikonfirmasi, tidak bisa diganti lagi lewat sini.",
		})
	}

	var pegawai models.Pegawai
	if err := config.DB.Where("id = ?", body.PegawaiID).First(&pegawai).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Pegawai tidak ditemukan."})
	}

	nomorPO := followUp.TrackingPenawaran.NomorPenawaran
	if followUp.TrackingPenawaran.NomorPO != nil && *followUp.TrackingPenawaran.NomorPO != "" {
		nomorPO = *followUp.TrackingPenawaran.NomorPO
	}

	now := time.Now()
	deadline := now.Add(24 * time.Hour)

	tx := config.DB.Begin()
	if tx.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal memulai transaksi."})
	}

	// Kalau Admin Proyek udah pernah dipilih sebelumnya (Stage 4, ganti
	// orang) -> alihkan daily pengecekan yang SAMA ke PIC baru, bukan bikin
	// daily baru numpuk. Daily pengecekan Finance gak ikut berubah.
	if followUp.AdminProyekID != nil && followUp.ActivityPengecekanAdminProyekID != nil {
		var existingActivity models.Activity
		if err := tx.Where("id = ?", *followUp.ActivityPengecekanAdminProyekID).First(&existingActivity).Error; err != nil {
			tx.Rollback()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Daily pengecekan Admin Proyek sebelumnya tidak ditemukan."})
		}

		oldNama := *followUp.AdminProyekID
		var oldPegawai models.Pegawai
		if err := tx.Where("id = ?", *followUp.AdminProyekID).First(&oldPegawai).Error; err == nil {
			oldNama = oldPegawai.Nama
		}

		if err := tx.Model(&models.Activity{}).
			Where("id = ?", existingActivity.ID).
			Update("pegawai_id", body.PegawaiID).Error; err != nil {
			tx.Rollback()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengalihkan daily pengecekan Admin Proyek."})
		}

		logFollowUp := models.LogFollowUp{
			Aksi:        "Ubah Admin Proyek",
			Keterangan:  "Admin Proyek diubah dari " + oldNama + " ke " + pegawai.Nama + " oleh " + namaPegawai + ". Daily pengecekan yang sama dialihkan tanggung jawabnya, bukan dibuat baru.",
			PegawaiID:   pegawaiID,
			NamaPegawai: namaPegawai,
			CreatedAt:   time.Now(),
		}
		followUp.LogAktivitas = append(followUp.LogAktivitas, logFollowUp)

		if err := tx.Model(&models.FollowUp{}).
			Where("id = ?", body.FollowUpID).
			Select("admin_proyek_id", "log_aktivitas").
			Updates(models.FollowUp{
				AdminProyekID: &body.PegawaiID,
				LogAktivitas:  followUp.LogAktivitas,
			}).Error; err != nil {
			tx.Rollback()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal update Follow Up."})
		}

		if err := tx.Commit().Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal commit transaksi."})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"message": "Admin Proyek berhasil dialihkan.",
			"data": map[string]interface{}{
				"activityId": existingActivity.ID,
				"followUpId": followUp.ID,
			},
		})
	}

	// Pertama kali dipilih -> bikin 2 daily pengecekan sekaligus: Admin
	// Proyek yang dipilih, dan Supervisi Finance Accounting.
	financeSupervisor, err := models.FindSupervisiFinanceAccounting(tx)
	if err != nil {
		tx.Rollback()
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	activityAdminProyek := models.Activity{
		ID:            uuid.New().String(),
		PegawaiID:     body.PegawaiID,
		TerkaitPO:     &nomorPO,
		Kategori:      models.KategoriDokumenPendukung,
		Judul:         "Pengecekan Dokumen PO (Admin Proyek)",
		Deskripsi:     "Cek kelengkapan PO customer & kesiapan data terkait penawaran " + nomorPO + " sebelum lanjut ke proses dokumen PO internal.",
		WaktuMulai:    now,
		TargetSelesai: deadline,
		Status:        models.StatusOnProgress,
	}
	if err := tx.Create(&activityAdminProyek).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal membuat daily pengecekan Admin Proyek."})
	}

	activityFinance := models.Activity{
		ID:            uuid.New().String(),
		PegawaiID:     financeSupervisor.ID,
		TerkaitPO:     &nomorPO,
		Kategori:      models.KategoriDokumenPendukung,
		Judul:         "Pengecekan Dokumen PO (Finance)",
		Deskripsi:     "Cek kelengkapan PO customer & kesiapan data terkait penawaran " + nomorPO + " sebelum lanjut ke proses dokumen PO internal.",
		WaktuMulai:    now,
		TargetSelesai: deadline,
		Status:        models.StatusOnProgress,
	}
	if err := tx.Create(&activityFinance).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal membuat daily pengecekan Finance."})
	}

	logFollowUp := models.LogFollowUp{
		Aksi:        "Pilih Admin Proyek",
		Keterangan:  "Admin Proyek dipilih: " + pegawai.Nama + " oleh " + namaPegawai + ". Daily pengecekan dokumen PO dibuat untuk Admin Proyek dan Finance (" + financeSupervisor.Nama + ").",
		PegawaiID:   pegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   time.Now(),
	}
	followUp.LogAktivitas = append(followUp.LogAktivitas, logFollowUp)

	// Select+Updates pakai STRUCT (bukan map) supaya serializer:json di
	// log_aktivitas ke-apply — Updates(map[string]interface{}{...}) ngirim
	// slice mentah ke driver Postgres dan meledak "could not determine data
	// type of parameter".
	if err := tx.Model(&models.FollowUp{}).
		Where("id = ?", body.FollowUpID).
		Select("admin_proyek_id", "activity_pengecekan_admin_proyek_id", "activity_pengecekan_finance_id", "stage", "log_aktivitas").
		Updates(models.FollowUp{
			AdminProyekID:                   &body.PegawaiID,
			ActivityPengecekanAdminProyekID: &activityAdminProyek.ID,
			ActivityPengecekanFinanceID:     &activityFinance.ID,
			Stage:                           4,
			LogAktivitas:                    followUp.LogAktivitas,
		}).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal update Follow Up."})
	}

	if err := tx.Commit().Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal commit transaksi."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Admin Proyek berhasil ditugaskan, daily pengecekan dokumen PO dibuat untuk Admin Proyek & Finance.",
		"data": map[string]interface{}{
			"activityPengecekanAdminProyekId": activityAdminProyek.ID,
			"activityPengecekanFinanceId":     activityFinance.ID,
			"followUpId":                      followUp.ID,
		},
	})
}

// ============================================
// 3. Konfirmasi Dokumen PO oleh Direktur/Komisaris (Stage 5 -> 6)
//    Gak pake daily -- mirip pola Persetujuan Manajemen. Kalau ACC, baru
//    daily "Upload Dokumen PO" buat Admin Proyek dibuat. Kalau ditolak,
//    balik ke Stage 4 dan 2 daily pengecekan direset ON_PROGRESS supaya
//    Admin Proyek & Finance ngulang.
// ============================================
func KonfirmasiDokumenPO(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, _, divisiStr, ok := getFollowUpClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	isBerwenang := divisiStr == "DIREKTUR" || divisiStr == "KOMISARIS"
	if !isBerwenang {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Hanya Direktur atau Komisaris yang bisa konfirmasi dokumen PO.",
		})
	}

	var body struct {
		Status string `json:"status"` // DITERIMA | PERLU_TINDAKAN
		Alasan string `json:"alasan"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}
	if body.Status != string(models.StatusDiterima) && body.Status != string(models.StatusPerluTindakan) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Status harus DITERIMA atau PERLU_TINDAKAN."})
	}
	if body.Status == string(models.StatusPerluTindakan) && strings.TrimSpace(body.Alasan) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Alasan wajib diisi kalau menolak."})
	}

	var followUp models.FollowUp
	if err := config.DB.
		Preload("TrackingPenawaran").
		Where("tracking_penawaran_id = ?", trackingID).
		First(&followUp).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Follow Up tidak ditemukan."})
	}

	if followUp.Stage != 5 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Belum waktunya konfirmasi dokumen PO (dua daily pengecekan harus selesai dulu), atau sudah pernah dikonfirmasi.",
		})
	}
	if followUp.AdminProyekID == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Admin Proyek tidak ditemukan pada Follow Up ini."})
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal memulai transaksi."})
	}

	if body.Status == string(models.StatusDiterima) {
		nomorPO := followUp.TrackingPenawaran.NomorPenawaran
		if followUp.TrackingPenawaran.NomorPO != nil && *followUp.TrackingPenawaran.NomorPO != "" {
			nomorPO = *followUp.TrackingPenawaran.NomorPO
		}

		now := time.Now()
		activityUploadPO := models.Activity{
			ID:            uuid.New().String(),
			PegawaiID:     *followUp.AdminProyekID,
			TerkaitPO:     &nomorPO,
			Kategori:      models.KategoriDokumenPendukung,
			Judul:         "Upload Dokumen PO",
			Deskripsi:     "Mengunggah Dokumen PO untuk PGA dan Finance terkait penawaran " + nomorPO,
			WaktuMulai:    now,
			TargetSelesai: now.Add(24 * time.Hour),
			Status:        models.StatusOnProgress,
		}
		if err := tx.Create(&activityUploadPO).Error; err != nil {
			tx.Rollback()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal membuat daily Upload Dokumen PO."})
		}

		followUp.LogAktivitas = append(followUp.LogAktivitas, models.LogFollowUp{
			Aksi:        "Konfirmasi Dokumen PO",
			Keterangan:  "Dokumen PO dikonfirmasi oleh " + namaPegawai + ". Admin Proyek sekarang bisa input Total BAST dan upload Dokumen PO.",
			PegawaiID:   pegawaiID,
			NamaPegawai: namaPegawai,
			CreatedAt:   now,
		})

		if err := tx.Model(&models.FollowUp{}).
			Where("id = ?", followUp.ID).
			Select("acc_direktur_komisaris_po", "activity_admin_proyek_id", "stage", "log_aktivitas").
			Updates(models.FollowUp{
				AccDirekturKomisarisPO: true,
				ActivityAdminProyekID:  &activityUploadPO.ID,
				Stage:                  6,
				LogAktivitas:           followUp.LogAktivitas,
			}).Error; err != nil {
			tx.Rollback()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal update Follow Up."})
		}
	} else {
		now := time.Now()
		newDeadline := now.Add(24 * time.Hour)

		if followUp.ActivityPengecekanAdminProyekID != nil {
			if err := tx.Model(&models.Activity{}).
				Where("id = ?", *followUp.ActivityPengecekanAdminProyekID).
				Updates(map[string]interface{}{
					"status":         models.StatusOnProgress,
					"target_selesai": newDeadline,
				}).Error; err != nil {
				tx.Rollback()
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal reset daily pengecekan Admin Proyek."})
			}
		}
		if followUp.ActivityPengecekanFinanceID != nil {
			if err := tx.Model(&models.Activity{}).
				Where("id = ?", *followUp.ActivityPengecekanFinanceID).
				Updates(map[string]interface{}{
					"status":         models.StatusOnProgress,
					"target_selesai": newDeadline,
				}).Error; err != nil {
				tx.Rollback()
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal reset daily pengecekan Finance."})
			}
		}

		followUp.LogAktivitas = append(followUp.LogAktivitas, models.LogFollowUp{
			Aksi:        "Dokumen PO Ditolak",
			Keterangan:  "Dokumen PO ditolak oleh " + namaPegawai + ". Alasan: " + body.Alasan + ". Admin Proyek & Finance perlu mengulang daily pengecekan.",
			PegawaiID:   pegawaiID,
			NamaPegawai: namaPegawai,
			CreatedAt:   now,
		})

		if err := tx.Model(&models.FollowUp{}).
			Where("id = ?", followUp.ID).
			Select("stage", "log_aktivitas").
			Updates(models.FollowUp{
				Stage:        4,
				LogAktivitas: followUp.LogAktivitas,
			}).Error; err != nil {
			tx.Rollback()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal update Follow Up."})
		}
	}

	if err := tx.Commit().Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal commit transaksi."})
	}

	updated, err := preloadFollowUp(trackingID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data terbaru."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Konfirmasi dokumen PO berhasil diproses.",
		"data":    updated,
	})
}