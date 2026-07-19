package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mantra/src/config"
	"mantra/src/models"
)

// â”€â”€ Helpers â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

func getImplementasiClaims(c echo.Context) (pegawaiID, namaPegawai, roleStr, divisiStr string, ok bool) {
	claims, valid := c.Get("user").(jwt.MapClaims)
	if !valid {
		return "", "", "", "", false
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	pegawaiID, _ = pegawaiMap["id"].(string)
	namaPegawai, _ = pegawaiMap["nama"].(string)
	roleStr, _ = claims["role"].(string)
	divisiStr, _ = pegawaiMap["divisi"].(string)
	return pegawaiID, namaPegawai, roleStr, divisiStr, true
}

func parseDate(dStr string) *time.Time {
	if dStr == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", dStr)
	if err == nil {
		return &t
	}
	t2, err2 := time.Parse(time.RFC3339, dStr)
	if err2 == nil {
		return &t2
	}
	return nil
}

func formatNumber(val float64) string {
	isInteger := val == float64(int64(val))
	var s string
	if isInteger {
		s = fmt.Sprintf("%.0f", val)
	} else {
		s = fmt.Sprintf("%.2f", val)
	}

	parts := strings.Split(s, ".")
	intPart := parts[0]
	decPart := ""
	if len(parts) > 1 {
		decPart = "," + parts[1]
	}

	var result []string
	length := len(intPart)
	for i, c := range intPart {
		if i > 0 && (length-i)%3 == 0 {
			result = append(result, ".")
		}
		result = append(result, string(c))
	}
	return strings.Join(result, "") + decPart
}

func preloadImplementasi(trackingID string) (*models.Implementasi, error) {
	var impl models.Implementasi
	err := config.DB.
		Where("tracking_penawaran_id = ?", trackingID).
		Preload("TrackingPenawaran.Perusahaan").
		Preload("TrackingPenawaran.Marketing").
		Preload("Barang").
		Preload("Dokumen").
		Preload("Dokumen.Pegawai").
		Preload("ActivityPembelian.Pegawai").
		Preload("ActivityPembelian.Children").
		Preload("ActivityPembelian.Children.Pegawai").
		Preload("ActivityPengantaran.Pegawai").
		Preload("ActivityPengantaran.Children").
		Preload("ActivityPengantaran.Children.Pegawai").
		Preload("ActivityInstalasi.Pegawai").
		Preload("ActivityInstalasi.Children").
		Preload("ActivityInstalasi.Children.Pegawai").
		First(&impl).Error

	if err != nil {
		return nil, err
	}
	return &impl, nil
}

func appendImplementasiLog(impl *models.Implementasi, aksi, keterangan, pegawaiID, namaPegawai string) {
	log := models.LogImplementasi{
		Aksi:        aksi,
		Keterangan:  keterangan,
		PegawaiID:   pegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   time.Now(),
	}
	impl.LogAktivitas = append(impl.LogAktivitas, log)
	config.DB.Save(impl)
}

func getKadivPGA_ID() string {
	var userPgaHead models.User
	errFind := config.DB.Preload("Pegawai").
		Joins("JOIN \"Pegawai\" ON \"Pegawai\".id = \"User\".pegawai_id").
		Where("\"Pegawai\".divisi = ? AND \"User\".role = ?", models.DivisiProcurementGA, models.RoleSupervisi).
		First(&userPgaHead).Error
	if errFind == nil {
		return userPgaHead.PegawaiID
	}
	return ""
}

// ──────────────────────────────────────────────────────────────────────────────────────────────────

func GetDetailImplementasi(c echo.Context) error {
	trackingID := c.Param("id")
	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	impl, err := preloadImplementasi(trackingID)
	if err != nil {
		var tracking models.TrackingPenawaran
		if errDb := config.DB.Preload("Perusahaan").First(&tracking, "id = ?", trackingID).Error; errDb == nil {
			newImpl := models.Implementasi{
				ID:                  uuid.New().String(),
				TrackingPenawaranID: trackingID,
				Status:              models.StatusOnProgress,
				LogAktivitas: []models.LogImplementasi{
					{
						Aksi:        "Implementasi Dimulai",
						Keterangan:  "Inisialisasi otomatis proses implementasi.",
						PegawaiID:   pegawaiID,
						NamaPegawai: namaPegawai,
						CreatedAt:   time.Now(),
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			if errCreate := config.DB.Create(&newImpl).Error; errCreate == nil {
				impl, _ = preloadImplementasi(trackingID)
				return c.JSON(http.StatusOK, impl)
			}
		}
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Implementasi tidak ditemukan."})
	}

	return c.JSON(http.StatusOK, impl)
}

// ──────────────────────────────────────────────────────────────────────────────────────────────────

func UpdateDetailImplementasi(c echo.Context) error {
	trackingID := c.Param("id")
	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	var body struct {
		NoPO            string `json:"noPO"`
		TanggalPO       string `json:"tanggalPO"`
		NoWO            string `json:"noWO"`
		TanggalWO       string `json:"tanggalWO"`
		NoDO            string `json:"noDO"`
		TanggalDO       string `json:"tanggalDO"`
		WaktuPengerjaan string `json:"waktuPengerjaan"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	impl, err := preloadImplementasi(trackingID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Implementasi tidak ditemukan."})
	}

	oldNoPO := impl.NoPO
	oldNoWO := impl.NoWO
	oldNoDO := impl.NoDO

	impl.NoPO = body.NoPO
	impl.TanggalPO = parseDate(body.TanggalPO)
	impl.NoWO = body.NoWO
	impl.TanggalWO = parseDate(body.TanggalWO)
	impl.NoDO = body.NoDO
	impl.TanggalDO = parseDate(body.TanggalDO)
	impl.WaktuPengerjaan = parseDate(body.WaktuPengerjaan)
	impl.UpdatedAt = time.Now()

	if impl.NoPO != "" && impl.ActivityPembelianID == nil {
		pgaHeadID := getKadivPGA_ID()
		if pgaHeadID != "" {
			actID := uuid.New().String()
			act := models.Activity{
				ID:            actID,
				PegawaiID:     pgaHeadID,
				TerkaitPO:     &impl.TrackingPenawaran.NomorPenawaran,
				Perusahaan:    &impl.TrackingPenawaran.Perusahaan.Nama,
				Kategori:      models.KategoriBillOfQuantity,
				Judul:         "Pembelian Barang Proyek - " + impl.TrackingPenawaran.Perusahaan.Nama,
				WaktuMulai:    time.Now(),
				Status:        models.StatusOnProgress,
			}
			if err := config.DB.Create(&act).Error; err == nil {
				impl.ActivityPembelianID = &actID
			}
		}
	}

	if impl.NoWO != "" && impl.ActivityPengantaranID == nil {
		pgaHeadID := getKadivPGA_ID()
		if pgaHeadID != "" {
			actID := uuid.New().String()
			act := models.Activity{
				ID:            actID,
				PegawaiID:     pgaHeadID,
				TerkaitPO:     &impl.TrackingPenawaran.NomorPenawaran,
				Perusahaan:    &impl.TrackingPenawaran.Perusahaan.Nama,
				Kategori:      models.KategoriAkomodasiProject,
				Judul:         "Pengantaran Barang Proyek - " + impl.TrackingPenawaran.Perusahaan.Nama,
				WaktuMulai:    time.Now(),
				Status:        models.StatusOnProgress,
			}
			if err := config.DB.Create(&act).Error; err == nil {
				impl.ActivityPengantaranID = &actID
			}
		}
	}

	if impl.NoDO != "" && impl.ActivityInstalasiID == nil {
		pgaHeadID := getKadivPGA_ID()
		if pgaHeadID != "" {
			actID := uuid.New().String()
			act := models.Activity{
				ID:            actID,
				PegawaiID:     pgaHeadID,
				TerkaitPO:     &impl.TrackingPenawaran.NomorPenawaran,
				Perusahaan:    &impl.TrackingPenawaran.Perusahaan.Nama,
				Kategori:      models.KategoriMonitorProgress,
				Judul:         "Instalasi dan Uji Coba Proyek - " + impl.TrackingPenawaran.Perusahaan.Nama,
				WaktuMulai:    time.Now(),
				Status:        models.StatusOnProgress,
			}
			if err := config.DB.Create(&act).Error; err == nil {
				impl.ActivityInstalasiID = &actID
			}
		}
	}

	if err := config.DB.Save(impl).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengupdate detail implementasi."})
	}

	if body.NoPO != "" {
		config.DB.Model(&models.TrackingPenawaran{}).Where("id = ?", trackingID).Update("nomor_po", body.NoPO)
	}

	var changes []string
	if oldNoPO != body.NoPO {
		changes = append(changes, fmt.Sprintf("No. Purchase Order diubah dari '%s' menjadi '%s'", oldNoPO, body.NoPO))
	}
	if oldNoWO != body.NoWO {
		changes = append(changes, fmt.Sprintf("No. Work Order diubah dari '%s' menjadi '%s'", oldNoWO, body.NoWO))
	}
	if oldNoDO != body.NoDO {
		changes = append(changes, fmt.Sprintf("No. Delivery Order diubah dari '%s' menjadi '%s'", oldNoDO, body.NoDO))
	}

	appendImplementasiLog(impl, "Update Info Order", strings.Join(changes, ", "), pegawaiID, namaPegawai)

	updated, _ := preloadImplementasi(trackingID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Detail implementasi berhasil diperbarui.",
		"data":    updated,
	})
}

// ──────────────────────────────────────────────────────────────────────────────────────────────────

func AddBarangImplementasi(c echo.Context) error {
	trackingID := c.Param("id")
	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	impl, err := preloadImplementasi(trackingID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Implementasi tidak ditemukan."})
	}

	var body struct {
		NamaBarang         string  `json:"namaBarang"`
		Status             string  `json:"status"`
		Qty                float64 `json:"qty"`
		Satuan             string  `json:"satuan"`
		HargaSatuan        float64 `json:"hargaSatuan"`
		Metode             string  `json:"metode"`
		EstimasiKedatangan string  `json:"estimasiKedatangan"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	barang := models.ImplementasiBarang{
		ID:                 uuid.New().String(),
		ImplementasiID:     impl.ID,
		NamaBarang:         body.NamaBarang,
		Status:             body.Status,
		Qty:                body.Qty,
		Satuan:             body.Satuan,
		HargaSatuan:        body.HargaSatuan,
		Metode:             body.Metode,
		EstimasiKedatangan: parseDate(body.EstimasiKedatangan),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := config.DB.Create(&barang).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menambahkan barang."})
	}

	appendImplementasiLog(impl, "Tambah Barang", fmt.Sprintf("Menambahkan barang baru dengan nama '%s' (Status: %s, Qty: %s %s, Harga: Rp %s, Metode: %s)", body.NamaBarang, body.Status, formatNumber(body.Qty), body.Satuan, formatNumber(body.HargaSatuan), body.Metode), pegawaiID, namaPegawai)

	updated, _ := preloadImplementasi(trackingID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Barang berhasil ditambahkan.",
		"data":    updated,
	})
}

// ──────────────────────────────────────────────────────────────────────────────────────────────────

func UpdateBarangImplementasi(c echo.Context) error {
	trackingID := c.Param("id")
	barangID := c.Param("barangId")
	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	impl, err := preloadImplementasi(trackingID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Implementasi tidak ditemukan."})
	}

	var barang models.ImplementasiBarang
	if err := config.DB.Where("id = ? AND implementasi_id = ?", barangID, impl.ID).First(&barang).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Barang tidak ditemukan."})
	}

	var body struct {
		NamaBarang         string  `json:"namaBarang"`
		Status             string  `json:"status"`
		Qty                float64 `json:"qty"`
		Satuan             string  `json:"satuan"`
		HargaSatuan        float64 `json:"hargaSatuan"`
		Metode             string  `json:"metode"`
		EstimasiKedatangan string  `json:"estimasiKedatangan"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body."})
	}

	var changes []string
	if barang.NamaBarang != body.NamaBarang {
		changes = append(changes, fmt.Sprintf("nama barang dari '%s' menjadi '%s'", barang.NamaBarang, body.NamaBarang))
	}
	if barang.Status != body.Status {
		changes = append(changes, fmt.Sprintf("status dari '%s' menjadi '%s'", barang.Status, body.Status))
	}
	if barang.Qty != body.Qty {
		changes = append(changes, fmt.Sprintf("qty dari '%s' menjadi '%s'", formatNumber(barang.Qty), formatNumber(body.Qty)))
	}
	if barang.Satuan != body.Satuan {
		changes = append(changes, fmt.Sprintf("satuan dari '%s' menjadi '%s'", barang.Satuan, body.Satuan))
	}
	if barang.HargaSatuan != body.HargaSatuan {
		changes = append(changes, fmt.Sprintf("harga satuan dari 'Rp %s' menjadi 'Rp %s'", formatNumber(barang.HargaSatuan), formatNumber(body.HargaSatuan)))
	}
	if barang.Metode != body.Metode {
		changes = append(changes, fmt.Sprintf("metode dari '%s' menjadi '%s'", barang.Metode, body.Metode))
	}
	newEst := parseDate(body.EstimasiKedatangan)
	if (barang.EstimasiKedatangan == nil && newEst != nil) || (barang.EstimasiKedatangan != nil && newEst == nil) || (barang.EstimasiKedatangan != nil && newEst != nil && !barang.EstimasiKedatangan.Equal(*newEst)) {
		oldEstStr := "kosong"
		if barang.EstimasiKedatangan != nil {
			oldEstStr = barang.EstimasiKedatangan.Format("2006-01-02")
		}
		newEstStr := "kosong"
		if newEst != nil {
			newEstStr = newEst.Format("2006-01-02")
		}
		changes = append(changes, fmt.Sprintf("estimasi kedatangan dari '%s' menjadi '%s'", oldEstStr, newEstStr))
	}

	barang.NamaBarang = body.NamaBarang
	barang.Status = body.Status
	barang.Qty = body.Qty
	barang.Satuan = body.Satuan
	barang.HargaSatuan = body.HargaSatuan
	barang.Metode = body.Metode
	barang.EstimasiKedatangan = newEst
	barang.UpdatedAt = time.Now()

	if err := config.DB.Save(&barang).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal memperbarui barang."})
	}

	keterangan := fmt.Sprintf("Memperbarui info barang '%s'", barang.NamaBarang)
	if len(changes) > 0 {
		keterangan = fmt.Sprintf("Memperbarui barang '%s' (%s)", barang.NamaBarang, strings.Join(changes, ", "))
	}
	appendImplementasiLog(impl, "Update Barang", keterangan, pegawaiID, namaPegawai)

	updated, _ := preloadImplementasi(trackingID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Barang berhasil diperbarui.",
		"data":    updated,
	})
}

// ──────────────────────────────────────────────────────────────────────────────────────────────────

func DeleteBarangImplementasi(c echo.Context) error {
	trackingID := c.Param("id")
	barangID := c.Param("barangId")
	pegawaiID, namaPegawai, _, _, ok := getImplementasiClaims(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	impl, err := preloadImplementasi(trackingID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Implementasi tidak ditemukan."})
	}

	var barang models.ImplementasiBarang
	if err := config.DB.Where("id = ? AND implementasi_id = ?", barangID, impl.ID).First(&barang).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Barang tidak ditemukan."})
	}

	namaBarang := barang.NamaBarang
	if err := config.DB.Delete(&barang).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menghapus barang."})
	}

	appendImplementasiLog(impl, "Hapus Barang", fmt.Sprintf("Menghapus barang '%s'", namaBarang), pegawaiID, namaPegawai)

	updated, _ := preloadImplementasi(trackingID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Barang berhasil dihapus.",
		"data":    updated,
	})
}

// ──────────────────────────────────────────────────────────────────────────────────────────────────

func AssignPGAStaff(c echo.Context) error {
	trackingID := c.Param("id")
	pegawaiID, namaPegawai, roleStr, _, ok := getImplementasiClaims(c)
	if !ok || roleStr != string(models.RoleSupervisi) {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized."})
	}

	impl, err := preloadImplementasi(trackingID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Data Implementasi tidak ditemukan."})
	}

	var req struct {
		StaffIDs []string `json:"staffIds"`
		Phase    string   `json:"phase"` // "pembelian", "pengantaran", "instalasi"
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid body"})
	}

	var parentID *string
	var activityName string
	var activityJudul string
	var parentActivity *models.Activity
	var deskripsi string

	switch req.Phase {
	case "pengantaran":
		parentID = impl.ActivityPengantaranID
		activityName = "Pengantaran"
		parentActivity = impl.ActivityPengantaran
		activityJudul = "Pengantaran Barang Proyek - " + impl.TrackingPenawaran.Perusahaan.Nama
		deskripsi = "Pengantaran Barang ke " + impl.TrackingPenawaran.Perusahaan.Nama
	case "instalasi":
		parentID = impl.ActivityInstalasiID
		activityName = "Instalasi"
		parentActivity = impl.ActivityInstalasi
		activityJudul = "Instalasi Proyek - " + impl.TrackingPenawaran.Perusahaan.Nama
		deskripsi = "Melakukan instalasi di lokasi proyek " + impl.TrackingPenawaran.Perusahaan.Nama
	default:
		parentID = impl.ActivityPembelianID
		activityName = "Pembelian"
		parentActivity = impl.ActivityPembelian
		activityJudul = "Pengecekan Barang Proyek - " + impl.TrackingPenawaran.Perusahaan.Nama
		deskripsi = "Pengecekan barang untuk penawaran #" + impl.TrackingPenawaran.NomorPenawaran
		if parentActivity != nil && parentActivity.Deskripsi != "" {
			deskripsi = parentActivity.Deskripsi
		}

		var items []string
		for _, b := range impl.Barang {
			items = append(items, fmt.Sprintf("%s (%d %s)", b.NamaBarang, int(b.Qty), b.Satuan))
		}
		if len(items) > 0 {
			deskripsi += "\n\nDaftar Barang:\n- " + strings.Join(items, "\n- ")
		}
	}

	if parentID == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Activity %s belum dibuat.", activityName)})
	}

	var targetSelesai time.Time
	if parentActivity != nil {
		targetSelesai = parentActivity.TargetSelesai
	}

	var assignedNames []string
	for _, staffID := range req.StaffIDs {
		var count int64
		config.DB.Model(&models.Activity{}).Where("parent_id = ? AND pegawai_id = ?", *parentID, staffID).Count(&count)
		if count > 0 {
			continue
		}

		var staff models.Pegawai
		if err := config.DB.First(&staff, "id = ?", staffID).Error; err != nil {
			continue
		}

		childActivityID := uuid.New().String()
		childActivity := models.Activity{
			ID:            childActivityID,
			PegawaiID:     staffID,
			ParentID:      parentID,
			TerkaitPO:     &impl.TrackingPenawaran.NomorPenawaran,
			Perusahaan:    &impl.TrackingPenawaran.Perusahaan.Nama,
			Kategori:      models.KategoriBillOfQuantity,
			Judul:         activityJudul,
			Deskripsi:     deskripsi,
			WaktuMulai:    time.Now(),
			TargetSelesai: targetSelesai,
			Status:        models.StatusOnProgress,
		}

		if err := config.DB.Create(&childActivity).Error; err == nil {
			assignedNames = append(assignedNames, staff.Nama)
		} else {
			fmt.Println("Error creating child activity:", err)
		}
	}

	if len(assignedNames) > 0 {
		keterangan := fmt.Sprintf("Menugaskan staff PGA untuk %s: %s", activityName, strings.Join(assignedNames, ", "))
		appendImplementasiLog(impl, "Assign Staff PGA", keterangan, pegawaiID, namaPegawai)
	} else {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal menugaskan staff PGA (mungkin sudah ditugaskan sebelumnya)."})
	}

	updated, _ := preloadImplementasi(trackingID)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Staff PGA berhasil ditugaskan.",
		"data":    updated,
	})
}
