package controllers

import (
	"errors"
	"mantra/src/config"
	"mantra/src/models"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ==========================================
// HELPER: Paginate Activity (Master)
// ==========================================

// ==========================================
// CONTROLLERS — supervisi.go
// ==========================================

func getSupervisiDivisi(c echo.Context) (string, error) {
	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "Unauthorized.")
	}
	pegawaiClaims, ok := claims["pegawai"].(map[string]interface{})
	if !ok {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "Data pegawai tidak ditemukan.")
	}
	divisi, ok := pegawaiClaims["divisi"].(string)
	if !ok || divisi == "" {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "Divisi tidak ditemukan.")
	}
	return divisi, nil
}

func baseSupervisiActivityQuery(divisi string) *gorm.DB {
	return config.DB.Model(&models.Activity{}).
		Preload("Pegawai").
		Preload("Reschedule").
		Preload("Dokumen").
		Where(`"Pegawai"."divisi" = ?`, divisi)
}

// ── 1. Get All Activity (non-riwayat) filtered by divisi ──────────────────────

func SupervisiGetActivityAktif(c echo.Context) error {
	divisi, err := getSupervisiDivisi(c)
	if err != nil {
		return err
	}

	search := c.QueryParam("search")
	sortBy := c.QueryParam("sortBy")
	sortDir := c.QueryParam("sortDir")
	filterKategori := c.QueryParam("kategori")
	filterStatus := c.QueryParam("status")
	isSupervised := c.QueryParam("isSupervised")

	query := baseSupervisiActivityQuery(divisi).
	    Joins(`JOIN "Pegawai" ON "Pegawai"."id" = "Activity"."pegawai_id"`).
		Where(`"Activity"."status" NOT IN ?`, []string{
			string(models.StatusDiterima),
			string(models.StatusDibatalkan),
		})

	if search != "" {
		like := "%" + search + "%"
		query = query.Where(
			`"Pegawai"."nama" ILIKE ? OR "Activity"."judul" ILIKE ? OR "Activity"."perusahaan" ILIKE ? OR "Activity"."terkait_po" ILIKE ?`,
			like, like, like, like,
		)
	}

	if filterKategori != "" {
		query = query.Where(`"Activity"."kategori" = ?`, filterKategori)
	}
	if filterStatus != "" {
		if filterStatus == "OVERDUE" {
			query = query.Where(`"Activity"."status" = ? AND "Activity"."target_selesai" < ?`, models.StatusOnProgress, time.Now())
		} else if filterStatus == string(models.StatusOnProgress) {
			query = query.Where(`"Activity"."status" = ? AND "Activity"."target_selesai" >= ?`, models.StatusOnProgress, time.Now())
		} else {
			query = query.Where(`"Activity"."status" = ?`, filterStatus)
		}
	}

	if isSupervised != "" {
		query = query.Where(`"Activity"."is_supervised" = ?`, isSupervised == "true")
	}

	orderClause := `"Activity"."created_at" DESC`
	validSortDir := "DESC"
	if strings.ToUpper(sortDir) == "ASC" {
		validSortDir = "ASC"
	}
	switch strings.ToLower(sortBy) {
	case "karyawan":
		orderClause = `"Pegawai"."nama" ` + validSortDir
	case "kategori":
		orderClause = `"Activity"."kategori" ` + validSortDir
	case "status":
		orderClause = `"Activity"."status" ` + validSortDir
	case "targetselesai":
		orderClause = `"Activity"."target_selesai" ` + validSortDir
	}

	query = query.Order(orderClause)
	return paginateMasterActivity(c, query, 10)
}

// ── 2. Get All Riwayat filtered by divisi ────────────────────────────────────

func SupervisiGetActivityRiwayat(c echo.Context) error {
	divisi, err := getSupervisiDivisi(c)
	if err != nil {
		return err
	}

	search := c.QueryParam("search")
	sortBy := c.QueryParam("sortBy")
	sortDir := c.QueryParam("sortDir")
	filterKategori := c.QueryParam("kategori")
	filterStatus := c.QueryParam("status")
	isSupervised := c.QueryParam("isSupervised")

	query := baseSupervisiActivityQuery(divisi).
		Joins(`JOIN "Pegawai" ON "Pegawai"."id" = "Activity"."pegawai_id"`).
		Where(`"Activity"."status" IN ?`, []string{
			string(models.StatusDiterima),
			string(models.StatusDibatalkan),
		})

	if search != "" {
		like := "%" + search + "%"
		query = query.Where(
			`"Pegawai"."nama" ILIKE ? OR "Activity"."judul" ILIKE ? OR "Activity"."perusahaan" ILIKE ? OR "Activity"."terkait_po" ILIKE ?`,
			like, like, like, like,
		)
	}

	if filterKategori != "" {
		query = query.Where(`"Activity"."kategori" = ?`, filterKategori)
	}
	if filterStatus != "" {
		query = query.Where(`"Activity"."status" = ?`, filterStatus)
	}

	if isSupervised != "" {
		query = query.Where(`"Activity"."is_supervised" = ?`, isSupervised == "true")
	}

	orderClause := `"Activity"."created_at" DESC`
	validSortDir := "DESC"
	if strings.ToUpper(sortDir) == "ASC" {
		validSortDir = "ASC"
	}
	switch strings.ToLower(sortBy) {
	case "karyawan":
		orderClause = `"Pegawai"."nama" ` + validSortDir
	case "kategori":
		orderClause = `"Activity"."kategori" ` + validSortDir
	case "status":
		orderClause = `"Activity"."status" ` + validSortDir
	case "targetselesai":
		orderClause = `"Activity"."target_selesai" ` + validSortDir
	}

	query = query.Order(orderClause)
	return paginateMasterActivity(c, query, 10)
}

// ── 3. Set IsSupervised = true ────────────────────────────────────────────────

func SupervisiMarkAsSupervised(c echo.Context) error {
	divisi, err := getSupervisiDivisi(c)
	if err != nil {
		return err
	}

	activityID := c.Param("id")
	if activityID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ID aktivitas tidak ditemukan."})
	}

	// Pastikan activity milik divisi yang sama
	var activity models.Activity
	err = config.DB.
		Joins(`JOIN "Pegawai" ON "Pegawai"."id" = "Activity"."pegawai_id"`).
		Where(`"Activity"."id" = ? AND "Pegawai"."divisi" = ?`, activityID, divisi).
		First(&activity).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Aktivitas tidak ditemukan atau bukan divisi Anda."})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	if activity.IsSupervised {
		return c.JSON(http.StatusOK, map[string]string{"message": "Aktivitas sudah berstatus supervised."})
	}

	if err := config.DB.Model(&activity).Update("is_supervised", true).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":  "Aktivitas berhasil ditandai sebagai supervised.",
		"activity": activity,
	})
}

type DashboardStatsResponse struct {
	TotalAktivitasBulanIni int `json:"totalAktivitasBulanIni"`
	JumlahOverdue          int `json:"jumlahOverdue"`
	DeadlineHariIni        int `json:"deadlineHariIni"`
}

func GetSupervisiDashboardStats(c echo.Context) error {
	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	pegawaiMap, ok := claims["pegawai"].(map[string]interface{})
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Data pegawai tidak ditemukan."})
	}

	divisi, ok := pegawaiMap["divisi"].(string)
	if !ok || divisi == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Divisi tidak ditemukan."})
	}

	now := time.Now()

	base := config.DB.Model(&models.Activity{}).
		Joins(`JOIN "Pegawai" ON "Pegawai"."id" = "Activity"."pegawai_id"`).
		Where(`"Pegawai"."divisi" = ?`, divisi).
		Where(`"Pegawai"."deleted_at" IS NULL`)

	// 1. Total aktivitas bulan ini
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	var totalBulanIni int64
	if err := base.Session(&gorm.Session{}).
		Where(`"Activity"."created_at" >= ? AND "Activity"."created_at" < ?`, startOfMonth, endOfMonth).
		Count(&totalBulanIni).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil total aktivitas."})
	}

	// 2. Jumlah overdue
	var jumlahOverdue int64
	if err := base.Session(&gorm.Session{}).
		Where(`"Activity"."target_selesai" < ?`, now).
		Where(`"Activity"."status" IN ?`, []string{
			string(models.StatusOnProgress),
			string(models.StatusPending),
			string(models.StatusPendingPegawai),
		}).
		Count(&jumlahOverdue).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil jumlah overdue."})
	}

	// 3. Aktivitas deadline hari ini
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var deadlineHariIni int64
	if err := base.Session(&gorm.Session{}).
		Where(`"Activity"."target_selesai" >= ? AND "Activity"."target_selesai" < ?`, startOfDay, endOfDay).
		Where(`"Activity"."status" IN ?`, []string{
			string(models.StatusOnProgress),
			string(models.StatusPending),
			string(models.StatusPendingPegawai),
		}).
		Count(&deadlineHariIni).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil deadline hari ini."})
	}

	return c.JSON(http.StatusOK, DashboardStatsResponse{
		TotalAktivitasBulanIni: int(totalBulanIni),
		JumlahOverdue:          int(jumlahOverdue),
		DeadlineHariIni:        int(deadlineHariIni),
	})
}
