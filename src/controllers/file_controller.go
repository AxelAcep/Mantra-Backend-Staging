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

// UploadDokumen handles POST /activity/:id/dokumen
func UploadDokumen(c echo.Context) error {
	// gunakan config.DB langsung seperti controller lain
	activityID := c.Param("id")

	// Ambil claims dari token (MapClaims)
	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)

	// Cek activity & status
	var activity models.Activity
	if err := config.DB.First(&activity, "id = ?", activityID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"message": "Activity tidak ditemukan"})
	}

	// Block jika status sudah selesai
	if activity.Status == models.StatusDiterima || activity.Status == models.StatusDibatalkan {
		return c.JSON(http.StatusForbidden, map[string]string{
			"message": "Tidak dapat mengunggah file, aktivitas sudah selesai",
		})
	}

	// Parse multipart form secara eksplisit (max 10MB)
	if err := c.Request().ParseMultipartForm(10 << 20); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Gagal parse form: " + err.Error()})
	}

	// Ambil file dari form
	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "File tidak ditemukan: " + err.Error()})
	}

	// Validasi ekstensi
	allowedExt := map[string]bool{
		// Dokumen
		".pdf": true, ".doc": true, ".docx": true,
		".xls": true, ".xlsx": true,
		".ppt": true, ".pptx": true,
		".txt": true, ".csv": true,
		// Gambar
		".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".webp": true, ".svg": true,
		// Arsip
		".zip": true, ".rar": true,
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedExt[ext] {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Tipe file tidak diizinkan"})
	}

	// Validasi ukuran (max 10MB)
	const maxSize = 10 << 20
	if file.Size > maxSize {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Ukuran file maksimal 10MB"})
	}

	// Buat folder uploads jika belum ada
	uploadDir := getUploadDir()
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal membuat folder upload"})
	}

	// Generate nama file unik
	uniqueID := uuid.New().String()
	safeOriginal := sanitizeFilename(strings.TrimSuffix(file.Filename, ext))
	newFilename := fmt.Sprintf("%s_%s%s", uniqueID, safeOriginal, ext)
	destPath := filepath.Join(uploadDir, newFilename)

	// Buka dan simpan file
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
		n, err := src.Read(buf)
		if n > 0 {
			dst.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	// Path yang disimpan di DB dan dikembalikan ke frontend
	// Format: /uploads/<filename> — bisa langsung diakses via static serving
	filePath := "/uploads/" + newFilename

	// Simpan ke DB
	dokumen := models.ActivityDokumen{
		ID:         uuid.New().String(),
		ActivityID: activityID,
		NamaFile:   file.Filename,
		Path:       filePath,
		UploadedBy: pegawaiID,
		CreatedAt:  time.Now(),
	}
	if err := config.DB.Create(&dokumen).Error; err != nil {
		// Hapus file jika DB gagal
		os.Remove(destPath)
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal menyimpan data dokumen"})
	}

	// Load relasi pegawai untuk response
	config.DB.Preload("Pegawai").First(&dokumen, "id = ?", dokumen.ID)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "File berhasil diunggah",
		"data":    dokumen,
	})
}

// DeleteDokumen handles DELETE /activity/:id/dokumen/:dokumenId
func DeleteDokumen(c echo.Context) error {
	// gunakan config.DB langsung seperti controller lain
	activityID := c.Param("id")
	dokumenID := c.Param("dokumenId")

	// Cek activity & status
	var activity models.Activity
	if err := config.DB.First(&activity, "id = ?", activityID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"message": "Activity tidak ditemukan"})
	}

	if activity.Status == models.StatusDiterima || activity.Status == models.StatusDibatalkan {
		return c.JSON(http.StatusForbidden, map[string]string{
			"message": "Tidak dapat menghapus file, aktivitas sudah selesai",
		})
	}

	// Cari dokumen
	var dokumen models.ActivityDokumen
	if err := config.DB.First(&dokumen, "id = ? AND activity_id = ?", dokumenID, activityID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"message": "Dokumen tidak ditemukan"})
	}

	// Hapus file fisik
	uploadDir := getUploadDir()
	filename := strings.TrimPrefix(dokumen.Path, "/uploads/")
	filePath := filepath.Join(uploadDir, filename)
	os.Remove(filePath) // best-effort, tidak gagalkan request jika file sudah tidak ada

	// Hapus dari DB
	if err := config.DB.Delete(&dokumen).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal menghapus dokumen"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Dokumen berhasil dihapus"})
}

// getUploadDir membaca dari env UPLOAD_DIR, fallback ke ./uploads
// Ini memudahkan konfigurasi Docker (mount volume ke path manapun)
func getUploadDir() string {
	if dir := os.Getenv("UPLOAD_DIR"); dir != "" {
		return dir
	}
	return "./uploads"
}

// sanitizeFilename membersihkan karakter berbahaya dari nama file
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		" ", "_", "/", "", "\\", "", "..", "",
		"<", "", ">", "", ":", "", "\"", "",
		"|", "", "?", "", "*", "",
	)
	result := replacer.Replace(name)
	if len(result) > 50 {
		result = result[:50]
	}
	return result
}
