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
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func currentBulanTahun() (int, int) {
	now := time.Now()
	return int(now.Month()), now.Year()
}

func queryInt(c echo.Context, key string, fallback int) int {
	val := fallback
	fmt.Sscanf(c.QueryParam(key), "%d", &val)
	return val
}

// ─── KPIResponse — pakai gorm column tag supaya Scan bekerjaaaa ─────────────────

type KPIResponse struct {
	PegawaiID string `gorm:"column:pegawai_id" json:"pegawaiId"`
	Nama      string `gorm:"column:nama"       json:"nama"`
	Divisi    string `gorm:"column:divisi"     json:"divisi"`
	Baik      int    `gorm:"column:baik"       json:"baik"`
	Cukup     int    `gorm:"column:cukup"      json:"cukup"`
	Buruk     int    `gorm:"column:buruk"      json:"buruk"`
}

// ─── Helper: base query JOIN dengan quote — PostgreSQL case-sensitive ─────────

func kpiBaseQuery() *gorm.DB {
	return config.DB.Table(`"KPIPegawai"`).
		Select(`
            "KPIPegawai".pegawai_id,
            "Pegawai".nama,
            "Pegawai".divisi,
            "KPIPegawai".baik,
            "KPIPegawai".cukup,
            "KPIPegawai".buruk
        `).
		Joins(`JOIN "Pegawai" ON "Pegawai".id = "KPIPegawai".pegawai_id`)
}

func currentMinggu() int {
	day := time.Now().Day()
	minggu := (day-1)/7 + 1
	if minggu > 4 {
		minggu = 4
	}
	return minggu
}

// ─── 1. Add KPI ───────────────────────────────────────────────────────────────
// POST /kpi/:pegawaiId
// Body: { "nilai": "BAIK"|"CUKUP"|"BURUK", "bulan"?: int, "tahun"?: int }

func AddKPI(c echo.Context) error {
	pegawaiID := c.Param("pegawaiId")

	var body struct {
		Nilai  models.NilaiKPI `json:"nilai"`
		Bulan  *int            `json:"bulan"`
		Tahun  *int            `json:"tahun"`
		Minggu *int            `json:"minggu"` // ← NEW, opsional
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid payload."})
	}
	if body.Nilai != models.NilaiKPIBaik &&
		body.Nilai != models.NilaiKPICukup &&
		body.Nilai != models.NilaiKPIBuruk {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Nilai harus BAIK, CUKUP, atau BURUK."})
	}

	bulan, tahun := currentBulanTahun()
	minggu := currentMinggu() // ← default minggu sekarang

	if body.Bulan != nil {
		bulan = *body.Bulan
	}
	if body.Tahun != nil {
		tahun = *body.Tahun
	}
	if body.Minggu != nil {
		minggu = *body.Minggu
	}

	// Validasi minggu
	if minggu < 1 || minggu > 4 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Minggu harus antara 1–4."})
	}

	// Pastikan pegawai ada
	var pegawai models.Pegawai
	if err := config.DB.First(&pegawai, "id = ?", pegawaiID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Pegawai tidak ditemukan."})
	}

	// FindOrCreate — 1 row per pegawai per bulan/tahun/minggu
	var kpi models.KPIPegawai
	err := config.DB.
		Where("pegawai_id = ? AND bulan = ? AND tahun = ? AND minggu = ?",
			pegawaiID, bulan, tahun, minggu).
		First(&kpi).Error

	if err != nil {
		kpi = models.KPIPegawai{
			ID:        uuid.NewString(),
			PegawaiID: pegawaiID,
			Bulan:     bulan,
			Tahun:     tahun,
			Minggu:    minggu, // ← set minggu
		}
		if err := config.DB.Create(&kpi).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
	}

	// Atomic increment
	field := map[models.NilaiKPI]string{
		models.NilaiKPIBaik:  "baik",
		models.NilaiKPICukup: "cukup",
		models.NilaiKPIBuruk: "buruk",
	}[body.Nilai]

	config.DB.Model(&kpi).Update(field, gorm.Expr(field+" + 1"))

	// Return fresh
	var result KPIResponse
	kpiBaseQuery().
		Where(`"KPIPegawai".id = ?`, kpi.ID).
		Scan(&result)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"bulan":  bulan,
		"tahun":  tahun,
		"minggu": minggu,
		"data":   result,
	})
}

// ─── 2. Get All KPI — Bulan Spesifik ─────────────────────────────────────────
// GET /kpi?bulan=3&tahun=2026

func GetAllKPIBulan(c echo.Context) error {
	bulan, tahun := currentBulanTahun()
	bulan = queryInt(c, "bulan", bulan)
	tahun = queryInt(c, "tahun", tahun)

	var results []KPIResponse
	config.DB.Table(`"KPIPegawai"`).
		Select(`
            "KPIPegawai".pegawai_id,
            "Pegawai".nama,
            "Pegawai".divisi,
            SUM("KPIPegawai".baik)  AS baik,
            SUM("KPIPegawai".cukup) AS cukup,
            SUM("KPIPegawai".buruk) AS buruk
        `).
		Joins(`JOIN "Pegawai" ON "Pegawai".id = "KPIPegawai".pegawai_id`).
		Where(`"KPIPegawai".bulan = ? AND "KPIPegawai".tahun = ?`, bulan, tahun).
		Group(`"KPIPegawai".pegawai_id, "Pegawai".nama, "Pegawai".divisi`).
		Order("baik DESC").
		Scan(&results)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"bulan": bulan,
		"tahun": tahun,
		"data":  results,
	})
}

// ─── 3. Get All KPI — Yearly Total ───────────────────────────────────────────
// GET /kpi/yearly?tahun=2026&bulanAwal=1&bulanAkhir=12

func GetAllKPIYearly(c echo.Context) error {
	defaultBulan, defaultTahun := currentBulanTahun()
	tahunStr := c.QueryParam("tahun")

	var startBulan, startTahun, endBulan, endTahun int
	if tahunStr != "" {
		tahun := queryInt(c, "tahun", defaultTahun)
		bulanAwal := queryInt(c, "bulanAwal", 1)
		bulanAkhir := queryInt(c, "bulanAkhir", 12)
		startBulan = bulanAwal
		startTahun = tahun
		endBulan = bulanAkhir
		endTahun = tahun
	} else {
		startBulan = queryInt(c, "startBulan", defaultBulan)
		startTahun = queryInt(c, "startTahun", defaultTahun)
		endBulan = queryInt(c, "endBulan", defaultBulan)
		endTahun = queryInt(c, "endTahun", defaultTahun)
	}

	var results []KPIResponse
	config.DB.Table(`"KPIPegawai"`).
		Select(`
            "KPIPegawai".pegawai_id,
            "Pegawai".nama,
            "Pegawai".divisi,
            SUM("KPIPegawai".baik)  AS baik,
            SUM("KPIPegawai".cukup) AS cukup,
            SUM("KPIPegawai".buruk) AS buruk
        `).
		Joins(`JOIN "Pegawai" ON "Pegawai".id = "KPIPegawai".pegawai_id`).
		Where(`("KPIPegawai".tahun > ? OR ("KPIPegawai".tahun = ? AND "KPIPegawai".bulan >= ?)) AND ("KPIPegawai".tahun < ? OR ("KPIPegawai".tahun = ? AND "KPIPegawai".bulan <= ?))`,
			startTahun, startTahun, startBulan, endTahun, endTahun, endBulan).
		Group(`"KPIPegawai".pegawai_id, "Pegawai".nama, "Pegawai".divisi`).
		Order("baik DESC").
		Scan(&results)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"startBulan": startBulan,
		"startTahun": startTahun,
		"endBulan":   endBulan,
		"endTahun":   endTahun,
		"data":       results,
	})
}

// ─── 4. Get KPI — 1 Pegawai, Bulan Spesifik ──────────────────────────────────
// GET /kpi/:pegawaiId?bulan=3&tahun=2026

func GetKPIPegawaiBulan(c echo.Context) error {
	pegawaiID := c.Param("pegawaiId")
	bulan, tahun := currentBulanTahun()
	bulan = queryInt(c, "bulan", bulan)
	tahun = queryInt(c, "tahun", tahun)

	var result KPIResponse
	kpiBaseQuery().
		Where(`"KPIPegawai".pegawai_id = ? AND "KPIPegawai".bulan = ? AND "KPIPegawai".tahun = ?`,
			pegawaiID, bulan, tahun).
		Scan(&result)

	// Belum ada KPI bulan ini — return 0 semua
	if result.Nama == "" {
		var pegawai models.Pegawai
		config.DB.First(&pegawai, "id = ?", pegawaiID)
		result = KPIResponse{
			PegawaiID: pegawaiID,
			Nama:      pegawai.Nama,
			Divisi:    string(pegawai.Divisi),
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"bulan": bulan,
		"tahun": tahun,
		"data":  result,
	})
}

// ─── 5. Get KPI — 1 Pegawai, Yearly ─────────────────────────────────────────
// GET /kpi/:pegawaiId/yearly?tahun=2026&bulanAwal=1&bulanAkhir=12

func GetKPIPegawaiYearly(c echo.Context) error {
	pegawaiID := c.Param("pegawaiId")
	_, tahun := currentBulanTahun()
	tahun = queryInt(c, "tahun", tahun)
	bulanAwal := queryInt(c, "bulanAwal", 1)
	bulanAkhir := queryInt(c, "bulanAkhir", 12)

	var result KPIResponse
	config.DB.Table(`"KPIPegawai"`).
		Select(`
            "KPIPegawai".pegawai_id,
            "Pegawai".nama,
            "Pegawai".divisi,
            SUM("KPIPegawai".baik)  AS baik,
            SUM("KPIPegawai".cukup) AS cukup,
            SUM("KPIPegawai".buruk) AS buruk
        `).
		Joins(`JOIN "Pegawai" ON "Pegawai".id = "KPIPegawai".pegawai_id`).
		Where(`"KPIPegawai".pegawai_id = ? AND "KPIPegawai".tahun = ? AND "KPIPegawai".bulan BETWEEN ? AND ?`,
			pegawaiID, tahun, bulanAwal, bulanAkhir).
		Group(`"KPIPegawai".pegawai_id, "Pegawai".nama, "Pegawai".divisi`).
		Scan(&result)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"tahun":      tahun,
		"bulanAwal":  bulanAwal,
		"bulanAkhir": bulanAkhir,
		"data":       result,
	})
}

// ─── 6. Distribusi KPI Bulan ──────────────────────────────────────────────────
// GET /kpi/distribusi?bulan=3&tahun=2026

func GetDistribusiKPIBulan(c echo.Context) error {
	defaultBulan, defaultTahun := currentBulanTahun()
	startBulan := queryInt(c, "startBulan", 0)

	type Distribusi struct {
		Baik  int `gorm:"column:baik"  json:"baik"`
		Cukup int `gorm:"column:cukup" json:"cukup"`
		Buruk int `gorm:"column:buruk" json:"buruk"`
	}

	var result Distribusi
	query := config.DB.Table(`"KPIPegawai"`).
		Select(`SUM(baik) AS baik, SUM(cukup) AS cukup, SUM(buruk) AS buruk`)

	if startBulan > 0 {
		startTahun := queryInt(c, "startTahun", defaultTahun)
		endBulan := queryInt(c, "endBulan", defaultBulan)
		endTahun := queryInt(c, "endTahun", defaultTahun)

		query = query.Where(`("KPIPegawai".tahun > ? OR ("KPIPegawai".tahun = ? AND "KPIPegawai".bulan >= ?)) AND ("KPIPegawai".tahun < ? OR ("KPIPegawai".tahun = ? AND "KPIPegawai".bulan <= ?))`,
			startTahun, startTahun, startBulan, endTahun, endTahun, endBulan)
	} else {
		bulan := queryInt(c, "bulan", defaultBulan)
		tahun := queryInt(c, "tahun", defaultTahun)
		query = query.Where(`"KPIPegawai".bulan = ? AND "KPIPegawai".tahun = ?`, bulan, tahun)
	}

	query.Scan(&result)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":  result,
	})
}
