package controllers

import (
	"fmt"
	"mantra/src/config"
	"mantra/src/models"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ─── Helper ───────────────────────────────────────────────────────────────────

func getPenawaranPegawaiID(c echo.Context) (string, bool) {
	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return "", false
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)
	return pegawaiID, pegawaiID != ""
}

type KirimPenawaranChatRequest struct {
	Pesan string `json:"pesan"`
}

func todayAt5PM() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 17, 0, 0, 0, now.Location())
}

// ─── Request DTO ──────────────────────────────────────────────────────────────

type CreateTrackingPenawaranRequest struct {
	NomorPenawaran string                  `json:"nomorPenawaran" validate:"required"`
	PerusahaanID   string                  `json:"perusahaanId"   validate:"required"`
	LokasiProyek   string                  `json:"lokasiProyek"   validate:"required"`
	CustomerName   string                  `json:"customerName"   validate:"required"`
	CustomerPhone  string                  `json:"customerPhone"  validate:"required"`
	CustomerEmail  string                  `json:"customerEmail"  validate:"required"`
	JenisPenawaran []models.JenisPenawaran `json:"jenisPenawaran" validate:"required,min=1"`
}

// ─── Controllers ──────────────────────────────────────────────────────────────

// POST /tracking-penawaran
func CreateTrackingPenawaran(c echo.Context) error {
	pegawaiID, ok := getPenawaranPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	var req CreateTrackingPenawaranRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Request tidak valid"})
	}

	var existing models.TrackingPenawaran
	if err := config.DB.Where(`"nomorPenawaran" = ?`, req.NomorPenawaran).First(&existing).Error; err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"message": "Nomor penawaran sudah digunakan"})
	}

	var perusahaan models.Perusahaan
	if err := config.DB.First(&perusahaan, `"id" = ?`, req.PerusahaanID).Error; err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Perusahaan tidak ditemukan"})
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal memulai transaksi"})
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	trackingID := uuid.NewString()
	tracking := models.TrackingPenawaran{
		ID:             trackingID,
		NomorPenawaran: req.NomorPenawaran,
		PerusahaanID:   req.PerusahaanID,
		MarketingID:    pegawaiID,
		LokasiProyek:   req.LokasiProyek,
		CustomerName:   req.CustomerName,
		CustomerPhone:  req.CustomerPhone,
		CustomerEmail:  req.CustomerEmail,
		JenisPenawaran: req.JenisPenawaran,
		StepSaatIni:    models.StepPermintaanMasuk,
	}
	if err := tx.Create(&tracking).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal membuat tracking penawaran"})
	}

	activityID := uuid.NewString()
	activity := models.Activity{
		ID:            activityID,
		PegawaiID:     pegawaiID,
		Kategori:      models.KategoriQuotation,
		TerkaitPO:	   &req.NomorPenawaran,
		Perusahaan:    &perusahaan.Nama,
		Judul:         "Permintaan Masuk oleh " + perusahaan.Nama,
		Deskripsi:     "Activity otomatis dari penawaran #" + req.NomorPenawaran,
		WaktuMulai:    time.Now(),
		TargetSelesai: todayAt5PM(),
		Status:        models.StatusOnProgress,
	}
	if err := tx.Create(&activity).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal membuat activity"})
	}

	permintaanID := uuid.NewString()
	permintaan := models.PermintaanMasuk{
		ID:                  permintaanID,
		TrackingPenawaranID: trackingID,
		ActivityID:          &activityID,
		Status:              models.StatusOnProgress,
	}
	if err := tx.Create(&permintaan).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal membuat permintaan masuk"})
	}

	if err := tx.Commit().Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal commit transaksi"})
	}

	var result models.TrackingPenawaran
	config.DB.
		Preload("Perusahaan").
		Preload("Marketing").
		Preload("PermintaanMasuk").
		Preload("PermintaanMasuk.Activity").
		First(&result, `"id" = ?`, trackingID)

	return c.JSON(http.StatusCreated, result)
}

// GET /tracking-penawaran
func GetTrackingPenawaranMarketing(c echo.Context) error {
	pegawaiID, ok := getPenawaranPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	var list []models.TrackingPenawaran
	config.DB.
		Preload("Perusahaan").
		Preload("PermintaanMasuk").
		Where(`"marketingId" = ?`, pegawaiID).
		Order(`"createdAt" DESC`).
		Find(&list)

	return c.JSON(http.StatusOK, list)
}

// GET /tracking-penawaran/mo/all
func GetTrackingPenawaranMO(c echo.Context) error {
	var list []models.TrackingPenawaran
	config.DB.
		Preload("Perusahaan").
		Preload("Marketing").
		Preload("PermintaanMasuk").
		Preload("PermintaanMasuk.PreSales").
		Order(`"createdAt" DESC`).
		Find(&list)

	return c.JSON(http.StatusOK, list)
}

// GET /tracking-penawaran/:id
func GetDetailTrackingPenawaran(c echo.Context) error {
	id := c.Param("id")

	var tracking models.TrackingPenawaran
	err := config.DB.
		Preload("Perusahaan").
		Preload("Marketing").
		Preload("PermintaanMasuk").
		Preload("PermintaanMasuk.PreSales").
		Preload("PermintaanMasuk.Activity").
		Preload("PermintaanMasuk.Dokumen").
		Preload("Chat").
		Preload("Chat.Pegawai").
		First(&tracking, `"id" = ?`, id).Error

	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"message": "Tracking penawaran tidak ditemukan"})
	}

	return c.JSON(http.StatusOK, tracking)
}

// PATCH /tracking-penawaran/:id/presales
func AssignPreSales(c echo.Context) error {
    trackingID := c.Param("id")

    claims, ok := c.Get("user").(jwt.MapClaims)
    if !ok {
        return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
    }
    pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
    pegawaiID, _ := pegawaiMap["id"].(string)
    namaPegawai, _ := pegawaiMap["nama"].(string)

    var body struct {
        PreSalesID string `json:"preSalesId"`
    }
    if err := c.Bind(&body); err != nil || body.PreSalesID == "" {
        return c.JSON(http.StatusBadRequest, map[string]string{"error": "preSalesId wajib diisi."})
    }

    var pegawai models.Pegawai
    if err := config.DB.Where("id = ?", body.PreSalesID).First(&pegawai).Error; err != nil {
        return c.JSON(http.StatusNotFound, map[string]string{"error": "Pegawai tidak ditemukan."})
    }

    var permintaanMasuk models.PermintaanMasuk
    if err := config.DB.
        Where("tracking_penawaran_id = ?", trackingID).
        First(&permintaanMasuk).Error; err != nil {
        return c.JSON(http.StatusNotFound, map[string]string{"error": "Permintaan masuk tidak ditemukan."})
    }

    permintaanMasuk.PreSalesID = &body.PreSalesID
    appendLog(&permintaanMasuk, "Assign PreSales", pegawai.Nama, pegawaiID, namaPegawai)
    config.DB.Save(&permintaanMasuk)

    var tracking models.TrackingPenawaran
    config.DB.
        Where("id = ?", trackingID).
        Preload("Marketing").
        Preload("Perusahaan").
        Preload("PermintaanMasuk.PreSales").
        Preload("PermintaanMasuk.Activity").
        Preload("PermintaanMasuk.Dokumen").
        First(&tracking)

    return c.JSON(http.StatusOK, map[string]interface{}{
        "message": "Pre-sales berhasil di-assign.",
        "data":    tracking,
    })
}



func UpdateStatusPermintaanMasuk(c echo.Context) error {
	trackingID := c.Param("id")

	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)
	namaPegawai, _ := pegawaiMap["nama"].(string)
	roleStr, _ := claims["role"].(string)

	var body struct {
		Status string `json:"status"`
		Alasan string `json:"alasanPenolakan"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	var permintaanMasuk models.PermintaanMasuk
	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Preload("TrackingPenawaran.Perusahaan").
		Preload("Activity").
		Preload("PreSales").
		First(&permintaanMasuk).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Permintaan masuk tidak ditemukan."})
	}

	switch body.Status {

	case "PERLU_TINDAKAN":
		if roleStr != "MASTER" {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Hanya Master yang bisa menolak."})
		}
		if body.Alasan == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Alasan wajib diisi."})
		}
		permintaanMasuk.Status = models.StatusPerluTindakan
		appendLog(&permintaanMasuk, "Tolak", body.Alasan, pegawaiID, namaPegawai)
		config.DB.Save(&permintaanMasuk)
		config.DB.Model(&models.TrackingPenawaran{}).
			Where("id = ?", trackingID).
			Update("status", models.StatusPerluTindakan)

	case "KONFIRMASI_SELESAI":
		if roleStr == "MASTER" {
			if permintaanMasuk.PreSalesID == nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "PreSales belum ditentukan."})
			}

			permintaanMasuk.Status = models.StatusSelesai
			appendLog(&permintaanMasuk, "Konfirmasi Selesai Diterima", "Permintaan masuk disetujui, lanjut ke BoQ", pegawaiID, namaPegawai)
			config.DB.Save(&permintaanMasuk)

			config.DB.Model(&models.TrackingPenawaran{}).
				Where("id = ?", trackingID).
				Updates(map[string]interface{}{
					"step_saat_ini": models.StepPenyusunanBoQ,
					"status":        models.StatusOnProgress,
				})

			var existingBoQ models.PenyusunanBoQ
			boqExists := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&existingBoQ).Error == nil

			if !boqExists {
				nomorPO := ""
				if permintaanMasuk.TrackingPenawaran.NomorPO != nil {
					nomorPO = *permintaanMasuk.TrackingPenawaran.NomorPO
				}
				namaPerusahaan := permintaanMasuk.TrackingPenawaran.Perusahaan.Nama

				// Buat activity dulu, simpan ID-nya
				activityID := generateActivityID()
				dailyBoq := models.Activity{
					ID:            activityID,
					PegawaiID:     *permintaanMasuk.PreSalesID,
					TerkaitPO:     permintaanMasuk.TrackingPenawaran.NomorPO,
					Perusahaan:    &namaPerusahaan,
					Kategori:      models.KategoriBillOfQuantity,
					Judul:         "Pembuatan BOQ " + namaPerusahaan,
					Deskripsi:     "Activity otomatis dari penawaran #" + nomorPO,
					WaktuMulai:    time.Now(),
					TargetSelesai: time.Now().Add(24 * time.Hour),
					Status:        models.StatusOnProgress,
				}
				if err := config.DB.Create(&dailyBoq).Error; err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal membuat activity."})
				}

				// Baru buat BoQ, link ke activity
				boq := models.PenyusunanBoQ{
					ID:                  uuid.New().String(),
					TrackingPenawaranID: trackingID,
					PembuatID:           permintaanMasuk.PreSalesID,
					ActivityID:          &activityID,
					Status:              models.StatusOnProgress,
					LogAktivitas: []models.LogBoq{
						{
							Aksi:        "BoQ telah dimulai",
							Keterangan:  "Proses penyusunan BoQ baru saja diinisialisasi.",
							PegawaiID:   *permintaanMasuk.PreSalesID,
							NamaPegawai: permintaanMasuk.PreSales.Nama,
							CreatedAt:   time.Now(),
						},
					},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				if err := config.DB.Create(&boq).Error; err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal membuat BoQ."})
				}
			}

		} else {
			permintaanMasuk.Status = models.StatusKonfirmasiSelesai
			appendLog(&permintaanMasuk, "Konfirmasi Selesai Diajukan", "Menunggu persetujuan Master", pegawaiID, namaPegawai)
			config.DB.Save(&permintaanMasuk)
			config.DB.Model(&models.TrackingPenawaran{}).
				Where("id = ?", trackingID).
				Update("status", models.StatusKonfirmasiSelesai)
		}

	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Status tidak valid."})
	}

	var updated models.TrackingPenawaran
	config.DB.
		Where("id = ?", trackingID).
		Preload("Marketing").
		Preload("Perusahaan").
		Preload("PermintaanMasuk.PreSales").
		Preload("PermintaanMasuk.Activity").
		Preload("PermintaanMasuk.Dokumen").
		First(&updated)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Status berhasil diupdate.",
		"data":    updated,
	})
}



// ==========================================
// CHAT
// ==========================================

// controllers/tracking_penawaran_controller.go

func GetPenawaranChat(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, ok := getPenawaranPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var chats []models.PenawaranChat
	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Preload("Pegawai").
		Order("created_at ASC").
		Find(&chats).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	for i := range chats {
		if chats[i].PegawaiID == pegawaiID {
			continue
		}
		alreadyRead := false
		for _, id := range chats[i].ReadBy {
			if id == pegawaiID {
				alreadyRead = true
				break
			}
		}
		if !alreadyRead {
			chats[i].ReadBy = append(chats[i].ReadBy, pegawaiID)
			config.DB.Save(&chats[i])
		}
	}

	config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Preload("Pegawai").
		Order("created_at ASC").
		Find(&chats)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "OK",
		"data":    chats,
	})
}

func KirimPenawaranChat(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, ok := getPenawaranPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		Pesan string `json:"pesan"`
	}
	if err := c.Bind(&body); err != nil || body.Pesan == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Pesan tidak boleh kosong."})
	}

	var tracking models.TrackingPenawaran
	if err := config.DB.
		Where("id = ?", trackingID).
		First(&tracking).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Tracking penawaran tidak ditemukan."})
	}

	chat := models.PenawaranChat{
		ID:                  uuid.New().String(),
		TrackingPenawaranID: trackingID,
		PegawaiID:           pegawaiID,
		Pesan:               body.Pesan,
		ReadBy:              []string{pegawaiID},
		CreatedAt:           time.Now(),
	}

	if err := config.DB.Create(&chat).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengirim pesan."})
	}

	if tracking.MarketingID != pegawaiID {
		notif := models.Notifikasi{
			ID:        fmt.Sprintf("NTF-PNWCHT-%s-%d", chat.ID, time.Now().UnixMilli()),
			PegawaiID: tracking.MarketingID,
			Judul:     "Pesan Baru di Penawaran",
			Pesan:     "Ada pesan baru di penawaran #" + tracking.NomorPenawaran,
			CreatedAt: time.Now(),
		}
		config.DB.Create(&notif)
	}

	config.DB.
		Preload("Pegawai").
		Where("id = ?", chat.ID).
		First(&chat)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "Pesan berhasil dikirim.",
		"data":    chat,
	})
}

func UpdatePenawaranChat(c echo.Context) error {
	chatID := c.Param("chatId")

	pegawaiID, ok := getPenawaranPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var chat models.PenawaranChat
	if err := config.DB.
		Where("id = ?", chatID).
		First(&chat).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Chat tidak ditemukan."})
	}

	if chat.PegawaiID != pegawaiID {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Anda tidak berhak mengubah pesan ini."})
	}

	var body struct {
		Pesan string `json:"pesan"`
	}
	if err := c.Bind(&body); err != nil || body.Pesan == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Pesan tidak boleh kosong."})
	}

	chat.Pesan = body.Pesan
	if err := config.DB.Save(&chat).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengubah pesan."})
	}

	config.DB.
		Preload("Pegawai").
		Where("id = ?", chat.ID).
		First(&chat)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Pesan berhasil diubah.",
		"data":    chat,
	})
}

func ReadPenawaranChat(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, ok := getPenawaranPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var chats []models.PenawaranChat
	if err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Find(&chats).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	for i := range chats {
		if chats[i].PegawaiID == pegawaiID {
			continue
		}
		alreadyRead := false
		for _, id := range chats[i].ReadBy {
			if id == pegawaiID {
				alreadyRead = true
				break
			}
		}
		if !alreadyRead {
			chats[i].ReadBy = append(chats[i].ReadBy, pegawaiID)
			config.DB.Save(&chats[i])
		}
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Chat berhasil ditandai sudah dibaca.",
	})
}

func GetUnreadPenawaranChatCount(c echo.Context) error {
	trackingID := c.Param("id")

	pegawaiID, ok := getPenawaranPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var chats []models.PenawaranChat
	config.DB.
		Where("tracking_penawaran_id = ? AND pegawai_id != ?", trackingID, pegawaiID).
		Find(&chats)

	count := 0
	for _, chat := range chats {
		alreadyRead := false
		for _, id := range chat.ReadBy {
			if id == pegawaiID {
				alreadyRead = true
				break
			}
		}
		if !alreadyRead {
			count++
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"unreadCount": count,
	})
}

func GetTotalUnreadPenawaranChatCount(c echo.Context) error {
	pegawaiID, ok := getPenawaranPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var chats []models.PenawaranChat
	config.DB.
		Where("pegawai_id != ?", pegawaiID).
		Find(&chats)

	count := 0
	for _, chat := range chats {
		alreadyRead := false
		for _, id := range chat.ReadBy {
			if id == pegawaiID {
				alreadyRead = true
				break
			}
		}
		if !alreadyRead {
			count++
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"totalUnread": count,
	})
}




// ─── Types ────────────────────────────────────────────────────────────────────

type PenawaranListItem struct {
	ID               string                 `json:"id"`
	NomorPenawaran   string                 `json:"nomorPenawaran"`
	TanggalMasuk     time.Time              `json:"tanggalMasuk"`
	PICReq           *PegawaiSummary        `json:"picReq"`
	PembuatPenawaran *PegawaiSummary        `json:"pembuatPenawaran"`
	EstimasiHarga    *float64               `json:"estimasiHarga,omitempty"`
	StepSaatIni      models.StepPenawaran   `json:"stepSaatIni"`
	Status           models.StatusActivity  `json:"status"`
	PerusahaanName   string                 `json:"perusahaanName,omitempty"`
	LokasiProyek     string                 `json:"lokasiProyek,omitempty"`
	JenisPenawaran   []models.JenisPenawaran `json:"jenisPenawaran,omitempty"`
	TanggalTerbit    *time.Time             `json:"tanggalTerbit,omitempty"`
	TotalTermin      int                    `json:"totalTermin,omitempty"`
	TerminDibayar    int                    `json:"terminDibayar,omitempty"`
}

type PegawaiSummary struct {
	ID   string `json:"id"`
	Nama string `json:"nama"`
}

type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"totalPages"`
}

type PenawaranListResponse struct {
	Data []PenawaranListItem `json:"data"`
	Meta PaginationMeta      `json:"meta"`
}

// ─── Helper ───────────────────────────────────────────────────────────────────

var stepsPengadaan = []models.StepPenawaran{
    models.StepPermintaanMasuk,
    models.StepPenyusunanBoQ,
    models.StepReviewInternal,
    models.StepPersetujuanManajemen, 
	models.StepFollowUp,
}

var stepsAktif = []models.StepPenawaran{
	models.StepImplementasi,
	models.StepBAST,
	models.StepPembayaran,
	models.StepGaransi,
}

// ─── GET /tracking-penawaran ──────────────────────────────────────────────────

func GetTrackingPenawaranList(c echo.Context) error {
	pegawaiID, ok := getPenawaranPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	_, _ = c.Get("isMaster").(bool)

	page := max(1, toInt(c.QueryParam("page"), 1))
	limit := max(1, toInt(c.QueryParam("limit"), 20))
	search := strings.TrimSpace(c.QueryParam("search"))
	filterStep := c.QueryParam("step")
	_ = pegawaiID

	offset := (page - 1) * limit

	query := config.DB.Model(&models.TrackingPenawaran{}).
		Where(`"step_saat_ini" IN ?`, stepsPengadaan)

	if search != "" {
		like := "%" + search + "%"
		query = query.Where(
			`"nomor_penawaran" ILIKE ? OR "customer_name" ILIKE ? OR "lokasi_proyek" ILIKE ?`,
			like, like, like,
		)
	}

	if filterStep != "" {
		query = query.Where(`"step_saat_ini" = ?`, filterStep)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal menghitung data"})
	}

	var rows []models.TrackingPenawaran
	err := query.
		Preload("Marketing").
		Preload("PermintaanMasuk.PreSales").
		Preload("PenyusunanBoQ").
		Preload("PenyusunanBoQ.Pembuat").
		Preload("Perusahaan").
		Preload("FollowUp").
		Preload("Accounting.Items").
		Order(`"created_at" DESC`).
		Limit(limit).
		Offset(offset).
		Find(&rows).Error
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal mengambil data"})
	}

	items := make([]PenawaranListItem, 0, len(rows))
	for _, r := range rows {
		item := PenawaranListItem{
			ID:             r.ID,
			NomorPenawaran: r.NomorPenawaran,
			TanggalMasuk:   r.CreatedAt,
			StepSaatIni:    r.StepSaatIni,
			Status:         r.Status, // dari field baru TrackingPenawaran
			PerusahaanName: r.Perusahaan.Nama,
			LokasiProyek:   r.LokasiProyek,
			JenisPenawaran: r.JenisPenawaran,
		}

		item.PICReq = &PegawaiSummary{
			ID:   r.Marketing.ID,
			Nama: r.Marketing.Nama,
		}

		if r.PermintaanMasuk != nil && r.PermintaanMasuk.PreSales != nil {
			item.PembuatPenawaran = &PegawaiSummary{
				ID:   r.PermintaanMasuk.PreSales.ID,
				Nama: r.PermintaanMasuk.PreSales.Nama,
			}
		}

		if r.PenyusunanBoQ != nil {
			item.EstimasiHarga = r.PenyusunanBoQ.EstimasiHarga
		}

		if r.FollowUp != nil && r.FollowUp.Status == models.StatusSelesai {
			item.TanggalTerbit = &r.FollowUp.UpdatedAt
		}

		if r.Accounting != nil {
			item.TotalTermin = len(r.Accounting.Items)
			paid := 0
			for _, it := range r.Accounting.Items {
				if it.SudahDibayar {
					paid++
				}
			}
			item.TerminDibayar = paid
		}

		items = append(items, item)
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	return c.JSON(http.StatusOK, PenawaranListResponse{
		Data: items,
		Meta: PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func GetTrackingPenawaranAktif(c echo.Context) error {
	pegawaiID, ok := getPenawaranPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	_, _ = c.Get("isMaster").(bool)

	page := max(1, toInt(c.QueryParam("page"), 1))
	limit := max(1, toInt(c.QueryParam("limit"), 20))
	search := strings.TrimSpace(c.QueryParam("search"))
	filterStep := c.QueryParam("step")
	_ = pegawaiID

	offset := (page - 1) * limit

	query := config.DB.Model(&models.TrackingPenawaran{}).
		Where(`"step_saat_ini" IN ?`, stepsAktif)

	if search != "" {
		like := "%" + search + "%"
		query = query.Where(
			`"nomor_penawaran" ILIKE ? OR "customer_name" ILIKE ? OR "lokasi_proyek" ILIKE ?`,
			like, like, like,
		)
	}

	if filterStep != "" {
		query = query.Where(`"step_saat_ini" = ?`, filterStep)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal menghitung data"})
	}

	var rows []models.TrackingPenawaran
	err := query.
		Preload("Marketing").
		Preload("PermintaanMasuk.PreSales").
		Preload("PenyusunanBoQ").
		Preload("PenyusunanBoQ.Pembuat").
		Preload("Perusahaan").
		Preload("FollowUp").
		Preload("Accounting.Items").
		Order(`"created_at" DESC`).
		Limit(limit).
		Offset(offset).
		Find(&rows).Error
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Gagal mengambil data"})
	}

	items := make([]PenawaranListItem, 0, len(rows))
	for _, r := range rows {
		item := PenawaranListItem{
			ID:             r.ID,
			NomorPenawaran: r.NomorPenawaran,
			TanggalMasuk:   r.CreatedAt,
			StepSaatIni:    r.StepSaatIni,
			Status:         r.Status, // dari field baru TrackingPenawaran
			PerusahaanName: r.Perusahaan.Nama,
			LokasiProyek:   r.LokasiProyek,
			JenisPenawaran: r.JenisPenawaran,
		}

		item.PICReq = &PegawaiSummary{
			ID:   r.Marketing.ID,
			Nama: r.Marketing.Nama,
		}

		if r.PermintaanMasuk != nil && r.PermintaanMasuk.PreSales != nil {
			item.PembuatPenawaran = &PegawaiSummary{
				ID:   r.PermintaanMasuk.PreSales.ID,
				Nama: r.PermintaanMasuk.PreSales.Nama,
			}
		}

		if r.PenyusunanBoQ != nil {
			item.EstimasiHarga = r.PenyusunanBoQ.EstimasiHarga
		}

		if r.FollowUp != nil && r.FollowUp.Status == models.StatusSelesai {
			item.TanggalTerbit = &r.FollowUp.UpdatedAt
		}

		if r.Accounting != nil {
			item.TotalTermin = len(r.Accounting.Items)
			paid := 0
			for _, it := range r.Accounting.Items {
				if it.SudahDibayar {
					paid++
				}
			}
			item.TerminDibayar = paid
		}

		items = append(items, item)
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	return c.JSON(http.StatusOK, PenawaranListResponse{
		Data: items,
		Meta: PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func toInt(s string, fallback int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// controllers/pegawai.go
func GetPegawaiByDivisi(c echo.Context) error {
	divisi := c.QueryParam("divisi")

	var pegawai []models.Pegawai
	query := config.DB.Model(&models.Pegawai{})

	if divisi != "" {
		query = query.Where("divisi = ?", divisi)
	}

	if err := query.Order("nama ASC").Find(&pegawai).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Gagal mengambil data pegawai.",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "OK",
		"data":    pegawai,
	})
}


// controllers/penawaran.go — tambah fungsi ini

func UploadPenawaranDokumen(c echo.Context) error {
	permintaanMasukID := c.Param("id")

	// Ambil claims — sama persis dengan pattern controller lain
	claims, ok := c.Get("user").(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ := pegawaiMap["id"].(string)

	// Cek permintaan masuk exist
	var permintaanMasuk models.PermintaanMasuk
	if err := config.DB.First(&permintaanMasuk, "id = ?", permintaanMasukID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"message": "Permintaan masuk tidak ditemukan",
		})
	}

	// Parse multipart form (max 10MB)
	if err := c.Request().ParseMultipartForm(10 << 20); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Gagal parse form: " + err.Error(),
		})
	}

	// Ambil file
	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "File tidak ditemukan: " + err.Error(),
		})
	}

	// Validasi ekstensi
	allowedExt := map[string]bool{
		".pdf": true, ".doc": true, ".docx": true,
		".xls": true, ".xlsx": true,
		".ppt": true, ".pptx": true,
		".txt": true, ".csv": true,
		".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".webp": true, ".svg": true,
		".zip": true, ".rar": true,
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedExt[ext] {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Tipe file tidak diizinkan",
		})
	}

	// Validasi ukuran (max 10MB)
	const maxSize = 10 << 20
	if file.Size > maxSize {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Ukuran file maksimal 10MB",
		})
	}

	// Buat folder uploads
	uploadDir := getUploadDir()
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal membuat folder upload",
		})
	}

	// Generate nama file unik
	uniqueID := uuid.New().String()
	safeOriginal := sanitizeFilename(strings.TrimSuffix(file.Filename, ext))
	newFilename := fmt.Sprintf("%s_%s%s", uniqueID, safeOriginal, ext)
	destPath := filepath.Join(uploadDir, newFilename)

	// Buka dan simpan file
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal membuka file",
		})
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal menyimpan file",
		})
	}
	defer dst.Close()

	buf := make([]byte, 32*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			dst.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}

	filePath := "/uploads/" + newFilename

	// Simpan ke DB
	dokumen := models.PenawaranDokumen{
		ID:                uuid.New().String(),
		NamaFile:          file.Filename,
		Path:              filePath,
		UploadedBy:        pegawaiID,
		PermintaanMasukID: &permintaanMasukID,
		CreatedAt:         time.Now(),
	}
	if err := config.DB.Create(&dokumen).Error; err != nil {
		os.Remove(destPath)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal menyimpan data dokumen",
		})
	}

	// Notifikasi chat penawaran
	var tracking models.TrackingPenawaran
	if err := config.DB.First(&tracking, "id = (SELECT tracking_penawaran_id FROM PermintaanMasuk WHERE id = ?)", permintaanMasukID).Error; err == nil {
		chatNotice := models.PenawaranChat{
			ID:                  uuid.New().String(),
			TrackingPenawaranID: tracking.ID,
			PegawaiID:           pegawaiID,
			Pesan:               "[SYSTEM_NOTIFICATION]:menambahkan dokumen **" + file.Filename + "**",
			ReadBy:              []string{pegawaiID},
			CreatedAt:           time.Now(),
		}
		config.DB.Create(&chatNotice)
	}

	// Preload pegawai untuk response
	config.DB.Preload("Pegawai").First(&dokumen, "id = ?", dokumen.ID)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "File berhasil diunggah",
		"data":    dokumen,
	})
}

func DeletePenawaranDokumen(c echo.Context) error {
	permintaanMasukID := c.Param("id")
	dokumenID := c.Param("dokumenId")

	// Cek permintaan masuk exist
	var permintaanMasuk models.PermintaanMasuk
	if err := config.DB.First(&permintaanMasuk, "id = ?", permintaanMasukID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"message": "Permintaan masuk tidak ditemukan",
		})
	}

	// Cari dokumen
	var dokumen models.PenawaranDokumen
	if err := config.DB.First(&dokumen, "id = ? AND permintaan_masuk_id = ?", dokumenID, permintaanMasukID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"message": "Dokumen tidak ditemukan",
		})
	}

	// Hapus file fisik
	uploadDir := getUploadDir()
	filename := strings.TrimPrefix(dokumen.Path, "/uploads/")
	filePath := filepath.Join(uploadDir, filename)
	os.Remove(filePath)

	// Hapus dari DB
	if err := config.DB.Delete(&dokumen).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal menghapus dokumen",
		})
	}

	// Notifikasi chat
	claims, ok := c.Get("user").(jwt.MapClaims)
	if ok {
		pegawaiMap, ok2 := claims["pegawai"].(map[string]interface{})
		if ok2 {
			pegawaiID, _ := pegawaiMap["id"].(string)
			var tracking models.TrackingPenawaran
			if err := config.DB.First(&tracking, "id = (SELECT tracking_penawaran_id FROM PermintaanMasuk WHERE id = ?)", permintaanMasukID).Error; err == nil {
				chatNotice := models.PenawaranChat{
					ID:                  uuid.New().String(),
					TrackingPenawaranID: tracking.ID,
					PegawaiID:           pegawaiID,
					Pesan:               "[SYSTEM_NOTIFICATION]:menghapus dokumen **" + dokumen.NamaFile + "**",
					ReadBy:              []string{pegawaiID},
					CreatedAt:           time.Now(),
				}
				config.DB.Create(&chatNotice)
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Dokumen berhasil dihapus",
	})
}

func UpdateDetailTrackingPenawaran(c echo.Context) error {
	id := c.Param("id")

	_, ok := getPenawaranPegawaiID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		CustomerName  string `json:"customerName"`
		CustomerPhone string `json:"customerPhone"`
		CustomerEmail string `json:"customerEmail"`
		LokasiProyek  string `json:"lokasiProyek"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	if err := config.DB.Model(&models.TrackingPenawaran{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"customer_name":  body.CustomerName,
			"customer_phone": body.CustomerPhone,
			"customer_email": body.CustomerEmail,
			"lokasi_proyek":  body.LokasiProyek,
		}).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal update detail."})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Detail berhasil diupdate."})
}

func AssignMarketing(c echo.Context) error {
    trackingID := c.Param("id")

    claims, ok := c.Get("user").(jwt.MapClaims)
    if !ok {
        return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
    }
    pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
    pegawaiID, _ := pegawaiMap["id"].(string)
    namaPegawai, _ := pegawaiMap["nama"].(string)

    var body struct {
        MarketingID string `json:"marketingId"`
    }
    if err := c.Bind(&body); err != nil || body.MarketingID == "" {
        return c.JSON(http.StatusBadRequest, map[string]string{"error": "marketingId wajib diisi."})
    }

    var pegawai models.Pegawai
    if err := config.DB.Where("id = ?", body.MarketingID).First(&pegawai).Error; err != nil {
        return c.JSON(http.StatusNotFound, map[string]string{"error": "Pegawai tidak ditemukan."})
    }

    if err := config.DB.Model(&models.TrackingPenawaran{}).
        Where("id = ?", trackingID).
        Update("marketing_id", body.MarketingID).Error; err != nil {
        return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal assign marketing."})
    }

    // Append log ke permintaan masuk
    var permintaanMasuk models.PermintaanMasuk
    if err := config.DB.Where("tracking_penawaran_id = ?", trackingID).First(&permintaanMasuk).Error; err == nil {
        appendLog(&permintaanMasuk, "Assign PIC Request", pegawai.Nama, pegawaiID, namaPegawai)
        config.DB.Save(&permintaanMasuk)
    }

    var tracking models.TrackingPenawaran
    config.DB.
        Where("id = ?", trackingID).
        Preload("Marketing").
        Preload("Perusahaan").
        Preload("PermintaanMasuk.PreSales").
        Preload("PermintaanMasuk.Activity").
        Preload("PermintaanMasuk.Dokumen").
        First(&tracking)

    return c.JSON(http.StatusOK, map[string]interface{}{
        "message": "Marketing berhasil di-assign.",
        "data":    tracking,
    })
}


func appendLog(pm *models.PermintaanMasuk, aksi, keterangan, pegawaiID, namaPegawai string) {
    pm.Logs = append(pm.Logs, models.LogAktivitas{
        Aksi:        aksi,
        Keterangan:  keterangan,
        PegawaiID:   pegawaiID,
        NamaPegawai: namaPegawai,
        CreatedAt:   time.Now(),
    })
}
