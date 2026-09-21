package controllers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"mantra/src/config"
	"mantra/src/models"

	"github.com/labstack/echo/v4"
)

func generateBarangID(tx interface{ Exec(string, ...interface{}) error }) string {
	var lastID string
	config.DB.Raw(`SELECT "id" FROM "Barang" ORDER BY "id" DESC LIMIT 1`).Scan(&lastID)
	if lastID == "" {
		return "B0001"
	}
	if len(lastID) > 1 && lastID[0] == 'B' {
		num, err := strconv.Atoi(lastID[1:])
		if err == nil {
			return fmt.Sprintf("B%04d", num+1)
		}
	}
	return "B0001"
}

// GET /barang — paginated list with search & sort
func GetBarangList(c echo.Context) error {
	page := 1
	limit := 20
	if v, err := strconv.Atoi(c.QueryParam("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(c.QueryParam("limit")); err == nil && v > 0 {
		limit = v
	}
	search := strings.TrimSpace(c.QueryParam("search"))
	sortBy := c.QueryParam("sortBy")
	sortDir := c.QueryParam("sortDir")

	offset := (page - 1) * limit

	query := config.DB.Model(&models.Barang{})

	if search != "" {
		like := "%" + search + "%"
		query = query.Where(`"no_barang" ILIKE ? OR "deskripsi" ILIKE ? OR "satuan" ILIKE ?`, like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menghitung data."})
	}

	orderClause := `"no_barang" ASC`
	if sortBy != "" {
		allowedSorts := map[string]string{
			"noBarang":  `"no_barang"`,
			"deskripsi": `"deskripsi"`,
			"satuan":    `"satuan"`,
		}
		if col, ok := allowedSorts[sortBy]; ok {
			dir := "ASC"
			if strings.ToUpper(sortDir) == "DESC" {
				dir = "DESC"
			}
			orderClause = col + " " + dir
		}
	}

	var items []models.Barang
	if err := query.Order(orderClause).Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data barang."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": items,
		"meta": map[string]interface{}{
			"page":       page,
			"limit":      limit,
			"total":      total,
			"totalPages": int(math.Ceil(float64(total) / float64(limit))),
		},
	})
}

// POST /barang — create new item
func CreateBarang(c echo.Context) error {
	var req struct {
		NoBarang  string `json:"noBarang"`
		Deskripsi string `json:"deskripsi"`
		Satuan    string `json:"satuan"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}
	if strings.TrimSpace(req.NoBarang) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "No. Barang wajib diisi."})
	}
	if strings.TrimSpace(req.Deskripsi) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Deskripsi barang wajib diisi."})
	}
	if strings.TrimSpace(req.Satuan) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Satuan wajib diisi."})
	}

	// Check unique NoBarang
	var existing models.Barang
	if err := config.DB.Where(`"no_barang" = ?`, strings.TrimSpace(req.NoBarang)).First(&existing).Error; err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "No. Barang sudah digunakan."})
	}

	newID := generateBarangID(nil)
	barang := models.Barang{
		ID:        newID,
		NoBarang:  strings.TrimSpace(req.NoBarang),
		Deskripsi: strings.TrimSpace(req.Deskripsi),
		Satuan:    strings.TrimSpace(req.Satuan),
	}

	if err := config.DB.Create(&barang).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menyimpan data barang."})
	}

	return c.JSON(http.StatusCreated, barang)
}

// PUT /barang/:id — update item
func UpdateBarang(c echo.Context) error {
	id := c.Param("id")
	var req struct {
		NoBarang  string `json:"noBarang"`
		Deskripsi string `json:"deskripsi"`
		Satuan    string `json:"satuan"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}
	if strings.TrimSpace(req.NoBarang) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "No. Barang wajib diisi."})
	}
	if strings.TrimSpace(req.Deskripsi) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Deskripsi barang wajib diisi."})
	}
	if strings.TrimSpace(req.Satuan) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Satuan wajib diisi."})
	}

	var barang models.Barang
	if err := config.DB.First(&barang, `"id" = ?`, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Barang tidak ditemukan."})
	}

	// Check unique NoBarang (excluding current)
	var existing models.Barang
	if err := config.DB.Where(`"no_barang" = ? AND "id" != ?`, strings.TrimSpace(req.NoBarang), id).First(&existing).Error; err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "No. Barang sudah digunakan."})
	}

	barang.NoBarang = strings.TrimSpace(req.NoBarang)
	barang.Deskripsi = strings.TrimSpace(req.Deskripsi)
	barang.Satuan = strings.TrimSpace(req.Satuan)

	if err := config.DB.Save(&barang).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal memperbarui data barang."})
	}

	return c.JSON(http.StatusOK, barang)
}

// DELETE /barang/:id — delete item
func DeleteBarang(c echo.Context) error {
	id := c.Param("id")
	var barang models.Barang
	if err := config.DB.First(&barang, `"id" = ?`, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Barang tidak ditemukan."})
	}

	if err := config.DB.Delete(&barang).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menghapus data barang."})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Barang berhasil dihapus."})
}
