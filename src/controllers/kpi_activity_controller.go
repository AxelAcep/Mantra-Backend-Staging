package controllers

import (
	"fmt"
	"net/http"
	"time"

	"mantra/src/config"
	"mantra/src/models"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		// Atomic Upsert counter logic
		minggu, bulan, tahun := getKPIPeriod(activity.TargetSelesai)

		fieldMap := map[models.NilaiKPI]string{
			models.NilaiKPIBaik:  "baik",
			models.NilaiKPICukup: "cukup",
			models.NilaiKPIBuruk: "buruk",
		}

		kpi := models.KPIPegawai{
			ID:        uuid.NewString(),
			PegawaiID: activity.PegawaiID,
			Bulan:     bulan,
			Tahun:     tahun,
			Minggu:    minggu,
		}

		// Adjust old rating if exists
		if activity.NilaiKPI != nil {
			oldField := fieldMap[*activity.NilaiKPI]
			if err := tx.Model(&models.KPIPegawai{}).
				Where("pegawai_id = ? AND bulan = ? AND tahun = ? AND minggu = ?",
					activity.PegawaiID, bulan, tahun, minggu).
				Update(oldField, gorm.Expr(fmt.Sprintf("\"KPIPegawai\".\"%s\" - 1", oldField))).Error; err != nil {
				return err
			}
		}

		// Atomic Create or Update counter
		newField := fieldMap[body.Nilai]
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "pegawai_id"}, {Name: "bulan"}, {Name: "tahun"}, {Name: "minggu"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				newField:     gorm.Expr(fmt.Sprintf("\"KPIPegawai\".\"%s\" + 1", newField)),
				"updated_at": time.Now(),
			}),
		}).Create(&kpi).Error

		if err != nil {
			return err
		}

		// Update Activity
		activity.NilaiKPI = &body.Nilai
		return tx.Save(&activity).Error
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal update KPI: " + err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":    "KPI berhasil diperbarui.",
		"activityId": activity.ID,
		"nilaiKPI":   activity.NilaiKPI,
	})
}
