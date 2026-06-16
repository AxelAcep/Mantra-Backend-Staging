package controllers

import (
	"math"
	"net/http"
	"strconv"

	"mantra/src/config"
	"mantra/src/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// GetNotifikasiList mengambil daftar notifikasi pegawai yang login saat ini
func GetNotifikasiList(c echo.Context) error {
	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	pegawaiMap, ok := claims["pegawai"].(map[string]interface{})
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiID, _ := pegawaiMap["id"].(string)

	page := 1
	limit := 10

	if p := c.QueryParam("page"); p != "" {
		if val, err := strconv.Atoi(p); err == nil && val > 0 {
			page = val
		}
	}
	if l := c.QueryParam("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	offset := (page - 1) * limit

	filter := c.QueryParam("filter")
	if filter == "daily-activity" {
		role, _ := claims["role"].(string)

		activityQuery := config.DB.Model(&models.Activity{}).
			Where("status = 'ON_PROGRESS' AND target_selesai < NOW()")

		if role == "MASTER" {
			// Master melihat semua overdue
		} else if role == "SUPERVISI" {
			pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
			divisi, _ := pegawaiMap["divisi"].(string)
			activityQuery = activityQuery.
				Joins("JOIN \"Pegawai\" ON \"Pegawai\".id = \"Activity\".pegawai_id").
				Where("\"Pegawai\".divisi = ?", divisi)
		} else {
			// KARYAWAN melihat overdue miliknya sendiri
			activityQuery = activityQuery.Where("pegawai_id = ?", pegawaiID)
		}

		var total int64
		if err := activityQuery.Count(&total).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
		}

		var activities []models.Activity
		if err := activityQuery.
			Preload("Pegawai").
			Preload("Parent").
			Preload("Parent.Pegawai").
			Order("\"Activity\".target_selesai DESC").
			Limit(limit).
			Offset(offset).
			Find(&activities).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
		}

		notifikasi := make([]models.Notifikasi, len(activities))
		for i, act := range activities {
			actCopy := act
			notifikasi[i] = models.Notifikasi{
				ID:         "VNTF-" + act.ID,
				PegawaiID:  pegawaiID,
				ActivityID: &actCopy.ID,
				Activity:   &actCopy,
				Judul:      act.Judul,
				Pesan:      "Daily Activity ini telah melewati batas waktu selesai (Overdue)",
				IsRead:     false,
				CreatedAt:  act.CreatedAt,
			}
		}

		totalPages := int(math.Ceil(float64(total) / float64(limit)))

		return c.JSON(http.StatusOK, map[string]interface{}{
			"message": "Data notifikasi berhasil diambil.",
			"data":    notifikasi,
			"meta": map[string]interface{}{
				"page":       page,
				"limit":      limit,
				"total":      total,
				"totalPages": totalPages,
			},
		})
	}

	var countQuery *gorm.DB
	var findQuery *gorm.DB

	if filter == "penawaran" {
		countQuery = config.DB.Model(&models.Notifikasi{}).Where("1 = 0")
		findQuery = config.DB.Model(&models.Notifikasi{}).Where("1 = 0")
	} else {
		countQuery = config.DB.Model(&models.Notifikasi{}).Where("\"Notifikasi\".pegawai_id = ?", pegawaiID)
		findQuery = config.DB.Model(&models.Notifikasi{}).Where("\"Notifikasi\".pegawai_id = ?", pegawaiID)
	}

	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	var notifikasi []models.Notifikasi
	if err := findQuery.
		Preload("Activity").
		Preload("Activity.Pegawai").
		Preload("Activity.Parent").
		Preload("Activity.Parent.Pegawai").
		Order("\"Notifikasi\".is_read ASC, \"Notifikasi\".created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifikasi).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Data notifikasi berhasil diambil.",
		"data":    notifikasi,
		"meta": map[string]interface{}{
			"page":       page,
			"limit":      limit,
			"total":      total,
			"totalPages": totalPages,
		},
	})
}

// GetUnreadNotifikasiCount menghitung total notifikasi yang belum dibaca
func GetUnreadNotifikasiCount(c echo.Context) error {
	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	pegawaiMap, ok := claims["pegawai"].(map[string]interface{})
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiID, _ := pegawaiMap["id"].(string)

	filter := c.QueryParam("filter")

	var count int64
	if filter == "penawaran" {
		count = 0
	} else if filter == "" || filter == "daily-activity" {
		role, _ := claims["role"].(string)

		activityQuery := config.DB.Model(&models.Activity{}).
			Where("status = 'ON_PROGRESS' AND target_selesai < NOW()")

		if role == "MASTER" {
			// Master melihat semua
		} else if role == "SUPERVISI" {
			pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
			divisi, _ := pegawaiMap["divisi"].(string)
			activityQuery = activityQuery.
				Joins("JOIN \"Pegawai\" ON \"Pegawai\".id = \"Activity\".pegawai_id").
				Where("\"Pegawai\".divisi = ?", divisi)
		} else {
			// KARYAWAN
			activityQuery = activityQuery.Where("pegawai_id = ?", pegawaiID)
		}

		if err := activityQuery.Count(&count).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menghitung unread notifikasi."})
		}
	} else {
		// Fallback default
		query := config.DB.Model(&models.Notifikasi{}).
			Where("\"Notifikasi\".pegawai_id = ? AND \"Notifikasi\".is_read = false", pegawaiID)
		if err := query.Count(&count).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menghitung unread notifikasi."})
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"unreadCount": count,
	})
}

// ReadNotifikasi menandai satu notifikasi sebagai telah dibaca
func ReadNotifikasi(c echo.Context) error {
	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	pegawaiMap, ok := claims["pegawai"].(map[string]interface{})
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiID, _ := pegawaiMap["id"].(string)

	id := c.Param("id")

	if err := config.DB.Model(&models.Notifikasi{}).
		Where("id = ? AND pegawai_id = ?", id, pegawaiID).
		Update("is_read", true).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal memperbarui status notifikasi."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Notifikasi ditandai sebagai dibaca.",
	})
}

// ReadAllNotifikasi menandai seluruh notifikasi pegawai sebagai telah dibaca
func ReadAllNotifikasi(c echo.Context) error {
	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	pegawaiMap, ok := claims["pegawai"].(map[string]interface{})
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiID, _ := pegawaiMap["id"].(string)

	if err := config.DB.Model(&models.Notifikasi{}).
		Where("pegawai_id = ? AND is_read = false", pegawaiID).
		Update("is_read", true).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal memperbarui semua status notifikasi."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Semua notifikasi ditandai sebagai dibaca.",
	})
}
