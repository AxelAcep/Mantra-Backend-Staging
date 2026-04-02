package controllers

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mantra/src/config"
	"mantra/src/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ==========================================
// HELPER: Paginate Activity (Master)
// ==========================================

type ActivityWithOverdue struct {
	models.Activity
	IsOverdue bool `json:"isOverdue"`
}

func paginateMasterActivity(c echo.Context, query *gorm.DB, pageSize int) error {
	pageStr := c.QueryParam("page")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var activities []models.Activity
	result := query.
		Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&activities)
	if result.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": result.Error.Error()})
	}

	now := time.Now()
	enriched := make([]ActivityWithOverdue, len(activities))
	for i, a := range activities {
		isOverdue := (a.Status == models.StatusOnProgress || a.Status == models.StatusPendingPegawai) && now.After(a.TargetSelesai)
		if isOverdue {
			a.Status = "OVERDUE"
		}
		enriched[i] = ActivityWithOverdue{
			Activity:  a,
			IsOverdue: isOverdue,
		}
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":       enriched,
		"page":       page,
		"pageSize":   pageSize,
		"total":      total,
		"totalPages": totalPages,
	})
}

// ==========================================
// HELPER: Paginate Reschedule (Master)
// Karena ActivityReschedule tidak punya field Activity
// (hanya ActivityID string), kita manual enrich setelah query
// ==========================================

func paginateMasterReschedule(c echo.Context, query *gorm.DB, pageSize int) error {
	pageStr := c.QueryParam("page")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var reschedules []models.ActivityReschedule
	result := query.
		Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&reschedules)

	if result.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": result.Error.Error()})
	}

	// Kumpulkan semua ActivityID, lalu fetch Activity + Pegawai-nya sekaligus
	activityIDs := make([]string, 0, len(reschedules))
	for _, r := range reschedules {
		activityIDs = append(activityIDs, r.ActivityID)
	}

	activityMap := make(map[string]*models.Activity)
	if len(activityIDs) > 0 {
		var activities []models.Activity
		config.DB.Preload("Pegawai").Where("id IN ?", activityIDs).Find(&activities)
		for i := range activities {
			activityMap[activities[i].ID] = &activities[i]
		}
	}

	// Struct inline untuk response yang sudah di-enrich
	type RescheduleWithActivity struct {
		models.ActivityReschedule
		Activity *models.Activity `json:"activity,omitempty"`
	}

	enriched := make([]RescheduleWithActivity, 0, len(reschedules))
	for _, r := range reschedules {
		enriched = append(enriched, RescheduleWithActivity{
			ActivityReschedule: r,
			Activity:           activityMap[r.ActivityID],
		})
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":       enriched,
		"page":       page,
		"pageSize":   pageSize,
		"total":      total,
		"totalPages": totalPages,
	})
}

// ==========================================
// HELPER: Base query Activity untuk Master
// - Tidak ada filter pegawaiID (Master lihat SEMUA)
// - TIDAK pakai Where("deleted_at IS NULL") karena model
//   menggunakan *time.Time custom, bukan gorm.Model,
//   sehingga kolom deleted_at tidak di-handle GORM otomatis
// ==========================================

func baseMasterActivityQuery() *gorm.DB {
	return config.DB.Model(&models.Activity{}).
		Preload("Pegawai").
		Preload("Kolaborator").
		Preload("Kolaborator.Pegawai").
		Preload("Dokumen").
		Preload("Reschedule")
}

// ==========================================
// 1. Semua pengajuan reschedule PENDING
//    GET /master/reschedule?page=1
//    Pagination: 5
// ==========================================

func MasterGetReschedulePending(c echo.Context) error {
	search := c.QueryParam("search")

	query := config.DB.Model(&models.ActivityReschedule{}).
		Joins(`JOIN "Activity" ON "Activity"."id" = "ActivityReschedule"."activity_id"`).
		Joins(`JOIN "Pegawai" ON "Pegawai"."id" = "Activity"."pegawai_id"`).
		Where(`"ActivityReschedule"."status" = ?`, models.StatusReschedulePending)

	if search != "" {
		like := "%" + search + "%"
		query = query.Where(
			`"Pegawai"."nama" ILIKE ? OR "Activity"."judul" ILIKE ? OR "Activity"."perusahaan" ILIKE ?`,
			like, like, like,
		)
	}

	return paginateMasterReschedule(c, query, 10)
}

// ==========================================
// 2. Semua aktivitas selesai (dimodif jadi filter PENDING)
//    GET /master/activity/selesai?page=1
//    Pagination: 5
// ==========================================

func MasterGetActivitySelesai(c echo.Context) error {
	search := c.QueryParam("search")
	sortBy := c.QueryParam("sortBy")
	sortDir := c.QueryParam("sortDir")

	query := baseMasterActivityQuery().
		Joins(`JOIN "Pegawai" ON "Pegawai"."id" = "Activity"."pegawai_id"`).
		Where(`"Activity"."status" IN ?`, []string{
			string(models.StatusKonfirmasiSelesai),
		})

	if search != "" {
		like := "%" + search + "%"
		query = query.Where(
			`"Pegawai"."nama" ILIKE ? OR "Activity"."judul" ILIKE ? OR "Activity"."perusahaan" ILIKE ? OR "Activity"."kategori"::text ILIKE ?`,
			like, like, like, like,
		)
	}

	orderClause := `"Activity"."created_at" DESC`
	validSortDir := "DESC"
	if strings.ToUpper(sortDir) == "ASC" {
		validSortDir = "ASC"
	}
	switch strings.ToLower(sortBy) {
	case "karyawan":
		orderClause = `"Pegawai"."nama" ` + validSortDir
	case "judul":
		orderClause = `"Activity"."judul" ` + validSortDir
	case "perusahaan":
		orderClause = `"Activity"."perusahaan" ` + validSortDir
	case "kategori":
		orderClause = `"Activity"."kategori" ` + validSortDir
	}

	query = query.Order(orderClause)

	return paginateMasterActivity(c, query, 10)
}

// ==========================================
// 3. Semua aktivitas SELAIN selesai
//    Bukan DITERIMA dan bukan DIBATALKAN
//    GET /master/activity/aktif?page=1
//    Pagination: 10
// ==========================================

func MasterGetActivityAktif(c echo.Context) error {
	search := c.QueryParam("search")
	sortBy := c.QueryParam("sortBy")   // "karyawan" | "kategori" | "status"
	sortDir := c.QueryParam("sortDir") // "asc" | "desc"

	// Filter dropdown
	filterKaryawan := c.QueryParam("karyawan") // pegawai_id
	filterKategori := c.QueryParam("kategori") // e.g. "QUOTATION"
	filterStatus := c.QueryParam("status")     // e.g. "ON_PROGRESS"

	query := baseMasterActivityQuery().
		Joins(`JOIN "Pegawai" ON "Pegawai"."id" = "Activity"."pegawai_id"`).
		Where(`"Activity"."status" NOT IN ?`, []string{
			string(models.StatusDiterima),
			string(models.StatusDibatalkan),
		})

	// Search: nama, judul, perusahaan, no referensi (terkait_po)
	if search != "" {
		like := "%" + search + "%"
		query = query.Where(
			`"Pegawai"."nama" ILIKE ? OR "Activity"."judul" ILIKE ? OR "Activity"."perusahaan" ILIKE ? OR "Activity"."terkait_po" ILIKE ?`,
			like, like, like, like,
		)
	}

	// Filter
	if filterKaryawan != "" {
		query = query.Where(`"Activity"."pegawai_id" = ?`, filterKaryawan)
	}
	if filterKategori != "" {
		query = query.Where(`"Activity"."kategori" = ?`, filterKategori)
	}
	if filterStatus != "" {
		query = query.Where(`"Activity"."status" = ?`, filterStatus)
	}

	// Sort
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
	}

	query = query.Order(orderClause)

	return paginateMasterActivity(c, query, 10)
}

// ==========================================
// 4. Semua aktivitas tanpa filter status
//    GET /master/activity?page=1
//    Pagination: 10
// ==========================================

func MasterGetActivitySemua(c echo.Context) error {
	query := baseMasterActivityQuery().
		Where("status = ? OR status = ?", "DITERIMA", "DIBATALKAN")
	return paginateMasterActivity(c, query, 10)
}

// GET /master/stats
func MasterGetStats(c echo.Context) error {
	var totalAktivitas, perluKonfirmasi, pengajuanReschedule, overdue int64

	config.DB.Model(&models.Activity{}).
		Count(&totalAktivitas)

	config.DB.Model(&models.Activity{}).
		Where("status = ?", models.StatusKonfirmasiSelesai).
		Count(&perluKonfirmasi)

	config.DB.Model(&models.ActivityReschedule{}).
		Where("status = ?", models.StatusReschedulePending).
		Count(&pengajuanReschedule)

	config.DB.Model(&models.Activity{}).
		Where("status IN ? AND target_selesai < ?", []string{string(models.StatusOnProgress), string(models.StatusPendingPegawai)}, time.Now()).
		Count(&overdue)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"totalAktivitas":         totalAktivitas,
		"perluKonfirmasiSelesai": perluKonfirmasi,
		"pengajuanReschedule":    pengajuanReschedule,
		"overdue":                overdue,
	})
}

// GET /master/activity/:id
func MasterGetActivityDetail(c echo.Context) error {
	id := c.Param("id")

	var activity models.Activity
	result := config.DB.
		Preload("Pegawai").
		Preload("Kolaborator").
		Preload("Kolaborator.Pegawai").
		Preload("Dokumen").
		Preload("Dokumen.Pegawai").
		Preload("Reschedule").
		Preload("Chat").
		Preload("Chat.Pegawai").
		Preload("Parent").
		Preload("Parent.Pegawai").
		First(&activity, "id = ?", id)

	if result.Error != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Activity tidak ditemukan."})
	}

	isOverdue := (activity.Status == models.StatusOnProgress || activity.Status == models.StatusPendingPegawai) &&
		activity.TargetSelesai.Before(time.Now())

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":      activity,
		"isOverdue": isOverdue,
	})
}

// GET /master/karyawan?page=1&limit=10&search=&mode=tahun&bulan=3&tahun=2026
func MasterGetKaryawan(c echo.Context) error {
	page := queryInt(c, "page", 1)
	limit := queryInt(c, "limit", 10)
	search := c.QueryParam("search")
	mode := c.QueryParam("mode") // "bulan" | "tahun"

	bulan, tahun := currentBulanTahun()
	bulan = queryInt(c, "bulan", bulan)
	tahun = queryInt(c, "tahun", tahun)

	if mode == "" {
		mode = "tahun"
	}

	offset := (page - 1) * limit

	// ── 1. Count + paginate Pegawai ───────────────────────────────────────────
	baseQuery := config.DB.Model(&models.Pegawai{}).Where("deleted_at IS NULL")
	if search != "" {
		baseQuery = baseQuery.Where("nama ILIKE ?", "%"+search+"%")
	}

	var total int64
	baseQuery.Count(&total)

	var pegawaiList []models.Pegawai
	baseQuery.Offset(offset).Limit(limit).Order("nama ASC").Find(&pegawaiList)

	if len(pegawaiList) == 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"data": []interface{}{}, "total": total,
			"page": page, "totalPages": 0,
		})
	}

	// ── 2. Extract IDs ────────────────────────────────────────────────────────
	ids := make([]string, len(pegawaiList))
	for i, p := range pegawaiList {
		ids[i] = p.ID
	}

	// ── 3. Batch KPI query ────────────────────────────────────────────────────
	type KPIRow struct {
		PegawaiID string
		Baik      int
		Cukup     int
		Buruk     int
	}

	var kpiRows []KPIRow
	kpiQuery := config.DB.Table(`"KPIPegawai"`).
		Select(`pegawai_id,
            COALESCE(SUM(baik),  0) AS baik,
            COALESCE(SUM(cukup), 0) AS cukup,
            COALESCE(SUM(buruk), 0) AS buruk`).
		Where("pegawai_id IN ? AND tahun = ?", ids, tahun).
		Group("pegawai_id")

	if mode == "bulan" {
		kpiQuery = kpiQuery.Where("bulan = ?", bulan)
	}
	kpiQuery.Scan(&kpiRows)

	kpiMap := map[string]KPIRow{}
	for _, k := range kpiRows {
		kpiMap[k.PegawaiID] = k
	}

	// ── 4. Batch aktivitas berjalan ───────────────────────────────────────────
	type ActivityCount struct {
		PegawaiID string
		Count     int64
	}

	var berjalanCounts []ActivityCount
	config.DB.Model(&models.Activity{}).
		Select("pegawai_id, COUNT(*) AS count").
		Where("pegawai_id IN ? AND status = ?", ids, models.StatusOnProgress).
		Group("pegawai_id").
		Scan(&berjalanCounts)

	berjalanMap := map[string]int64{}
	for _, b := range berjalanCounts {
		berjalanMap[b.PegawaiID] = b.Count
	}

	// ── 5. Batch total aktivitas ──────────────────────────────────────────────
	var totalCounts []ActivityCount
	config.DB.Model(&models.Activity{}).
		Select("pegawai_id, COUNT(*) AS count").
		Where("pegawai_id IN ?", ids).
		Group("pegawai_id").
		Scan(&totalCounts)

	totalMap := map[string]int64{}
	for _, t := range totalCounts {
		totalMap[t.PegawaiID] = t.Count
	}

	// ── 6. Build result ───────────────────────────────────────────────────────
	type KaryawanRow struct {
		ID                string `json:"id"`
		Nama              string `json:"nama"`
		Divisi            string `json:"divisi"`
		Baik              int    `json:"baik"`
		Cukup             int    `json:"cukup"`
		Buruk             int    `json:"buruk"`
		AktivitasBerjalan int64  `json:"aktivitasBerjalan"`
		TotalAktivitas    int64  `json:"totalAktivitas"`
	}

	results := make([]KaryawanRow, len(pegawaiList))
	for i, p := range pegawaiList {
		kpi := kpiMap[p.ID]
		results[i] = KaryawanRow{
			ID:                p.ID,
			Nama:              p.Nama,
			Divisi:            string(p.Divisi),
			Baik:              kpi.Baik,
			Cukup:             kpi.Cukup,
			Buruk:             kpi.Buruk,
			AktivitasBerjalan: berjalanMap[p.ID],
			TotalAktivitas:    totalMap[p.ID],
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":       results,
		"total":      total,
		"page":       page,
		"totalPages": totalPages,
	})
}

// GET /master/kpi/:pegawaiId?mode=tahun&bulan=3&tahun=2026
func MasterGetDetailKPI(c echo.Context) error {
	pegawaiID := c.Param("pegawaiId")
	mode := c.QueryParam("mode") // "bulan" | "tahun"
	bulan, tahun := currentBulanTahun()
	bulan = queryInt(c, "bulan", bulan)
	tahun = queryInt(c, "tahun", tahun)

	if mode == "" {
		mode = "tahun"
	}

	// ── 1. Fetch Pegawai ──────────────────────────────────────────────────────
	var pegawai models.Pegawai
	if err := config.DB.First(&pegawai, "id = ?", pegawaiID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Pegawai tidak ditemukan."})
	}

	// ── 2. KPI Summary ────────────────────────────────────────────────────────
	type KPISummary struct {
		Baik  int `json:"baik"`
		Cukup int `json:"cukup"`
		Buruk int `json:"buruk"`
	}

	var summary KPISummary
	kpiQuery := config.DB.Table(`"KPIPegawai"`).
		Select(`COALESCE(SUM(baik),  0) AS baik,
                COALESCE(SUM(cukup), 0) AS cukup,
                COALESCE(SUM(buruk), 0) AS buruk`).
		Where("pegawai_id = ? AND tahun = ?", pegawaiID, tahun)

	if mode == "bulan" {
		kpiQuery = kpiQuery.Where("bulan = ?", bulan)
	}
	kpiQuery.Scan(&summary)

	// ── 3. KPI Weekly Breakdown ──────────────────────────────────────────────
	type KPIWeekly struct {
		Minggu int `json:"minggu"`
		Baik   int `json:"baik"`
		Cukup  int `json:"cukup"`
		Buruk  int `json:"buruk"`
	}

	var weekly []KPIWeekly
	config.DB.Table(`"KPIPegawai"`).
		Select(`minggu,
                COALESCE(SUM(baik),  0) AS baik,
                COALESCE(SUM(cukup), 0) AS cukup,
                COALESCE(SUM(buruk), 0) AS buruk`).
		Where("pegawai_id = ? AND tahun = ? AND bulan = ?", pegawaiID, tahun, bulan).
		Group("minggu").
		Order("minggu ASC").
		Scan(&weekly)

	// ── 4. Return Response ───────────────────────────────────────────────────
	return c.JSON(http.StatusOK, map[string]interface{}{
		"pegawai": map[string]interface{}{
			"id":     pegawai.ID,
			"nama":   pegawai.Nama,
			"divisi": pegawai.Divisi,
		},
		"summary": summary,
		"weekly":  weekly,
	})
}

func MasterGetActivityRiwayat(c echo.Context) error {
	search := c.QueryParam("search")
	sortBy := c.QueryParam("sortBy")
	sortDir := c.QueryParam("sortDir")
	filterKaryawan := c.QueryParam("karyawan")
	filterKategori := c.QueryParam("kategori")
	filterStatus := c.QueryParam("status")

	query := baseMasterActivityQuery().
		Joins(`JOIN "Pegawai" ON "Pegawai"."id" = "Activity"."pegawai_id"`).
		Where(`"Activity"."status" IN ?`, []string{ // ← IN, bukan NOT IN
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

	if filterKaryawan != "" {
		query = query.Where(`"Activity"."pegawai_id" = ?`, filterKaryawan)
	}
	if filterKategori != "" {
		query = query.Where(`"Activity"."kategori" = ?`, filterKategori)
	}
	if filterStatus != "" {
		query = query.Where(`"Activity"."status" = ?`, filterStatus)
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
	}

	query = query.Order(orderClause)

	return paginateMasterActivity(c, query, 10)
}

func GetKPIOverview(c echo.Context) error {
	pegawaiID := c.Param("pegawaiId")
	page := queryInt(c, "page", 1)
	limit := queryInt(c, "limit", 10)

	// Default bulan dan tahun
	now := time.Now()
	bulan := queryInt(c, "bulan", int(now.Month()))
	tahun := queryInt(c, "tahun", now.Year())

	offset := (page - 1) * limit

	// ── 1. Fetch Pegawai ──────────────────────────────────────────────────────
	var pegawai struct {
		ID     string
		Nama   string
		Divisi string
	}
	if err := config.DB.Table("Pegawai").Select("id, nama, divisi").Where("id = ?", pegawaiID).First(&pegawai).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Pegawai tidak ditemukan."})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data pegawai."})
	}

	// ── 2. Distribusi KPI ────────────────────────────────────────────────────
	var kpiSummary struct {
		Baik  int `json:"baik"`
		Cukup int `json:"cukup"`
		Buruk int `json:"buruk"`
	}
	config.DB.Table("KPIPegawai").
		Select("COALESCE(SUM(baik), 0) AS baik, COALESCE(SUM(cukup), 0) AS cukup, COALESCE(SUM(buruk), 0) AS buruk").
		Where("pegawai_id = ? AND tahun = ?", pegawaiID, tahun).
		Where("bulan = ?", bulan).
		Scan(&kpiSummary)

	// ── 3. Tren Kualitas Mingguan ────────────────────────────────────────────
	var weeklyTrends []struct {
		Minggu int `json:"minggu"`
		Baik   int `json:"baik"`
		Cukup  int `json:"cukup"`
		Buruk  int `json:"buruk"`
	}
	config.DB.Table("KPIPegawai").
		Select("minggu, COALESCE(SUM(baik), 0) AS baik, COALESCE(SUM(cukup), 0) AS cukup, COALESCE(SUM(buruk), 0) AS buruk").
		Where("pegawai_id = ? AND tahun = ? AND bulan = ?", pegawaiID, tahun, bulan).
		Group("minggu").
		Order("minggu ASC").
		Scan(&weeklyTrends)

	// ── 4. Riwayat Aktivitas ────────────────────────────────────────────────
	var total int64
	config.DB.Model(&models.Activity{}).
		Where("pegawai_id = ?", pegawaiID).
		Count(&total)

	var activities []models.Activity
	config.DB.Preload("Pegawai").
		Where("pegawai_id = ?", pegawaiID).
		Order("updated_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&activities)

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	// ── 5. Return Response ──────────────────────────────────────────────────
	return c.JSON(http.StatusOK, map[string]interface{}{
		"pegawai": map[string]interface{}{
			"id":     pegawai.ID,
			"nama":   pegawai.Nama,
			"divisi": pegawai.Divisi,
		},
		"kpiSummary":   kpiSummary,
		"weeklyTrends": weeklyTrends,
		"activities": map[string]interface{}{
			"data":       activities,
			"total":      total,
			"page":       page,
			"totalPages": totalPages,
		},
	})
}
