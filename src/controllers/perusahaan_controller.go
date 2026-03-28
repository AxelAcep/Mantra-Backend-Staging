package controllers

import (
	"net/http"

	"mantra/src/config"
	"mantra/src/models"

	"github.com/labstack/echo/v4"
)

func GetPerusahaanList(c echo.Context) error {
	var companies []models.Perusahaan
	if err := config.DB.Find(&companies).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, companies)
}
