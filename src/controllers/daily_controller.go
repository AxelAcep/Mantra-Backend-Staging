package controllers

import (
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"mantra/src/config"
	"mantra/src/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ==========================================
// HELPER — Generate Activity ID
// ==========================================

func generateActivityID() string {
	const characters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	result := make([]byte, 10)
	for i := range result {
		result[i] = characters[r.Intn(len(characters))]
	}
	return "PM-" + string(result)
}

func generateKolaboratorID() string {
	const characters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	result := make([]byte, 10)
	for i := range result {
		result[i] = characters[r.Intn(len(characters))]
	}
	return "KOL-" + string(result)
}

// ==========================================
// REQUEST STRUCT
// ==========================================

type KolaboratorRequest struct {
	PegawaiID string `json:"pegawaiId"`
	Judul     string `json:"judul"`
	Deskripsi string `json:"deskripsi"`
	Kategori  string `json:"kategori"`
}

type CreateActivityRequest struct {
	TerkaitPO     *string              `json:"terkaitPO"`
	Perusahaan    *string              `json:"perusahaan"`
	Kategori      string               `json:"kategori"`
	Judul         string               `json:"judul"`
	Deskripsi     string               `json:"deskripsi"`
	WaktuMulai    time.Time            `json:"waktuMulai"`
	TargetSelesai time.Time            `json:"targetSelesai"`
	Kolaborator   []KolaboratorRequest `json:"kolaborator"`
}

// ==========================================
// CREATE ACTIVITY
// ==========================================

func CreateActivity(c echo.Context) error {
	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	pegawaiMap, ok := claims["pegawai"].(map[string]interface{})
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiID, _ := pegawaiMap["id"].(string)

	var req CreateActivityRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}

	if req.Kategori == "" || req.Judul == "" || req.Deskripsi == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Kategori, judul, dan deskripsi wajib diisi."})
	}
	if req.TargetSelesai.Before(req.WaktuMulai) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Target selesai tidak boleh sebelum waktu mulai."})
	}

	var activity models.Activity
	var kolaboratorList []models.ActivityKolaborator
	var childActivities []models.Activity

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		// Buat activity utama
		activity = models.Activity{
			ID:            generateActivityID(),
			PegawaiID:     pegawaiID,
			TerkaitPO:     req.TerkaitPO,
			Perusahaan:    req.Perusahaan,
			Kategori:      models.KategoriActivity(req.Kategori),
			Judul:         req.Judul,
			Deskripsi:     req.Deskripsi,
			WaktuMulai:    time.Now(),
			TargetSelesai: time.Now().Add(24 * time.Hour),
			Status:        models.StatusOnProgress,
		}
		if err := tx.Create(&activity).Error; err != nil {
			return err
		}

		// Buat child activity per kolaborator
		for _, kol := range req.Kolaborator {
			if kol.PegawaiID == "" || kol.Judul == "" || kol.Deskripsi == "" || kol.Kategori == "" {
				return fmt.Errorf("data kolaborator tidak lengkap")
			}

			childActivity := models.Activity{
				ID:                     generateActivityID(),
				PegawaiID:              kol.PegawaiID,
				ParentID:               &activity.ID,
				TerkaitPO:              req.TerkaitPO,
				Perusahaan:             req.Perusahaan,
				Kategori:               models.KategoriActivity(kol.Kategori),
				Judul:                  kol.Judul,
				Deskripsi:              kol.Deskripsi,
				WaktuMulai:             time.Now(),
				TargetSelesai:          time.Now().Add(24 * time.Hour),
				Status:                 models.StatusPending,
				IsKonfirmasiKolaborasi: true,
			}
			if err := tx.Create(&childActivity).Error; err != nil {
				return err
			}
			childActivities = append(childActivities, childActivity)

			kolaborator := models.ActivityKolaborator{
				ID:              generateKolaboratorID(),
				ActivityID:      activity.ID,
				PegawaiID:       kol.PegawaiID,
				ChildActivityID: &childActivity.ID,
				Judul:           kol.Judul,
				Status:          models.StatusPending,
			}
			if err := tx.Create(&kolaborator).Error; err != nil {
				return err
			}
			kolaboratorList = append(kolaboratorList, kolaborator)

			// Kirim notifikasi ke kolaborator
			notif := models.Notifikasi{
				ID:         fmt.Sprintf("NTF-%s-%d", childActivity.ID, time.Now().UnixNano()),
				PegawaiID:  kol.PegawaiID,
				ActivityID: &childActivity.ID,
				Judul:      "Kamu mendapatkan tugas baru",
				Pesan:      fmt.Sprintf("Kamu ditugaskan oleh %s untuk: %s", pegawaiID, kol.Judul),
				IsRead:     false,
			}
			if err := tx.Create(&notif).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		if err.Error() == "data kolaborator tidak lengkap" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message":     "Activity berhasil dibuat.",
		"activity":    activity,
		"kolaborator": kolaboratorList,
		"children":    childActivities,
	})
}

// ==========================================
// HELPER — Base Activity Query By Pegawai
// ==========================================

func baseActivityQuery(pegawaiID string) *gorm.DB {
	now := time.Now()
	return config.DB.Model(&models.Activity{}).
		Where("pegawai_id = ? AND status != ?", pegawaiID, models.StatusDibatalkan).
		Preload("Pegawai").
		Preload("Kolaborator").
		Preload("Dokumen").
		Preload("Reschedule").
		Order(fmt.Sprintf(`
			CASE
				WHEN status != 'DITERIMA' AND target_selesai < '%s' THEN 1
				WHEN status = 'DITOLAK' THEN 2
				WHEN status = 'PENDING' THEN 3
				WHEN status = 'ON_PROGRESS' THEN 4
				WHEN status = 'DITERIMA' THEN 5
				ELSE 6
			END ASC, created_at DESC
		`, now.Format("2006-01-02 15:04:05")))
}

func paginateActivity(c echo.Context, query *gorm.DB) error {
	page := 1
	limit := 10

	if p := c.QueryParam("page"); p != "" {
		if val, err := strconv.Atoi(p); err == nil && val > 0 {
			page = val
		}
	}
	if l := c.QueryParam("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	offset := (page - 1) * limit

	var total int64
	query.Count(&total)

	var activities []models.Activity
	if err := query.Limit(limit).Offset(offset).Find(&activities).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Data activity berhasil diambil.",
		"data":    activities,
		"meta": map[string]interface{}{
			"page":       page,
			"limit":      limit,
			"total":      total,
			"totalPages": totalPages,
		},
	})
}

func getPegawaiID(c echo.Context) (string, bool) {
	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return "", false
	}
	pegawaiMap, ok := claims["pegawai"].(map[string]interface{})
	if !ok {
		return "", false
	}
	id, _ := pegawaiMap["id"].(string)
	return id, id != ""
}

// ==========================================
// GET ACTIVITY BERJALAN (selain DITERIMA)
// ==========================================

func GetActivityBerjalan(c echo.Context) error {
	pegawaiID, ok := getPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	query := baseActivityQuery(pegawaiID).
		Where("status != ?", models.StatusDiterima)

	return paginateActivity(c, query)
}

// ==========================================
// GET ACTIVITY AKTIF (ON_PROGRESS)
// ==========================================

func GetActivityAktif(c echo.Context) error {
	pegawaiID, ok := getPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	query := baseActivityQuery(pegawaiID).
		Where("status = ?", models.StatusOnProgress)

	return paginateActivity(c, query)
}

// ==========================================
// GET ACTIVITY PENDING
// ==========================================

func GetActivityPending(c echo.Context) error {
	pegawaiID, ok := getPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	query := baseActivityQuery(pegawaiID).
		Where("status = ? OR status = ?", models.StatusPending, models.StatusKonfirmasiSelesai)

	return paginateActivity(c, query)
}

// ==========================================
// GET ACTIVITY PERLU TINDAKAN (DITOLAK + OVERDUE)
// ==========================================

func GetActivityPerluTindakan(c echo.Context) error {
	pegawaiID, ok := getPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	now := time.Now()
	query := baseActivityQuery(pegawaiID).
		// Menambahkan filter agar status 'Pending' tidak ikut ditarik sama sekali
		Where("status != ?", models.StatusPending).
		Where(
			config.DB.Where("status = ?", models.StatusDitolak).
				Or("status = ?", models.StatusPendingPegawai).
				Or("target_selesai < ? AND status = ?", now, models.StatusOnProgress),
		)

	return paginateActivity(c, query)
}

// ==========================================
// GET ACTIVITY RIWAYAT (semua)
// ==========================================

func GetActivityRiwayat(c echo.Context) error {
	pegawaiID, ok := getPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	query := baseActivityQuery(pegawaiID).Where("status IN ?", []string{"DITERIMA", "DIBATALKAN"})
	return paginateActivity(c, query)
}

// ==========================================
// GET ACTIVITY COUNT DASHBOARD
// ==========================================

func GetActivityCount(c echo.Context) error {
	pegawaiID, ok := getPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayEnd := todayStart.Add(24 * time.Hour)

	var aktif, deadlineHariIni, approval, overdue int64

	config.DB.Model(&models.Activity{}).
		Where("pegawai_id = ? AND status = ?", pegawaiID, models.StatusOnProgress).
		Count(&aktif)

	config.DB.Model(&models.Activity{}).
		Where("pegawai_id = ? AND target_selesai >= ? AND target_selesai < ? AND status NOT IN ?",
			pegawaiID, todayStart, todayEnd,
			[]string{string(models.StatusDiterima)},
		).Count(&deadlineHariIni)

	config.DB.Model(&models.Activity{}).
		Where("pegawai_id = ? AND status IN ?", pegawaiID, []string{
			string(models.StatusPending),
			string(models.StatusKonfirmasiSelesai),
		}).
		Count(&approval)

	config.DB.Model(&models.Activity{}).
		Where("pegawai_id = ? AND target_selesai < ? AND status IN ?",
			pegawaiID, now,
			models.StatusOnProgress,
		).Count(&overdue)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Count berhasil diambil.",
		"data": map[string]interface{}{
			"aktif":           aktif,
			"deadlineHariIni": deadlineHariIni,
			"approval":        approval,
			"overdue":         overdue,
		},
	})
}

// ==========================================
// GET DETAIL ACTIVITY
// ==========================================

func GetDetailActivity(c echo.Context) error {
	id := c.Param("id")

	var activity models.Activity
	err := config.DB.
		Preload("Pegawai").
		Preload("Kolaborator").
		Preload("Kolaborator.Pegawai").
		Preload("Kolaborator.ChildActivity").
		Preload("Dokumen").
		Preload("Dokumen.Pegawai").
		Preload("Reschedule").
		Preload("Chat").
		Preload("Chat.Pegawai").
		Preload("Children").
		Preload("Children.Pegawai").
		Preload("Parent").
		Preload("Parent.Pegawai").
		Where("id = ?", id).
		First(&activity).Error

	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Activity tidak ditemukan."})
	}

	// Cek apakah overdue
	isOverdue := (activity.Status == models.StatusOnProgress) &&
		time.Now().After(activity.TargetSelesai)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":   "Detail activity berhasil diambil.",
		"data":      activity,
		"isOverdue": isOverdue,
	})
}

// ==========================================
// TAMBAH KOLABORATOR
// ==========================================

type TambahKolaboratorRequest struct {
	PegawaiID string `json:"pegawaiId"`
	Judul     string `json:"judul"`
	Deskripsi string `json:"deskripsi"`
	Kategori  string `json:"kategori"`
}

func TambahKolaborator(c echo.Context) error {
	activityID := c.Param("id")

	var req TambahKolaboratorRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}

	if req.PegawaiID == "" || req.Judul == "" || req.Deskripsi == "" || req.Kategori == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Semua field wajib diisi."})
	}

	// Cek activity ada
	var activity models.Activity
	if err := config.DB.Where("id = ?", activityID).First(&activity).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Activity tidak ditemukan."})
	}

	var kolaborator models.ActivityKolaborator
	var childActivity models.Activity

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		childActivity = models.Activity{
			ID:                     generateActivityID(),
			PegawaiID:              req.PegawaiID,
			ParentID:               &activity.ID,
			TerkaitPO:              activity.TerkaitPO,
			Perusahaan:             activity.Perusahaan,
			Kategori:               models.KategoriActivity(req.Kategori),
			Judul:                  req.Judul,
			Deskripsi:              req.Deskripsi,
			WaktuMulai:             time.Now(),
			TargetSelesai:          time.Now().Add(24 * time.Hour),
			Status:                 models.StatusPendingPegawai,
			IsKonfirmasiKolaborasi: true,
		}
		if err := tx.Create(&childActivity).Error; err != nil {
			return err
		}

		kolaborator = models.ActivityKolaborator{
			ID:              generateKolaboratorID(),
			ActivityID:      activity.ID,
			PegawaiID:       req.PegawaiID,
			ChildActivityID: &childActivity.ID,
			Judul:           req.Judul,
			Status:          models.StatusOnProgress,
		}
		if err := tx.Create(&kolaborator).Error; err != nil {
			return err
		}

		notif := models.Notifikasi{
			ID:         fmt.Sprintf("NTF-%s-%d", childActivity.ID, time.Now().UnixNano()),
			PegawaiID:  req.PegawaiID,
			ActivityID: &childActivity.ID,
			Judul:      "Kamu mendapatkan tugas baru",
			Pesan:      fmt.Sprintf("Kamu ditugaskan untuk: %s", req.Judul),
			IsRead:     false,
		}
		return tx.Create(&notif).Error
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message":       "Kolaborator berhasil ditambahkan.",
		"kolaborator":   kolaborator,
		"childActivity": childActivity,
	})
}

// ==========================================
// PENGAJUAN RESCHEDULE
// ==========================================

type RescheduleRequest struct {
	Alasan            string    `json:"alasan"`
	TargetSelesaiBaru time.Time `json:"targetSelesaiBaru"`
}

func PengajuanReschedule(c echo.Context) error {
	activityID := c.Param("id")

	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)

	var req RescheduleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}
	if req.Alasan == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Alasan wajib diisi."})
	}
	if req.TargetSelesaiBaru.IsZero() {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Target selesai baru wajib diisi."})
	}

	// Cek activity ada dan milik pegawai ini
	var activity models.Activity
	if err := config.DB.Where("id = ? AND pegawai_id = ?", activityID, pegawaiID).First(&activity).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Activity tidak ditemukan."})
	}

	// Cek tidak ada reschedule pending
	var existingReschedule models.ActivityReschedule
	if err := config.DB.Where("activity_id = ? AND status = ?", activityID, models.StatusReschedulePending).First(&existingReschedule).Error; err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "Masih ada pengajuan reschedule yang menunggu konfirmasi."})
	}

	// Cek target baru harus setelah sekarang
	if req.TargetSelesaiBaru.Before(time.Now()) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Mohon pilih target selesai baru yang melewati waktu saat ini."})
	}

	isOverdue := (activity.Status == models.StatusOnProgress) &&
		time.Now().After(activity.TargetSelesai)

	var reschedule models.ActivityReschedule
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		reschedule = models.ActivityReschedule{
			ID:                generateKolaboratorID(),
			ActivityID:        activityID,
			TargetSelesaiBaru: req.TargetSelesaiBaru,
			Alasan:            req.Alasan,
			Status:            models.StatusReschedulePending,
		}
		if err := tx.Create(&reschedule).Error; err != nil {
			return err
		}

		if err := tx.Model(&activity).Update("status", models.StatusPending).Error; err != nil {
			return err
		}

		// Notif ke semua Master
		var masters []models.User
		if err := tx.Where("role = ?", models.RoleMaster).Find(&masters).Error; err != nil {
			return err
		}

		overdueInfo := ""
		if isOverdue {
			overdueInfo = " (OVERDUE)"
		}

		for _, master := range masters {
			notif := models.Notifikasi{
				ID:         fmt.Sprintf("NTF-%s-%s-%d", reschedule.ID, master.PegawaiID, time.Now().UnixNano()),
				PegawaiID:  master.PegawaiID,
				ActivityID: &activityID,
				Judul:      "Pengajuan Reschedule" + overdueInfo,
				Pesan:      fmt.Sprintf("Activity %s mengajukan reschedule: %s", activityID, req.Alasan),
				IsRead:     false,
			}
			if err := tx.Create(&notif).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message":    "Pengajuan reschedule berhasil dikirim.",
		"reschedule": reschedule,
		"isOverdue":  isOverdue,
	})
}

// ==========================================
// KONFIRMASI RESCHEDULE — MASTER
// ==========================================

type KonfirmasiRescheduleRequest struct {
	Status string `json:"status"`
	Alasan string `json:"alasan"`
}

func KonfirmasiReschedule(c echo.Context) error {
	rescheduleID := c.Param("rescheduleId")

	var req KonfirmasiRescheduleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}

	// Validasi status
	if req.Status != string(models.StatusRescheduleDiterima) &&
		req.Status != string(models.StatusRescheduleDitolak) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Status harus DITERIMA atau DITOLAK."})
	}

	if req.Status == string(models.StatusRescheduleDitolak) && req.Alasan == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Alasan penolakan wajib diisi."})
	}

	// Ambil data reschedule
	var reschedule models.ActivityReschedule
	if err := config.DB.
		Where("id = ? AND status = ?", rescheduleID, models.StatusReschedulePending).
		First(&reschedule).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Pengajuan reschedule tidak ditemukan atau sudah diproses."})
	}

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		// Ambil activity
		var activity models.Activity
		if err := tx.Where("id = ?", reschedule.ActivityID).First(&activity).Error; err != nil {
			return err
		}

		// ===== UPDATE RESCHEDULE =====
		reschedule.Status = models.StatusReschedule(req.Status)
		if req.Status == string(models.StatusRescheduleDitolak) {
			reschedule.AlasanPenolakan = &req.Alasan
		}

		if err := tx.Save(&reschedule).Error; err != nil {
			return err
		}

		// ===== UPDATE ACTIVITY =====
		if req.Status == string(models.StatusRescheduleDiterima) {
			activity.TargetSelesai = reschedule.TargetSelesaiBaru
			activity.Status = models.StatusOnProgress

			// update parent jika ada
			if activity.ParentID != nil {
				if err := tx.Model(&models.Activity{}).
					Where("id = ?", *activity.ParentID).
					Updates(map[string]interface{}{
						"target_selesai": reschedule.TargetSelesaiBaru,
						"status":         models.StatusOnProgress,
					}).Error; err != nil {
					return err
				}
			}

		} else {
			activity.Status = models.StatusDitolak

			// update collaborator
			if err := tx.Model(&models.ActivityKolaborator{}).
				Where("child_activity_id = ?", activity.ID).
				Update("status", models.StatusDitolak).Error; err != nil {
				return err
			}
		}

		if err := tx.Save(&activity).Error; err != nil {
			return err
		}

		// ===== NOTIF =====
		var pesanNotif string
		if req.Status == string(models.StatusRescheduleDiterima) {
			pesanNotif = fmt.Sprintf(
				"Reschedule activity %s diterima. Target baru: %s",
				activity.ID,
				reschedule.TargetSelesaiBaru.Format("02 Jan 2006 15:04"),
			)
		} else {
			pesanNotif = fmt.Sprintf(
				"Reschedule activity %s ditolak. Alasan: %s",
				activity.ID,
				req.Alasan,
			)
		}

		notif := models.Notifikasi{
			ID:         fmt.Sprintf("NTF-KR-%s-%d", reschedule.ID, time.Now().UnixNano()),
			PegawaiID:  activity.PegawaiID,
			ActivityID: &activity.ID,
			Judul:      "Konfirmasi Reschedule",
			Pesan:      pesanNotif,
			IsRead:     false,
		}

		if err := tx.Create(&notif).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Terjadi kesalahan pada server.",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Konfirmasi reschedule berhasil.",
	})
}

// ==========================================
// PENGAJUAN SELESAI
// ==========================================

func PengajuanSelesai(c echo.Context) error {
	activityID := c.Param("id")

	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)

	var activity models.Activity
	if err := config.DB.Where("id = ? AND pegawai_id = ?", activityID, pegawaiID).First(&activity).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Activity tidak ditemukan."})
	}

	if activity.Status != models.StatusOnProgress {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Hanya activity dengan status On Progress yang bisa diajukan selesai."})
	}

	// Cek overdue — wajib reschedule dulu
	if time.Now().After(activity.TargetSelesai) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Activity sudah overdue. Ajukan reschedule terlebih dahulu."})
	}

	// Cek tidak ada reschedule pending
	var pendingReschedule models.ActivityReschedule
	if err := config.DB.Where("activity_id = ? AND status = ?", activityID, models.StatusReschedulePending).First(&pendingReschedule).Error; err == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Masih ada pengajuan reschedule yang menunggu konfirmasi."})
	}

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		activity.Status = models.StatusKonfirmasiSelesai
		activity.WaktuSubmit = &now
		if err := tx.Save(&activity).Error; err != nil {
			return err
		}

		// Notif ke semua Master
		var masters []models.User
		if err := tx.Where("role = ?", models.RoleMaster).Find(&masters).Error; err != nil {
			return err
		}

		for _, master := range masters {
			notif := models.Notifikasi{
				ID:         fmt.Sprintf("NTF-PS-%s-%s-%d", activityID, master.PegawaiID, time.Now().UnixNano()),
				PegawaiID:  master.PegawaiID,
				ActivityID: &activityID,
				Judul:      "Pengajuan Selesai",
				Pesan:      fmt.Sprintf("Activity %s mengajukan konfirmasi selesai.", activityID),
				IsRead:     false,
			}
			if err := tx.Create(&notif).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Pengajuan selesai berhasil dikirim. Menunggu konfirmasi Master.",
	})
}

// ==========================================
// KONFIRMASI PENGAJUAN SELESAI — MASTER
// ==========================================

type KonfirmasiSelesaiRequest struct {
	Status string `json:"status"`
	Alasan string `json:"alasan"`
}

func KonfirmasiSelesai(c echo.Context) error {
	activityID := c.Param("id")

	var req KonfirmasiSelesaiRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}

	if req.Status != string(models.StatusDiterima) && req.Status != string(models.StatusDitolak) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Status harus DITERIMA atau DITOLAK."})
	}

	if req.Status == string(models.StatusDitolak) && req.Alasan == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Alasan penolakan wajib diisi."})
	}

	var activity models.Activity
	if err := config.DB.Where("id = ? AND status = ?", activityID, models.StatusKonfirmasiSelesai).First(&activity).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Activity tidak ditemukan atau tidak dalam status Pending."})
	}

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		activity.Status = models.StatusActivity(req.Status)

		pesanNotif := "Activity kamu telah diterima dan dinyatakan selesai."
		judulNotif := "Activity Diterima"

		if req.Status == string(models.StatusDitolak) {
			pesanNotif = fmt.Sprintf("Pengajuan selesai ditolak. Alasan: %s", req.Alasan)
			judulNotif = "Pengajuan Selesai Ditolak"
			activity.AlasanPenolakan = &req.Alasan

			if err := tx.Model(&models.ActivityKolaborator{}).
				Where("child_activity_id = ?", activityID).
				Update("status", models.StatusDitolak).Error; err != nil {
				return err
			}
		}

		if req.Status == string(models.StatusDiterima) {
			if err := tx.Model(&models.ActivityKolaborator{}).
				Where("child_activity_id = ?", activityID).
				Update("status", models.StatusDiterima).Error; err != nil {
				return err
			}
		}

		if err := tx.Save(&activity).Error; err != nil {
			return err
		}

		notif := models.Notifikasi{
			ID:         fmt.Sprintf("NTF-KS-%s-%d", activityID, time.Now().UnixNano()),
			PegawaiID:  activity.PegawaiID,
			ActivityID: &activityID,
			Judul:      judulNotif,
			Pesan:      pesanNotif,
			IsRead:     false,
		}
		return tx.Create(&notif).Error
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Konfirmasi berhasil."})
}

// ==========================================
// GET CHAT BY ACTIVITY
// ==========================================

func GetChat(c echo.Context) error {
	activityID := c.Param("id")

	var activity models.Activity
	if err := config.DB.Where("id = ?", activityID).First(&activity).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Activity tidak ditemukan."})
	}

	var chats []models.ActivityChat
	if err := config.DB.
		Preload("Pegawai").
		Where("activity_id = ?", activityID).
		Order("created_at ASC").
		Find(&chats).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Chat berhasil diambil.",
		"data":    chats,
	})
}

// ==========================================
// KIRIM CHAT
// ==========================================

type KirimChatRequest struct {
	Pesan string `json:"pesan"`
}

func KirimChat(c echo.Context) error {
	activityID := c.Param("id")

	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)
	role, _ := claims["role"].(string)

	var req KirimChatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}
	if req.Pesan == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Pesan tidak boleh kosong."})
	}

	// Cek activity ada
	var activity models.Activity
	if err := config.DB.Preload("Pegawai").Where("id = ?", activityID).First(&activity).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Activity tidak ditemukan."})
	}

	// Ambil data pegawai pengirim
	var pengirim models.Pegawai
	if err := config.DB.Where("id = ?", pegawaiID).First(&pengirim).Error; err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Data pegawai tidak ditemukan."})
	}

	// === Validasi izin kirim chat ===
	isMaster := role == string(models.RoleMaster)
	isOwner := activity.PegawaiID == pegawaiID

	// Supervisi dengan divisi sama dengan pembuat activity
	isSupervisiSameDivisi := role == string(models.RoleSupervisi) &&
		pengirim.Divisi == activity.Pegawai.Divisi

	// Pembuat parent activity
	isParentOwner := false
	if activity.ParentID != nil {
		var parentActivity models.Activity
		if err := config.DB.Where("id = ?", *activity.ParentID).First(&parentActivity).Error; err == nil {
			isParentOwner = parentActivity.PegawaiID == pegawaiID
		}
	}

	if !isMaster && !isOwner && !isSupervisiSameDivisi && !isParentOwner {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Anda tidak memiliki akses untuk mengirim pesan di activity ini."})
	}

	// === Kirim chat ===
	var chat models.ActivityChat
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		chat = models.ActivityChat{
			ID:         "CHT-" + generateActivityID(),
			ActivityID: activityID,
			PegawaiID:  pegawaiID,
			Pesan:      req.Pesan,
		}
		if err := tx.Create(&chat).Error; err != nil {
			return err
		}

		// Notif ke lawan chat
		if isMaster {
			// Notif ke pemilik activity
			notif := models.Notifikasi{
				ID:         fmt.Sprintf("NTF-CHT-%s-%d", chat.ID, time.Now().UnixNano()),
				PegawaiID:  activity.PegawaiID,
				ActivityID: &activityID,
				Judul:      "Pesan baru",
				Pesan:      fmt.Sprintf("Pesan baru dari activity %s", activityID),
				IsRead:     false,
			}
			return tx.Create(&notif).Error
		}

		// Notif ke semua Master
		var masters []models.User
		if err := tx.Where("role = ?", models.RoleMaster).Find(&masters).Error; err != nil {
			return err
		}
		for _, master := range masters {
			notif := models.Notifikasi{
				ID:         fmt.Sprintf("NTF-CHT-%s-%s-%d", chat.ID, master.PegawaiID, time.Now().UnixNano()),
				PegawaiID:  master.PegawaiID,
				ActivityID: &activityID,
				Judul:      "Pesan baru",
				Pesan:      fmt.Sprintf("Pesan baru dari activity %s", activityID),
				IsRead:     false,
			}
			if err := tx.Create(&notif).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	config.DB.Preload("Pegawai").First(&chat, "id = ?", chat.ID)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "Pesan berhasil dikirim.",
		"data":    chat,
	})
}

// ==========================================
// READ CHAT
// ==========================================

func ReadChat(c echo.Context) error {
	activityID := c.Param("id")

	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)

	// Ambil semua chat di activity ini yang belum dibaca oleh pegawai ini
	var chats []models.ActivityChat
	if err := config.DB.Where("activity_id = ?", activityID).Find(&chats).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	for _, chat := range chats {
		// Skip kalau pengirim adalah diri sendiri
		if chat.PegawaiID == pegawaiID {
			continue
		}

		// Cek apakah sudah ada di ReadBy
		alreadyRead := false
		for _, id := range chat.ReadBy {
			if id == pegawaiID {
				alreadyRead = true
				break
			}
		}

		if !alreadyRead {
			chat.ReadBy = append(chat.ReadBy, pegawaiID)
			config.DB.Save(&chat)
		}
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Chat berhasil ditandai sudah dibaca.",
	})
}

// ==========================================
// GET UNREAD COUNT
// ==========================================

func GetUnreadChatCount(c echo.Context) error {
	activityID := c.Param("id")

	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)

	var chats []models.ActivityChat
	if err := config.DB.Where("activity_id = ? AND pegawai_id != ?", activityID, pegawaiID).Find(&chats).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	unread := 0
	for _, chat := range chats {
		alreadyRead := false
		for _, id := range chat.ReadBy {
			if id == pegawaiID {
				alreadyRead = true
				break
			}
		}
		if !alreadyRead {
			unread++
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Berhasil.",
		"data": map[string]int{
			"unread": unread,
		},
	})
}

// ==========================================
// GET ACTIVITY MENUNGGU KONFIRMASI KOLABORASI
// ==========================================

func GetActivityKonfirmasiKolaborasi(c echo.Context) error {
	pegawaiID, ok := getPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var activities []models.Activity
	if err := config.DB.
		Preload("Pegawai").
		Preload("Parent").
		Preload("Parent.Pegawai").
		Where("pegawai_id = ? AND is_konfirmasi_kolaborasi = ?", pegawaiID, true).
		Order("created_at DESC").
		Find(&activities).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Data berhasil diambil.",
		"data":    activities,
	})
}

// ==========================================
// KONFIRMASI KOLABORASI
// ==========================================

type KonfirmasiKolaborasiRequest struct {
	Status    string `json:"status"`
	Deskripsi string `json:"deskripsi"`
	Kategori  string `json:"kategori"`
}

func KonfirmasiKolaborasi(c echo.Context) error {
	activityID := c.Param("id")

	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)

	var req KonfirmasiKolaborasiRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}
	if req.Status != "DITERIMA" && req.Status != "DITOLAK" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Status harus DITERIMA atau DITOLAK."})
	}

	var activity models.Activity
	if err := config.DB.Where("id = ? AND pegawai_id = ? AND is_konfirmasi_kolaborasi = ?", activityID, pegawaiID, true).First(&activity).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Activity tidak ditemukan."})
	}

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		if req.Status == "DITERIMA" {
			activity.IsKonfirmasiKolaborasi = false
			activity.Status = models.StatusOnProgress
			if req.Deskripsi != "" {
				activity.Deskripsi = req.Deskripsi
			}
			if req.Kategori != "" {
				activity.Kategori = models.KategoriActivity(req.Kategori)
			}
		} else {
			activity.IsKonfirmasiKolaborasi = false
			activity.Status = models.StatusDibatalkan
		}

		if err := tx.Save(&activity).Error; err != nil {
			return err
		}

		kolStatus := models.StatusOnProgress
		if req.Status == "DITOLAK" {
			kolStatus = models.StatusDibatalkan
		}
		if err := tx.Model(&models.ActivityKolaborator{}).
			Where("child_activity_id = ?", activityID).
			Update("status", kolStatus).Error; err != nil {
			return err
		}

		if activity.ParentID != nil {
			var parent models.Activity
			if err := tx.Where("id = ?", *activity.ParentID).First(&parent).Error; err != nil {
				return err
			}

			pesanNotif := fmt.Sprintf("%s menerima tugas kolaborasi: %s", pegawaiID, activity.Judul)
			if req.Status == "DITOLAK" {
				pesanNotif = fmt.Sprintf("%s menolak tugas kolaborasi: %s", pegawaiID, activity.Judul)
			}

			notif := models.Notifikasi{
				ID:         fmt.Sprintf("NTF-KOL-%s-%d", activityID, time.Now().UnixNano()),
				PegawaiID:  parent.PegawaiID,
				ActivityID: &activity.ID,
				Judul:      "Konfirmasi Kolaborasi",
				Pesan:      pesanNotif,
				IsRead:     false,
			}
			return tx.Create(&notif).Error
		}

		return nil
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Konfirmasi kolaborasi berhasil.",
	})
}
