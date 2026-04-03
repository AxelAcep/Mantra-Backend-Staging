package controllers

import (
	"net/http"
	"time"

	"mantra/src/config"
	"mantra/src/models"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// Helper to get Week/Month/Year from a time object
func getKPIPeriod(t time.Time) (int, int, int) {
	day := t.Day()
	minggu := (day-1)/7 + 1
	if minggu > 4 {
		minggu = 4
	}
	return minggu, int(t.Month()), t.Year()
}

// UpdateActivityKPI sets or updates the KPI rating for a specific activity
// PATCH /activity/master/:id/kpi
func UpdateActivityKPI(c echo.Context) error {
	id := c.Param("id")

	var body struct {
		Nilai models.NilaiKPI `json:"nilai"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid payload."})
	}

	if body.Nilai != models.NilaiKPIBaik &&
		body.Nilai != models.NilaiKPICukup &&
		body.Nilai != models.NilaiKPIBuruk {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Nilai harus BAIK, CUKUP, atau BURUK."})
	}

	var activity models.Activity
	if err := config.DB.First(&activity, "id = ?", id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Activity tidak ditemukan."})
	}

	if activity.Status != models.StatusDiterima {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Hanya activity yang sudah Selesai (DITERIMA) yang dapat diberikan nilai KPI."})
	}

	// Use TargetSelesai as the period reference
	minggu, bulan, tahun := getKPIPeriod(activity.TargetSelesai)

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Get current KPIPegawai record for this period
		var kpi models.KPIPegawai
		err := tx.
			Where("pegawai_id = ? AND bulan = ? AND tahun = ? AND minggu = ?",
				activity.PegawaiID, bulan, tahun, minggu).
			First(&kpi).Error

		if err != nil {
			if err == gorm.ErrRecordNotFound {
				// Create new record for this period if not exists
				kpi = models.KPIPegawai{
					ID:        uuid.NewString(),
					PegawaiID: activity.PegawaiID,
					Bulan:     bulan,
					Tahun:     tahun,
					Minggu:    minggu,
				}
				if err := tx.Create(&kpi).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}

		// 2. Adjust counters
		fieldMap := map[models.NilaiKPI]string{
			models.NilaiKPIBaik:  "baik",
			models.NilaiKPICukup: "cukup",
			models.NilaiKPIBuruk: "buruk",
		}

		// If there was an old rating, decrement its counter
		if activity.NilaiKPI != nil {
			oldField := fieldMap[*activity.NilaiKPI]
			if err := tx.Model(&kpi).Update(oldField, gorm.Expr(oldField+" - 1")).Error; err != nil {
				return err
			}
		}

		// Increment new rating counter
		newField := fieldMap[body.Nilai]
		if err := tx.Model(&kpi).Update(newField, gorm.Expr(newField+" + 1")).Error; err != nil {
			return err
		}

		// 3. Update Activity
		activity.NilaiKPI = &body.Nilai
		if err := tx.Save(&activity).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal update KPI: " + err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":   "KPI berhasil diperbarui.",
		"activityId": activity.ID,
		"nilaiKPI":   activity.NilaiKPI,
	})
}
