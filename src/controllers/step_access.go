package controllers

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"mantra/src/config"
	"mantra/src/models"
)

// Guard akses "siapa boleh LIHAT tahap apa" di alur Pengadaan Barang — dipakai
// bareng di semua GetDetailXxx per step. MO (MANAGER_OPERASIONAL), DIREKTUR,
// KOMISARIS, dan role MASTER selalu boleh akses semua tahap.
//
// "Admin Proyek" (step Follow Up/Implementasi/BAST/Garansi) BUKAN divisi tetap — itu
// pegawai spesifik yang di-assign per-tracking lewat AssignAdminProyek — jadi
// dicek terpisah (isAssignedAdminProyek), bukan lewat daftar divisi statis.

// getStepAccessClaims ekstrak pegawaiId/role/divisi dari JWT — dipakai khusus
// buat guard akses per-tahap (beda dari getXxxClaims tiap controller yang juga
// ngambil namaPegawai buat keperluan log).
func getStepAccessClaims(c echo.Context) (pegawaiID, roleStr, divisiStr string, ok bool) {
	claims, valid := c.Get("user").(jwt.MapClaims)
	if !valid {
		return "", "", "", false
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ = pegawaiMap["id"].(string)
	roleStr, _ = claims["role"].(string)
	divisiStr, _ = pegawaiMap["divisi"].(string)
	return pegawaiID, roleStr, divisiStr, true
}

var stepAllowedDivisi = map[models.StepPenawaran][]string{
	models.StepPermintaanMasuk:      {"SALES", "ADMIN_SEKERTARIS", "PRESALES"},
	models.StepPenyusunanBoQ:        {"SALES", "ADMIN_SEKERTARIS", "PRESALES"},
	models.StepReviewInternal:       {"ADMIN_SEKERTARIS", "ADMIN_SEKERTARIAT"},
	models.StepPersetujuanManajemen: {"ADMIN_SEKERTARIS", "ADMIN_SEKERTARIAT"},
	models.StepFollowUp:             {"SALES", "ADMIN_SEKERTARIAT", "FINANCE_ACCOUNTING"},
	models.StepImplementasi:         {"SALES", "PROCUREMENT_GA", "FINANCE_ACCOUNTING", "ADMIN_SEKERTARIAT"},
	models.StepBAST:                 {"SALES", "FINANCE_ACCOUNTING"},
	models.StepGaransi:              {"FINANCE_ACCOUNTING"},
	models.StepPembayaran:           {"FINANCE_ACCOUNTING"},
}

// canViewStep ngecek akses generik berbasis divisi/role doang (belum termasuk
// pengecualian "Admin Proyek" per-tracking — lihat canViewStepForTracking).
func canViewStep(step models.StepPenawaran, roleStr, divisiStr string) bool {
	if roleStr == "MASTER" {
		return true
	}
	switch divisiStr {
	case "MANAGER_OPERASIONAL", "DIREKTUR", "KOMISARIS":
		return true
	}
	for _, d := range stepAllowedDivisi[step] {
		if d == divisiStr {
			return true
		}
	}
	return false
}

// isAssignedAdminProyek ngecek apakah pegawai yang login ini emang Admin
// Proyek yang di-assign ke tracking tsb (lewat FollowUp.ActivityAdminProyek).
func isAssignedAdminProyek(pegawaiID, trackingID string) bool {
	if pegawaiID == "" {
		return false
	}
	var followUp models.FollowUp
	if err := config.DB.
		Preload("ActivityAdminProyek").
		Where("tracking_penawaran_id = ?", trackingID).
		First(&followUp).Error; err != nil {
		return false
	}
	return followUp.ActivityAdminProyek != nil && followUp.ActivityAdminProyek.PegawaiID == pegawaiID
}

// canViewStepForTracking = canViewStep + pengecualian Admin Proyek buat step
// Follow Up/Implementasi/BAST/Garansi (tahap-tahap yang aksesnya bisa
// tergantung assignment per-tracking, bukan cuma divisi).
func canViewStepForTracking(step models.StepPenawaran, roleStr, divisiStr, pegawaiID, trackingID string) bool {
	if canViewStep(step, roleStr, divisiStr) {
		return true
	}
	switch step {
	case models.StepFollowUp, models.StepImplementasi, models.StepBAST, models.StepGaransi:
		return isAssignedAdminProyek(pegawaiID, trackingID)
	}
	return false
}
