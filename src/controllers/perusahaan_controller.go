package controllers

import (
	"fmt"
	"net/http"
	"strconv"

	"mantra/src/config"
	"mantra/src/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// Helper: Pembangkit ID kustom berformat MXXXX (misal: M0013)
func generatePerusahaanID(tx *gorm.DB) (string, error) {
	var lastID string
	err := tx.Model(&models.Perusahaan{}).Unscoped().Select("id").Order("id DESC").Limit(1).Row().Scan(&lastID)
	if err != nil {
		return "M0001", nil
	}
	if len(lastID) > 1 && lastID[0] == 'M' {
		numPart := lastID[1:]
		num, err := strconv.Atoi(numPart)
		if err == nil {
			return fmt.Sprintf("M%04d", num+1), nil
		}
	}
	return "M0001", nil
}

// GET /perusahaan - List all companies
func GetPerusahaanList(c echo.Context) error {
	var companies []models.Perusahaan
	if err := config.DB.Find(&companies).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, companies)
}

// GET /perusahaan/:id - Get detail company
func GetPerusahaanDetail(c echo.Context) error {
	id := c.Param("id")
	var company models.Perusahaan
	if err := config.DB.First(&company, "id = ?", id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Perusahaan tidak ditemukan."})
	}
	return c.JSON(http.StatusOK, company)
}

// request payload
type CreatePerusahaanReq struct {
	Nama    string `json:"nama"`
	Alamat  string `json:"alamat"`
	Telepon string `json:"telepon"` // Maps to nomor_telepon in DB
}

// POST /perusahaan - Create new company
func CreatePerusahaan(c echo.Context) error {
	var req CreatePerusahaanReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}
	if req.Nama == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Nama perusahaan wajib diisi."})
	}

	var company models.Perusahaan

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		newID, err := generatePerusahaanID(tx)
		if err != nil {
			return err
		}

		addr := req.Alamat
		phone := req.Telepon

		company = models.Perusahaan{
			ID:           newID,
			Nama:         req.Nama,
			Alamat:       &addr,
			NomorTelepon: &phone,
		}

		return tx.Create(&company).Error
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menyimpan data perusahaan ke database."})
	}

	return c.JSON(http.StatusCreated, company)
}

// PUT /perusahaan/:id - Update existing company
func UpdatePerusahaan(c echo.Context) error {
	id := c.Param("id")
	var req CreatePerusahaanReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}
	if req.Nama == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Nama perusahaan wajib diisi."})
	}

	var company models.Perusahaan
	if err := config.DB.First(&company, "id = ?", id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Perusahaan tidak ditemukan."})
	}

	addr := req.Alamat
	phone := req.Telepon

	company.Nama = req.Nama
	company.Alamat = &addr
	company.NomorTelepon = &phone

	if err := config.DB.Save(&company).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal memperbarui data perusahaan di database."})
	}

	return c.JSON(http.StatusOK, company)
}

// DELETE /perusahaan/:id - Delete existing company
func DeletePerusahaan(c echo.Context) error {
	id := c.Param("id")
	var company models.Perusahaan
	if err := config.DB.First(&company, "id = ?", id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Perusahaan tidak ditemukan."})
	}

	if err := config.DB.Delete(&company).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menghapus data perusahaan di database."})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Perusahaan berhasil dihapus."})
}

