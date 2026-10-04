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
	models.StepPenyusunanBoQ:        {"SALES", "ADMIN_SEKERTARIS", "ADMIN_SEKERTARIAT", "PRESALES"},
	models.StepReviewInternal:       {"ADMIN_SEKERTARIS", "ADMIN_SEKERTARIAT"},
	models.StepPersetujuanManajemen: {"ADMIN_SEKERTARIS", "ADMIN_SEKERTARIAT"},
	models.StepFollowUp:             {"SALES", "ADMIN_SEKERTARIS", "ADMIN_SEKERTARIAT", "FINANCE_ACCOUNTING"},
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
	// Supervisi Sales -- approver gate di Review Internal (lihat
	// TryFinalizeReviewInternal/UpdateStatusReviewInternal), boleh liat SEMUA
	// tracking di step ini, gak cuma yang sales-nya terkait dia (beda sama
	// isRelatedSales di bawah, yang di-scope per-tracking).
	if step == models.StepReviewInternal && roleStr == "SUPERVISI" && divisiStr == "SALES" {
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
// Proyek yang di-assign ke tracking tsb -- pakai FollowUp.AdminProyekID
// (di-set langsung begitu AssignAdminProyek pertama kali milih orangnya,
// Stage 3->4), BUKAN FollowUp.ActivityAdminProyekID (itu baru ke-isi belakangan
// di Stage 6 pas daily Upload Dokumen PO dibuat -- kalau dipakai buat guard
// akses, Admin Proyek yang bersangkutan gak bakal punya akses sama sekali
// selama Stage 3-5, padahal itu justru bagian paling aktif dia ngerjain
// pengecekan dokumen PO).
func isAssignedAdminProyek(pegawaiID, trackingID string) bool {
	if pegawaiID == "" {
		return false
	}
	var followUp models.FollowUp
	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		First(&followUp).Error; err != nil {
		return false
	}
	return followUp.AdminProyekID != nil && *followUp.AdminProyekID == pegawaiID
}

// isRelatedSales ngecek apakah pegawai yang login ini adalah Sales/Marketing
// yang bikin tracking ini (TrackingPenawaran.MarketingID) -- bukan divisi
// tetap kayak Supervisi Sales di atas, tapi pegawai spesifik per-tracking,
// mirip isAssignedAdminProyek.
func isRelatedSales(pegawaiID, trackingID string) bool {
	if pegawaiID == "" {
		return false
	}
	var tracking models.TrackingPenawaran
	if err := config.DB.Where("id = ?", trackingID).First(&tracking).Error; err != nil {
		return false
	}
	return tracking.MarketingID == pegawaiID
}

// canViewStepForTracking = canViewStep + pengecualian yang aksesnya
// tergantung assignment per-tracking, bukan cuma divisi: Admin Proyek buat
// step Follow Up/Implementasi/BAST/Garansi, dan Sales terkait buat step
// Review Internal.
func canViewStepForTracking(step models.StepPenawaran, roleStr, divisiStr, pegawaiID, trackingID string) bool {
	if canViewStep(step, roleStr, divisiStr) {
		return true
	}
	switch step {
	case models.StepFollowUp, models.StepImplementasi, models.StepBAST, models.StepGaransi:
		return isAssignedAdminProyek(pegawaiID, trackingID)
	case models.StepReviewInternal:
		return isRelatedSales(pegawaiID, trackingID)
	}
	return false
}

// canEditImplementasiBarang menentukan siapa yang boleh mengisi/mengubah/
// menghapus daftar barang pembelian (tahap implementasi): hanya divisi
// PROCUREMENT_GA (semua role di divisi itu) + MASTER sebagai super admin
// — target_hari_ini.md poin 1. Berbeda dari matriks view StepImplementasi
// yang lebih longgar.
func canEditImplementasiBarang(roleStr, divisiStr string) bool {
	if roleStr == "MASTER" {
		return true
	}
	return divisiStr == "PROCUREMENT_GA"
}

// canManageBastGaransi menentukan siapa yang boleh menambah/mengubah/melakukan
// action di tahap BAST (step 7) & Garansi (step 8) — target_hari_ini.md poin 2:
// hanya Admin Proyek yang di-assign per-tracking, role MASTER, dan divisi
// MANAGER_OPERASIONAL. Direktur/Komisaris view-only (tetap bisa lihat lewat
// canViewStepForTracking).
func canManageBastGaransi(pegawaiID, trackingID, roleStr, divisiStr string) bool {
	if roleStr == "MASTER" {
		return true
	}
	if divisiStr == "MANAGER_OPERASIONAL" {
		return true
	}
	return isAssignedAdminProyek(pegawaiID, trackingID)
}
