package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (a *Activity) AfterUpdate(tx *gorm.DB) error {
	fmt.Println(">>> AfterUpdate Activity triggered, status:", a.Status, "kategori:", a.Kategori)

	if a.Status != StatusDiterima {
		fmt.Println(">>> Bukan DITERIMA, skip")
		return nil
	}

	// ── Existing: Quotation → Review Internal auto-selesai ──────────────────
	if a.Kategori == KategoriQuotation {
		if err := handleQuotationDiterima(tx, a); err != nil {
			fmt.Println(">>> Error handleQuotationDiterima:", err)
			return err
		}
	}

	// ── Activity Pembelian Barang Implementasi → auto-buat Activity Pengantaran ──
	if err := handlePembelianBarangDiterima(tx, a); err != nil {
		fmt.Println(">>> Error handlePembelianBarangDiterima:", err)
		return err
	}

	// ── Activity Pengantaran Barang Implementasi → auto-buat Activity Instalasi ──
	if err := handlePengantaranBarangDiterima(tx, a); err != nil {
		fmt.Println(">>> Error handlePengantaranBarangDiterima:", err)
		return err
	}

	// ── Activity Instalasi Barang Implementasi → auto-buat BAST + Activity Admin Proyek ──
	if err := handleInstalasiBarangDiterima(tx, a); err != nil {
		fmt.Println(">>> Error handleInstalasiBarangDiterima:", err)
		return err
	}

	return nil
}

// ─── Quotation → Review Internal (existing logic, dipisah biar rapi) ──────────

func handleQuotationDiterima(tx *gorm.DB, a *Activity) error {
	if a.Kategori != KategoriQuotation {
		return nil
	}

	fmt.Println(">>> Mencari Review Internal dengan activity_admin_id:", a.ID)

	var review ReviewInternal
	if err := tx.Where("activity_admin_id = ?", a.ID).First(&review).Error; err != nil {
		fmt.Println(">>> Review Internal tidak ditemukan:", err)
		return nil
	}

	// Ambil nama pegawai
	namaPegawai := ""
	var pegawai Pegawai
	if err := tx.Where("id = ?", a.PegawaiID).First(&pegawai).Error; err == nil {
		namaPegawai = pegawai.Nama
	}

	fmt.Println(">>> Daily selesai, Review Internal langsung selesai")

	// Review Internal selesai
	review.AccAdminDirektur = true
	review.AccManajerOps = true
	review.Status = StatusSelesai
	appendReviewInternalLogDirect(&review, "Review Internal Selesai", "Otomatis selesai setelah daily Pengecekan Penawaran selesai", a.PegawaiID, namaPegawai)
	tx.Save(&review)

	// Update TrackingPenawaran ke step berikutnya
	tx.Model(&TrackingPenawaran{}).
		Where("id = ?", review.TrackingPenawaranID).
		Updates(map[string]interface{}{
			"step_saat_ini": StepPersetujuanManajemen,
			"status":        StatusOnProgress,
		})

	// Buat Persetujuan Manajemen
	var existing PersetujuanManajemen
	if tx.Where("tracking_penawaran_id = ?", review.TrackingPenawaranID).First(&existing).Error != nil {
		persetujuan := PersetujuanManajemen{
			ID:                   uuid.New().String(),
			TrackingPenawaranID:  review.TrackingPenawaranID,
			AccDirekturKomisaris: false,
			Status:               StatusOnProgress,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		tx.Create(&persetujuan)
	}

	return nil
}

func appendReviewInternalLogDirect(review *ReviewInternal, aksi, keterangan, pegawaiID, namaPegawai string) {
	log := LogReviewInternal{
		Aksi:        aksi,
		Keterangan:  keterangan,
		PegawaiID:   pegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   time.Now(),
	}
	review.LogAktivitas = append(review.LogAktivitas, log)
}

// ─── Activity Pembelian Barang DITERIMA → auto-buat Activity Pengantaran ──────

func handlePembelianBarangDiterima(tx *gorm.DB, a *Activity) error {
	// Cek apakah activity ini adalah "activity pembelian" milik sebuah Implementasi.
	// Dicek lewat FK, bukan Kategori, karena Kategori (AKOMODASI_PROJECT) bisa
	// dipakai activity lain juga — FK activity_pembelian_id lebih presisi.
	var impl Implementasi
	if err := tx.Where("activity_pembelian_id = ?", a.ID).First(&impl).Error; err != nil {
		// Bukan activity pembelian barang implementasi, skip diam-diam.
		return nil
	}

	// Guard idempotency: kalau activity pengantaran udah pernah dibuat, jangan dobel.
	if impl.ActivityPengantaranID != nil && *impl.ActivityPengantaranID != "" {
		fmt.Println(">>> Activity Pengantaran sudah ada, skip:", *impl.ActivityPengantaranID)
		return nil
	}

	fmt.Println(">>> Activity Pembelian Barang DITERIMA, mencari FollowUp untuk tracking:", impl.TrackingPenawaranID)

	var followUp FollowUp
	if err := tx.Where("tracking_penawaran_id = ?", impl.TrackingPenawaranID).First(&followUp).Error; err != nil {
		fmt.Println(">>> FollowUp tidak ditemukan, skip:", err)
		return nil
	}

	if followUp.ActivityAdminProyekID == nil || *followUp.ActivityAdminProyekID == "" {
		fmt.Println(">>> FollowUp belum punya Admin Proyek, skip")
		return nil
	}

	var adminProyekActivity Activity
	if err := tx.Preload("Pegawai").Where("id = ?", *followUp.ActivityAdminProyekID).First(&adminProyekActivity).Error; err != nil {
		fmt.Println(">>> Activity Admin Proyek tidak ditemukan, skip:", err)
		return nil
	}

	// Ambil nama pegawai yang meng-update (buat log)
	namaPegawai := ""
	var pegawai Pegawai
	if err := tx.Where("id = ?", a.PegawaiID).First(&pegawai).Error; err == nil {
		namaPegawai = pegawai.Nama
	}

	now := time.Now()
	pengantaranActivity := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     adminProyekActivity.PegawaiID,
		Kategori:      KategoriAkomodasiProject,
		Judul:         "Pengantaran Barang Implementasi",
		Deskripsi:     "Activity otomatis pengantaran barang untuk tahap Implementasi setelah pembelian barang diterima",
		WaktuMulai:    now,
		TargetSelesai: now.Add(48 * time.Hour),
		Status:        StatusOnProgress,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := tx.Create(&pengantaranActivity).Error; err != nil {
		fmt.Println(">>> Gagal membuat Activity Pengantaran:", err)
		return err
	}

	fmt.Println(">>> Activity Pengantaran dibuat:", pengantaranActivity.ID, "untuk pegawai:", pengantaranActivity.PegawaiID)

	// Update Implementasi.ActivityPengantaranID + log — pakai Model().Update()
	// (bukan Save(&impl)) biar gak nge-upsert ulang association (mis. impl.Barang).
	impl.LogAktivitas = append(impl.LogAktivitas, LogImplementasi{
		Aksi:        "Buat Activity Pengantaran",
		Keterangan:  fmt.Sprintf("Activity pengantaran otomatis dibuat untuk %s, deadline 2 hari", adminProyekActivity.Pegawai.Nama),
		PegawaiID:   a.PegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   now,
	})

	if err := tx.Model(&Implementasi{}).
		Where("id = ?", impl.ID).
		Select("activity_pengantaran_id", "log_aktivitas").
		Updates(Implementasi{
			ActivityPengantaranID: &pengantaranActivity.ID,
			LogAktivitas:          impl.LogAktivitas,
		}).Error; err != nil {
		fmt.Println(">>> Gagal update Implementasi.ActivityPengantaranID:", err)
		return err
	}

	return nil
}

// ─── Activity Pengantaran Barang DITERIMA → auto-buat Activity Instalasi ──────

func handlePengantaranBarangDiterima(tx *gorm.DB, a *Activity) error {
	// Cek apakah activity ini adalah "activity pengantaran" milik sebuah Implementasi.
	var impl Implementasi
	if err := tx.Where("activity_pengantaran_id = ?", a.ID).First(&impl).Error; err != nil {
		// Bukan activity pengantaran barang implementasi, skip diam-diam.
		return nil
	}

	// Guard idempotency: kalau activity instalasi udah pernah dibuat, jangan dobel.
	if impl.ActivityInstalasiID != nil && *impl.ActivityInstalasiID != "" {
		fmt.Println(">>> Activity Instalasi sudah ada, skip:", *impl.ActivityInstalasiID)
		return nil
	}

	fmt.Println(">>> Activity Pengantaran Barang DITERIMA, buat Activity Instalasi untuk tracking:", impl.TrackingPenawaranID)

	// Ambil nama pegawai yang meng-update (buat log)
	namaPegawai := ""
	var pegawai Pegawai
	if err := tx.Where("id = ?", a.PegawaiID).First(&pegawai).Error; err == nil {
		namaPegawai = pegawai.Nama
	}

	now := time.Now()
	instalasiActivity := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     a.PegawaiID,
		Kategori:      KategoriAkomodasiProject,
		Judul:         "Instalasi Barang Implementasi",
		Deskripsi:     "Activity otomatis instalasi barang untuk tahap Implementasi setelah pengantaran barang diterima",
		WaktuMulai:    now,
		TargetSelesai: now.Add(48 * time.Hour),
		Status:        StatusOnProgress,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := tx.Create(&instalasiActivity).Error; err != nil {
		fmt.Println(">>> Gagal membuat Activity Instalasi:", err)
		return err
	}

	fmt.Println(">>> Activity Instalasi dibuat:", instalasiActivity.ID, "untuk pegawai:", instalasiActivity.PegawaiID)

	// Update Implementasi.ActivityInstalasiID + log — pakai Model().Update()
	// (bukan Save(&impl)) biar gak nge-upsert ulang association (mis. impl.Barang).
	impl.LogAktivitas = append(impl.LogAktivitas, LogImplementasi{
		Aksi:        "Buat Activity Instalasi",
		Keterangan:  "Activity instalasi otomatis dibuat, deadline 2 hari",
		PegawaiID:   a.PegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   now,
	})

	if err := tx.Model(&Implementasi{}).
		Where("id = ?", impl.ID).
		Select("activity_instalasi_id", "log_aktivitas").
		Updates(Implementasi{
			ActivityInstalasiID: &instalasiActivity.ID,
			LogAktivitas:        impl.LogAktivitas,
		}).Error; err != nil {
		fmt.Println(">>> Gagal update Implementasi.ActivityInstalasiID:", err)
		return err
	}

	return nil
}

// ─── Activity Instalasi Barang DITERIMA → auto-buat BAST + Activity Admin Proyek ──

func handleInstalasiBarangDiterima(tx *gorm.DB, a *Activity) error {
	// Cek apakah activity ini adalah "activity instalasi" milik sebuah Implementasi.
	var impl Implementasi
	if err := tx.Where("activity_instalasi_id = ?", a.ID).First(&impl).Error; err != nil {
		// Bukan activity instalasi barang implementasi, skip diam-diam.
		return nil
	}

	// Guard idempotency: kalau BAST buat tracking ini udah pernah dibuat, jangan dobel.
	var existingBast Bast
	if tx.Where("tracking_penawaran_id = ?", impl.TrackingPenawaranID).First(&existingBast).Error == nil {
		fmt.Println(">>> BAST sudah ada, skip:", existingBast.ID)
		return nil
	}

	fmt.Println(">>> Activity Instalasi Barang DITERIMA, mencari FollowUp untuk tracking:", impl.TrackingPenawaranID)

	var followUp FollowUp
	if err := tx.Where("tracking_penawaran_id = ?", impl.TrackingPenawaranID).First(&followUp).Error; err != nil {
		fmt.Println(">>> FollowUp tidak ditemukan, skip:", err)
		return nil
	}

	if followUp.ActivityAdminProyekID == nil || *followUp.ActivityAdminProyekID == "" {
		fmt.Println(">>> FollowUp belum punya Admin Proyek, skip")
		return nil
	}

	var adminProyekActivity Activity
	if err := tx.Preload("Pegawai").Where("id = ?", *followUp.ActivityAdminProyekID).First(&adminProyekActivity).Error; err != nil {
		fmt.Println(">>> Activity Admin Proyek tidak ditemukan, skip:", err)
		return nil
	}

	// Ambil nama pegawai yang meng-update (buat log)
	namaPegawai := ""
	var pegawai Pegawai
	if err := tx.Where("id = ?", a.PegawaiID).First(&pegawai).Error; err == nil {
		namaPegawai = pegawai.Nama
	}

	now := time.Now()
	bastActivity := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     adminProyekActivity.PegawaiID,
		Kategori:      KategoriAkomodasiProject,
		Judul:         "Pembuatan BAST",
		Deskripsi:     "Activity otomatis pembuatan BAST setelah instalasi barang diterima",
		WaktuMulai:    now,
		TargetSelesai: now.Add(48 * time.Hour),
		Status:        StatusOnProgress,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := tx.Create(&bastActivity).Error; err != nil {
		fmt.Println(">>> Gagal membuat Activity Admin Proyek untuk BAST:", err)
		return err
	}

	fmt.Println(">>> Activity BAST dibuat:", bastActivity.ID, "untuk pegawai:", bastActivity.PegawaiID)

	bast := Bast{
		ID:                    uuid.New().String(),
		TrackingPenawaranID:   impl.TrackingPenawaranID,
		Status:                StatusOnProgress,
		ActivityAdminProyekID: &bastActivity.ID,
		LogAktivitas: []LogBast{
			{
				Aksi:        "Buat BAST",
				Keterangan:  fmt.Sprintf("BAST otomatis dibuat untuk %s setelah instalasi barang diterima", adminProyekActivity.Pegawai.Nama),
				PegawaiID:   a.PegawaiID,
				NamaPegawai: namaPegawai,
				CreatedAt:   now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := tx.Create(&bast).Error; err != nil {
		fmt.Println(">>> Gagal membuat BAST:", err)
		return err
	}

	fmt.Println(">>> BAST dibuat:", bast.ID, "untuk tracking:", impl.TrackingPenawaranID)

	// Update TrackingPenawaran ke step BAST
	if err := tx.Model(&TrackingPenawaran{}).
		Where("id = ?", impl.TrackingPenawaranID).
		Updates(map[string]interface{}{
			"step_saat_ini": StepBAST,
			"status":        StatusOnProgress,
		}).Error; err != nil {
		fmt.Println(">>> Gagal update TrackingPenawaran step BAST:", err)
		return err
	}

	return nil
}