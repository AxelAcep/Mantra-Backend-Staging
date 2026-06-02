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

func getReviewClaims(c echo.Context) (pegawaiID, namaPegawai, roleStr, divisiStr string, ok bool) {
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

func preloadReviewInternal(trackingID string) (models.ReviewInternal, error) {
    var review models.ReviewInternal
    err := config.DB.
        Where("tracking_penawaran_id = ?", trackingID).
        Preload("TrackingPenawaran.Perusahaan").
        Preload("TrackingPenawaran.Marketing").
        Preload("Dokumen").
        First(&review).Error
    return review, err
}

// ── Get Detail ─────────────────────────────────────────────────────────────

func GetDetailReviewInternal(c echo.Context) error {
    trackingID := c.Param("id")

    _, _, _, _, ok := getReviewClaims(c)
    if !ok {
        return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
    }

    review, err := preloadReviewInternal(trackingID)
    if err != nil {
        return c.JSON(http.StatusNotFound, map[string]string{"error": "Review Internal tidak ditemukan."})
    }

    return c.JSON(http.StatusOK, review)
}

// ── Update Status ──────────────────────────────────────────────────────────

func UpdateStatusReviewInternal(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, roleStr, divisiStr, ok := getReviewClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Unauthorized.",
		})
	}

	var body struct {
		Status string `json:"status"`
		Alasan string `json:"alasanPenolakan"`
	}

	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid body.",
		})
	}

	var review models.ReviewInternal

	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		First(&review).Error; err != nil {

		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Review Internal tidak ditemukan.",
		})
	}

	isAdminSekertariat := divisiStr == "ADMIN_SEKERTARIAT"
	isManajerOps := divisiStr == "MANAGER_OPERASIONAL"

	isSalesPresalesSupervisi :=
		divisiStr == "SALES" ||
			divisiStr == "PRESALES" ||
			(roleStr == "SUPERVISI" && divisiStr == "SALES")

	switch body.Status {

	case "ACC":

		if !isAdminSekertariat && !isManajerOps {
			return c.JSON(http.StatusForbidden, map[string]string{
				"error": "Hanya Admin Sekertariat atau Manajer Operasional yang bisa acc.",
			})
		}

		if review.Status == models.StatusPerluTindakan {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Status perlu tindakan, harus dikonfirmasi ulang dulu.",
			})
		}

		if isAdminSekertariat {
			review.AccAdminDirektur = true

			appendReviewInternalLog(
				&review,
				"ACC Admin Sekertariat",
				"Review Internal disetujui oleh Admin Sekertariat",
				pegawaiID,
				namaPegawai,
			)
		}

		if isManajerOps {
			review.AccManajerOps = true

			appendReviewInternalLog(
				&review,
				"ACC Manager Operasional",
				"Review Internal disetujui oleh Manager Operasional",
				pegawaiID,
				namaPegawai,
			)
		}

		// Kalau dua-duanya sudah acc
		if review.AccAdminDirektur && review.AccManajerOps {

			review.Status = models.StatusSelesai

			appendReviewInternalLog(
				&review,
				"Review Internal Selesai",
				"Semua approval selesai, lanjut ke Persetujuan Manajemen",
				pegawaiID,
				namaPegawai,
			)

			config.DB.Save(&review)

			config.DB.Model(&models.TrackingPenawaran{}).
				Where("id = ?", trackingID).
				Updates(map[string]interface{}{
					"step_saat_ini": models.StepPersetujuanManajemen,
					"status":        models.StatusOnProgress,
				})

			var existingPersetujuan models.PersetujuanManajemen

			persetujuanExists :=
				config.DB.
					Where("tracking_penawaran_id = ?", trackingID).
					First(&existingPersetujuan).Error == nil

			if !persetujuanExists {

				persetujuan := models.PersetujuanManajemen{
					ID:                   uuid.New().String(),
					TrackingPenawaranID:  trackingID,
					AccDirekturKomisaris: false,
					Status:               models.StatusOnProgress,
					CreatedAt:            time.Now(),
					UpdatedAt:            time.Now(),
				}

				if err := config.DB.Create(&persetujuan).Error; err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{
						"error": "Gagal membuat Persetujuan Manajemen.",
					})
				}
			}

		} else {

			config.DB.Save(&review)
		}

	case "PERLU_TINDAKAN":

		if !isAdminSekertariat && !isManajerOps {
			return c.JSON(http.StatusForbidden, map[string]string{
				"error": "Hanya Admin Sekertariat atau Manajer Operasional yang bisa menolak.",
			})
		}

		if body.Alasan == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Alasan wajib diisi.",
			})
		}

		review.Status = models.StatusPerluTindakan

		appendReviewInternalLog(
			&review,
			"Perlu Tindakan: "+body.Alasan,
			body.Alasan,
			pegawaiID,
			namaPegawai,
		)

		config.DB.Save(&review)

		config.DB.Model(&models.TrackingPenawaran{}).
			Where("id = ?", trackingID).
			Update("status", models.StatusPerluTindakan)

	case "ON_PROGRESS":

		if !isSalesPresalesSupervisi {
			return c.JSON(http.StatusForbidden, map[string]string{
				"error": "Hanya Sales, Presales, atau Supervisi yang bisa konfirmasi ulang.",
			})
		}

		if review.Status != models.StatusPerluTindakan {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Status bukan Perlu Tindakan.",
			})
		}

		review.Status = models.StatusOnProgress

		appendReviewInternalLog(
			&review,
			"Konfirmasi Ulang",
			"Review Internal dikonfirmasi ulang dan diproses kembali",
			pegawaiID,
			namaPegawai,
		)

		config.DB.Save(&review)

		config.DB.Model(&models.TrackingPenawaran{}).
			Where("id = ?", trackingID).
			Update("status", models.StatusOnProgress)

	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Status tidak valid.",
		})
	}

	updated, err := preloadReviewInternal(trackingID)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Gagal mengambil data terbaru.",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Status berhasil diupdate.",
		"data":    updated,
	})
}


func UploadDokumenReviewInternal(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, namaPegawai, _, _, ok := getReviewClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var reviewInternal models.ReviewInternal
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&reviewInternal).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Review Internal tidak ditemukan untuk penawaran ini.",
		})
	}

	if err := c.Request().ParseMultipartForm(10 << 20); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Gagal parse form: " + err.Error(),
		})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "File tidak ditemukan: " + err.Error(),
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
			"error": "Tipe file tidak diizinkan.",
		})
	}

	const maxSize = 10 << 20
	if file.Size > maxSize {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Ukuran file maksimal 10MB.",
		})
	}

	uploadDir := getUploadDir()
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Gagal membuat folder upload.",
		})
	}

	uniqueID := uuid.New().String()
	safeOriginal := sanitizeFilename(strings.TrimSuffix(file.Filename, ext))
	newFilename := fmt.Sprintf("%s_%s%s", uniqueID, safeOriginal, ext)
	destPath := filepath.Join(uploadDir, newFilename)

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Gagal membuka file.",
		})
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Gagal menyimpan file.",
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

	dokumen := models.PenawaranDokumen{
		ID:               uuid.New().String(),
		NamaFile:         file.Filename,
		Path:             "/uploads/" + newFilename,
		UploadedBy:       pegawaiID,
		ReviewInternalID: &reviewInternal.ID,
		CreatedAt:        time.Now(),
	}
	if err := config.DB.Create(&dokumen).Error; err != nil {
		os.Remove(destPath)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Gagal menyimpan data dokumen Review Internal.",
		})
	}

	appendReviewInternalLog(&reviewInternal, "Upload Dokumen", "Menambahkan dokumen **"+file.Filename+"**", pegawaiID, namaPegawai)

	config.DB.Preload("Pegawai").First(&dokumen, "id = ?", dokumen.ID)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "File Review Internal berhasil diunggah.",
		"data":    dokumen,
	})
}

func DeleteDokumenReviewInternal(c echo.Context) error {
	trackingID := c.Param("id")
	dokumenID := c.Param("dokumenId")

	pegawaiID, namaPegawai, _, _, ok := getReviewClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var reviewInternal models.ReviewInternal
	if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&reviewInternal).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Review Internal tidak ditemukan.",
		})
	}

	var dokumen models.PenawaranDokumen
	if err := config.DB.Where("id = ? AND review_internal_id = ?", dokumenID, reviewInternal.ID).First(&dokumen).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Dokumen Review Internal tidak ditemukan.",
		})
	}

	uploadDir := getUploadDir()
	filename := strings.TrimPrefix(dokumen.Path, "/uploads/")
	filePath := filepath.Join(uploadDir, filename)
	os.Remove(filePath)

	if err := config.DB.Delete(&dokumen).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Gagal menghapus dokumen Review Internal.",
		})
	}

	appendReviewInternalLog(&reviewInternal, "Hapus Dokumen", "Menghapus dokumen **"+dokumen.NamaFile+"**", pegawaiID, namaPegawai)

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Dokumen Review Internal berhasil dihapus.",
	})
}

func appendReviewInternalLog(reviewInternal *models.ReviewInternal, aksi, keterangan, pegawaiID, namaPegawai string) {
    log := models.LogReviewInternal{
        Aksi:        aksi,
        Keterangan:  keterangan,
        PegawaiID:   pegawaiID,
        NamaPegawai: namaPegawai,
        CreatedAt:   time.Now(),
    }
    reviewInternal.LogAktivitas = append(reviewInternal.LogAktivitas, log)
    config.DB.Save(reviewInternal)
}