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

	// ── Activity BastEntry (Admin Proyek) DITERIMA → entry pertama auto-buat Garansi (paralel dgn BAST) ──
	if err := handleBastEntryDiterima(tx, a); err != nil {
		fmt.Println(">>> Error handleBastEntryDiterima:", err)
		return err
	}

	// ── Activity kunjungan Garansi bulan berjalan DITERIMA → lanjut bulan berikutnya ──
	if err := handleGaransiMonthDiterima(tx, a); err != nil {
		fmt.Println(">>> Error handleGaransiMonthDiterima:", err)
		return err
	}

	// ── Activity penagihan Termin DITERIMA → lanjut termin berikutnya ──────
	if err := handleItemTerminDiterima(tx, a); err != nil {
		fmt.Println(">>> Error handleItemTerminDiterima:", err)
		return err
	}

	// ── Rantai berurutan Stage 4: Admin Proyek → Finance → Admin Sekertaris
	//    (minta TTD Direktur) → Stage 5 (nunggu konfirmasi Direktur) ────────
	if err := handlePengecekanAdminProyekDiterima(tx, a); err != nil {
		fmt.Println(">>> Error handlePengecekanAdminProyekDiterima:", err)
		return err
	}
	if err := handlePengecekanFinanceDiterima(tx, a); err != nil {
		fmt.Println(">>> Error handlePengecekanFinanceDiterima:", err)
		return err
	}
	if err := handleMintaTTDDirekturDiterima(tx, a); err != nil {
		fmt.Println(">>> Error handleMintaTTDDirekturDiterima:", err)
		return err
	}

	return nil
}

// followUpNomorPO ngambil nomor PO/penawaran punya tracking punya FollowUp
// ini — dipakai di ketiga handler rantai pengecekan dokumen PO di bawah.
func followUpNomorPO(tx *gorm.DB, followUp *FollowUp) (string, error) {
	var tracking TrackingPenawaran
	if err := tx.Where("id = ?", followUp.TrackingPenawaranID).First(&tracking).Error; err != nil {
		return "", err
	}
	nomorPO := tracking.NomorPenawaran
	if tracking.NomorPO != nil && *tracking.NomorPO != "" {
		nomorPO = *tracking.NomorPO
	}
	return nomorPO, nil
}

// ─── 1. Pengecekan Admin Proyek DITERIMA → buat daily pengecekan Finance ─────
// Rantai berurutan FollowUp Stage 4: Admin Proyek -> Finance -> Admin
// Sekertaris (minta TTD Direktur) -> Stage 5. Sebelumnya 2 daily
// (Admin Proyek & Finance) dibuat BARENGAN/paralel; sekarang berurutan satu-satu.

func handlePengecekanAdminProyekDiterima(tx *gorm.DB, a *Activity) error {
	var followUp FollowUp
	if err := tx.Where("activity_pengecekan_admin_proyek_id = ?", a.ID).First(&followUp).Error; err != nil {
		// Bukan activity pengecekan Admin Proyek, skip diam-diam.
		return nil
	}

	// Guard idempotency: kalau daily Finance udah pernah dibuat, jangan dobel.
	if followUp.ActivityPengecekanFinanceID != nil && *followUp.ActivityPengecekanFinanceID != "" {
		fmt.Println(">>> Daily pengecekan Finance udah ada, skip:", *followUp.ActivityPengecekanFinanceID)
		return nil
	}

	fmt.Println(">>> Pengecekan Admin Proyek selesai, buat daily pengecekan Finance:", followUp.ID)

	financeSupervisor, err := FindSupervisiFinanceAccounting(tx)
	if err != nil {
		fmt.Println(">>> Gagal cari Supervisi Finance Accounting:", err)
		return err
	}

	namaPegawai := ""
	var pegawai Pegawai
	if err := tx.Where("id = ?", a.PegawaiID).First(&pegawai).Error; err == nil {
		namaPegawai = pegawai.Nama
	}

	nomorPO, err := followUpNomorPO(tx, &followUp)
	if err != nil {
		fmt.Println(">>> Gagal ambil nomor PO:", err)
		return err
	}

	now := time.Now()
	activityFinance := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     financeSupervisor.ID,
		TerkaitPO:     &nomorPO,
		Kategori:      KategoriDokumenPendukung,
		Judul:         "Pengecekan Dokumen PO (Finance)",
		Deskripsi:     "Cek kelengkapan PO customer & kesiapan data terkait penawaran " + nomorPO + " sebelum lanjut ke proses dokumen PO internal.",
		WaktuMulai:    now,
		TargetSelesai: now.Add(24 * time.Hour),
		Status:        StatusOnProgress,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := tx.Create(&activityFinance).Error; err != nil {
		fmt.Println(">>> Gagal membuat daily pengecekan Finance:", err)
		return err
	}

	followUp.LogAktivitas = append(followUp.LogAktivitas, LogFollowUp{
		Aksi:        "Pengecekan Admin Proyek Selesai",
		Keterangan:  "Daily pengecekan Admin Proyek selesai. Lanjut daily pengecekan Finance (" + financeSupervisor.Nama + ").",
		PegawaiID:   a.PegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   now,
	})

	return tx.Model(&FollowUp{}).Where("id = ?", followUp.ID).
		Select("activity_pengecekan_finance_id", "log_aktivitas").
		Updates(FollowUp{
			ActivityPengecekanFinanceID: &activityFinance.ID,
			LogAktivitas:                followUp.LogAktivitas,
		}).Error
}

// ─── 2. Pengecekan Finance DITERIMA → buat daily Admin Sekertaris (minta TTD) ─

func handlePengecekanFinanceDiterima(tx *gorm.DB, a *Activity) error {
	var followUp FollowUp
	if err := tx.Where("activity_pengecekan_finance_id = ?", a.ID).First(&followUp).Error; err != nil {
		// Bukan activity pengecekan Finance, skip diam-diam.
		return nil
	}

	// Guard idempotency: kalau daily minta TTD Direktur udah pernah dibuat, jangan dobel.
	if followUp.ActivityMintaTTDDirekturID != nil && *followUp.ActivityMintaTTDDirekturID != "" {
		fmt.Println(">>> Daily minta TTD Direktur udah ada, skip:", *followUp.ActivityMintaTTDDirekturID)
		return nil
	}

	fmt.Println(">>> Pengecekan Finance selesai, buat daily Admin Sekertaris minta TTD Direktur:", followUp.ID)

	var adminSekertaris Pegawai
	if err := tx.Where("divisi = ?", DivisiAdminSekertaris).First(&adminSekertaris).Error; err != nil {
		fmt.Println(">>> Gagal cari Admin Sekertaris:", err)
		return fmt.Errorf("admin Sekertaris belum ada/belum di-assign — hubungi admin buat set divisi Admin Sekertaris")
	}

	namaPegawai := ""
	var pegawai Pegawai
	if err := tx.Where("id = ?", a.PegawaiID).First(&pegawai).Error; err == nil {
		namaPegawai = pegawai.Nama
	}

	nomorPO, err := followUpNomorPO(tx, &followUp)
	if err != nil {
		fmt.Println(">>> Gagal ambil nomor PO:", err)
		return err
	}

	now := time.Now()
	activityTTD := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     adminSekertaris.ID,
		TerkaitPO:     &nomorPO,
		Kategori:      KategoriDokumenPendukung,
		Judul:         "Minta TTD Direktur - Dokumen PO",
		Deskripsi:     "Meminta tanda tangan Direktur untuk dokumen PO terkait penawaran " + nomorPO,
		WaktuMulai:    now,
		TargetSelesai: now.Add(24 * time.Hour),
		Status:        StatusOnProgress,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := tx.Create(&activityTTD).Error; err != nil {
		fmt.Println(">>> Gagal membuat daily minta TTD Direktur:", err)
		return err
	}

	followUp.LogAktivitas = append(followUp.LogAktivitas, LogFollowUp{
		Aksi:        "Pengecekan Finance Selesai",
		Keterangan:  "Daily pengecekan Finance selesai. Lanjut daily Admin Sekertaris (" + adminSekertaris.Nama + ") minta TTD Direktur.",
		PegawaiID:   a.PegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   now,
	})

	return tx.Model(&FollowUp{}).Where("id = ?", followUp.ID).
		Select("activity_minta_ttd_direktur_id", "log_aktivitas").
		Updates(FollowUp{
			ActivityMintaTTDDirekturID: &activityTTD.ID,
			LogAktivitas:               followUp.LogAktivitas,
		}).Error
}

// ─── 3. Admin Sekertaris minta TTD Direktur DITERIMA → Stage 5 ──────────────
// Ini penutup rantai berurutan Stage 4 -- begitu daily ke-3 ini DITERIMA,
// FollowUp maju ke Stage 5 (nunggu konfirmasi Direktur/Komisaris, lihat
// KonfirmasiDokumenPO). Guard Stage==4 nyegah re-trigger kalau daily ini
// di-update lagi setelah Stage udah maju.

func handleMintaTTDDirekturDiterima(tx *gorm.DB, a *Activity) error {
	var followUp FollowUp
	err := tx.Where("activity_minta_ttd_direktur_id = ? AND stage = ?", a.ID, 4).First(&followUp).Error
	if err != nil {
		// Bukan activity minta TTD Direktur yang lagi di Stage 4, skip diam-diam.
		return nil
	}

	fmt.Println(">>> Admin Sekertaris selesai minta TTD Direktur, FollowUp lanjut Stage 5:", followUp.ID)

	namaPegawai := ""
	var pegawai Pegawai
	if err := tx.Where("id = ?", a.PegawaiID).First(&pegawai).Error; err == nil {
		namaPegawai = pegawai.Nama
	}

	followUp.LogAktivitas = append(followUp.LogAktivitas, LogFollowUp{
		Aksi:        "Minta TTD Direktur Selesai",
		Keterangan:  "Admin Sekertaris selesai meminta TTD Direktur atas dokumen PO. Menunggu konfirmasi Direktur/Komisaris.",
		PegawaiID:   a.PegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   time.Now(),
	})

	return tx.Model(&FollowUp{}).Where("id = ?", followUp.ID).
		Select("stage", "log_aktivitas").
		Updates(FollowUp{Stage: 5, LogAktivitas: followUp.LogAktivitas}).Error
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

	// wasPerluTindakan: daily ini abis di-reschedule ulang (ON_PROGRESS ->
	// DITERIMA lagi) setelah Supervisi Sales nolak -- kalau iya, WAJIB
	// diproses ulang biarpun AccAdminDirektur/AccManajerOps masih true dari
	// sebelumnya (guard idempotency di bawah gak boleh nge-skip ini).
	wasPerluTindakan := review.Status == StatusPerluTindakan

	// Guard idempotency: jangan re-ACC otomatis kalau udah pernah kejadian
	// (dan bukan kasus revisi setelah ditolak).
	if review.AccAdminDirektur && review.AccManajerOps && !wasPerluTindakan {
		fmt.Println(">>> Review Internal udah pernah ke-ACC otomatis, skip:", review.ID)
		return nil
	}

	fmt.Println(">>> Daily Pengecekan Penawaran selesai, ACC Admin Sekertaris & Manager Ops otomatis")

	// ACC Admin Sekertaris & Manager Ops otomatis begitu daily-nya selesai.
	// TAPI gak langsung Selesai -- masih nunggu 1 gate manual lagi: approval
	// Supervisi Sales (AccSupervisiSales), mirip AccDirekturKomisaris di
	// Persetujuan Manajemen. Lihat TryFinalizeReviewInternal.
	review.AccAdminDirektur = true
	review.AccManajerOps = true
	if wasPerluTindakan {
		// Daily udah direvisi & disetujui lagi -> Review Internal siap
		// dikonfirmasi Supervisi Sales lagi, gak perlu proses manual
		// "konfirmasi ulang" terpisah.
		review.Status = StatusOnProgress
		appendReviewInternalLogDirect(&review, "ACC Otomatis (Revisi)", "Admin Sekertaris & Manager Ops otomatis ACC lagi setelah daily Pengecekan Penawaran direvisi & disetujui ulang. Menunggu approval Supervisi Sales lagi.", a.PegawaiID, namaPegawai)
		tx.Model(&TrackingPenawaran{}).Where("id = ?", review.TrackingPenawaranID).Update("status", StatusOnProgress)
	} else {
		appendReviewInternalLogDirect(&review, "ACC Otomatis", "Admin Sekertaris & Manager Ops otomatis ACC setelah daily Pengecekan Penawaran selesai. Menunggu approval Supervisi Sales.", a.PegawaiID, namaPegawai)
	}
	tx.Save(&review)

	return TryFinalizeReviewInternal(tx, &review, a.PegawaiID, namaPegawai)
}

// TryFinalizeReviewInternal ngecek 3 syarat approval Review Internal (Admin
// Sekertaris, Manager Ops -- dua-duanya auto-set begitu daily Pengecekan
// Penawaran selesai -- dan Supervisi Sales -- WAJIB dipencet manual lewat
// endpoint ACC). Begitu ketiganya lengkap, baru Status jadi SELESAI dan lanjut
// bikin Persetujuan Manajemen. Dipanggil dari 2 titik: hook otomatis (pas
// daily selesai) dan endpoint manual ACC (pas Supervisi Sales approve) --
// gak peduli urutan mana yang duluan lengkap.
func TryFinalizeReviewInternal(tx *gorm.DB, review *ReviewInternal, pegawaiID, namaPegawai string) error {
	if !(review.AccAdminDirektur && review.AccManajerOps && review.AccSupervisiSales) {
		fmt.Println(">>> Review Internal belum lengkap approval-nya, skip finalize:", review.ID)
		return nil
	}
	if review.Status == StatusSelesai {
		fmt.Println(">>> Review Internal udah SELESAI, skip finalize dobel:", review.ID)
		return nil
	}

	review.Status = StatusSelesai
	appendReviewInternalLogDirect(review, "Review Internal Selesai", "Semua approval lengkap (Admin Sekertaris, Manager Ops, Supervisi Sales), lanjut ke Persetujuan Manajemen", pegawaiID, namaPegawai)
	if err := tx.Save(review).Error; err != nil {
		return err
	}

	// Update TrackingPenawaran ke step berikutnya
	if err := tx.Model(&TrackingPenawaran{}).
		Where("id = ?", review.TrackingPenawaranID).
		Updates(map[string]interface{}{
			"step_saat_ini": StepPersetujuanManajemen,
			"status":        StatusOnProgress,
		}).Error; err != nil {
		return err
	}

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
		if err := tx.Create(&persetujuan).Error; err != nil {
			return err
		}
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

	// Hold pengantaran jika kondisi SESUDAH_DP dan termin 1 belum lunas.
	// Pengantaran baru dibuat otomatis setelah termin 1 ditandai lunas
	// (lihat resume di handleItemTerminDiterima / BayarItemTermin).
	if followUp.KondisiPengantaran != nil && *followUp.KondisiPengantaran == "SESUDAH_DP" {
		var termin TerminPembayaran
		if errTermin := tx.
			Where("tracking_penawaran_id = ?", impl.TrackingPenawaranID).
			First(&termin).Error; errTermin == nil {
			var itemTermin1 ItemTermin
			if errItem := tx.
				Where("termin_pembayaran_id = ? AND index = ?", termin.ID, 1).
				First(&itemTermin1).Error; errItem == nil {
				if !itemTermin1.SudahDibayar {
					fmt.Println(">>> Pengantaran di-HOLD: kondisi SESUDAH_DP, termin 1 belum lunas. Menunggu pembayaran DP.")
					return nil
				}
			}
		}
	}

	// Ambil nama pegawai yang meng-update (buat log)
	namaPegawai := ""
	var pegawai Pegawai
	if err := tx.Where("id = ?", a.PegawaiID).First(&pegawai).Error; err == nil {
		namaPegawai = pegawai.Nama
	}

	// Pengantaran sekarang PIC-nya Supervisi PGA -- sama kayak Pembelian
	// (activity a ini), bukan Admin Proyek lagi. Dulu di sini dicari Admin
	// Proyek buat jadi PIC-nya; sekarang tinggal dilanjutkan aja ke pegawai
	// yang sama yang ngerjain Pembelian (a.PegawaiID).
	now := time.Now()
	pengantaranActivity := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     a.PegawaiID,
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
		Keterangan:  fmt.Sprintf("Activity pengantaran otomatis dibuat untuk %s (Supervisi PGA), deadline 2 hari", namaPegawai),
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

	// Instalasi tetap PIC-nya Admin Proyek (bukan Supervisi PGA kayak
	// Pembelian/Pengantaran di atas) -- makanya dicari lagi ke FollowUp, gak
	// bisa asal pakai a.PegawaiID (yang sekarang Supervisi PGA).
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
	if err := tx.Where("id = ?", *followUp.ActivityAdminProyekID).First(&adminProyekActivity).Error; err != nil {
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
	instalasiActivity := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     adminProyekActivity.PegawaiID,
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
// BAST dipecah berdasarkan JenisPenawaran tracking-nya: kalau ada "PAC Montair"
// DAN "Generator FirePro" dua-duanya → 2 Bast terpisah (PAC + FIRE), masing-
// masing jumlah entry-nya dari FollowUp.TotalBastPAC/TotalBastFire. Kalau
// cuma salah satu atau gak ada dua-duanya → 1 Bast (kategori itu, atau UMUM),
// jumlah entry dari field yang sesuai. Maksimal 2 Bast, gak pernah lebih.
func handleInstalasiBarangDiterima(tx *gorm.DB, a *Activity) error {
	// Cek apakah activity ini adalah "activity instalasi" milik sebuah Implementasi.
	var impl Implementasi
	if err := tx.Where("activity_instalasi_id = ?", a.ID).First(&impl).Error; err != nil {
		// Bukan activity instalasi barang implementasi, skip diam-diam.
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

	var tracking TrackingPenawaran
	if err := tx.Where("id = ?", impl.TrackingPenawaranID).First(&tracking).Error; err != nil {
		fmt.Println(">>> TrackingPenawaran tidak ditemukan, skip:", err)
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

	kategoriList := DetectBastKategori(tracking.JenisPenawaran)

	var perusahaan Perusahaan
	kodePerusahaan := "XXX"
	if err := tx.Where("id = ?", tracking.PerusahaanID).First(&perusahaan).Error; err == nil {
		kodePerusahaan = KodePerusahaanFromNama(perusahaan.Nama)
	}

	anyBastDibuat := false
	// PAC & FIRE dua-duanya ada -> butuh 2 input terpisah (TotalBastPAC/Fire).
	// Kalau cuma salah satu (atau gak ada dua-duanya) -> tetap pakai field
	// generik TotalBAST, sama kayak sebelum ada pemisahan BAST — biar gak
	// maksa isi 2 kotak input padahal cuma butuh 1 BAST.
	dualKategori := len(kategoriList) == 2

	for _, kategori := range kategoriList {
		// Guard idempotency per kategori: kalau Bast kategori ini buat tracking
		// ini udah pernah dibuat, jangan dobel.
		var existingBast Bast
		if tx.Where("tracking_penawaran_id = ? AND kategori = ?", impl.TrackingPenawaranID, kategori).First(&existingBast).Error == nil {
			fmt.Println(">>> BAST kategori", kategori, "sudah ada, skip:", existingBast.ID)
			continue
		}

		var jumlahBastPtr *int
		switch {
		case !dualKategori:
			jumlahBastPtr = followUp.TotalBAST
		case kategori == KategoriBastPAC:
			jumlahBastPtr = followUp.TotalBastPAC
		case kategori == KategoriBastFire:
			jumlahBastPtr = followUp.TotalBastFire
		}

		if jumlahBastPtr == nil || *jumlahBastPtr <= 0 {
			fmt.Println(">>> Total BAST kategori", kategori, "belum diisi, skip pembuatan BAST")
			continue
		}
		jumlahBast := *jumlahBastPtr

		now := time.Now()

		bast := Bast{
			ID:                  uuid.New().String(),
			TrackingPenawaranID: impl.TrackingPenawaranID,
			Kategori:            kategori,
			Status:              StatusOnProgress,
			LogAktivitas: []LogBast{
				{
					Aksi:        "Buat BAST",
					Keterangan:  fmt.Sprintf("BAST %s otomatis dibuat untuk %s setelah instalasi barang diterima (%d entry)", kategori, adminProyekActivity.Pegawai.Nama, jumlahBast),
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

		fmt.Println(">>> BAST dibuat:", bast.ID, "kategori:", kategori, "untuk tracking:", impl.TrackingPenawaranID)
		anyBastDibuat = true

		// Semua slot entry dibuat sekaligus (biar keliatan totalnya dari
		// awal), TAPI daily-nya cuma dibuat buat entry pertama — entry ke-2
		// dst baru dapet daily setelah entry sebelumnya DITERIMA
		// (AdvanceBastEntryIfReady). PAC & FIRE jalan paralel satu sama
		// lain, tapi masing-masing tetep urut sendiri-sendiri.
		//
		// Index TETAP lokal per Bast (1, 2, 3, ... entry ke berapa dalam
		// Bast INI) -- BUKAN global. Yang global itu NoReferensi (nomor
		// referensi/kode BAST yang ditampilkan, lihat GenerateBastKode).
		// Index dipakai buat nentuin urutan internal (entry berikutnya di
		// AdvanceBastEntryIfReady) DAN buat nandain "entry pertama BAST ini"
		// yang men-trigger pembuatan Garansi (handleBastEntryDiterima, cek
		// entry.Index == 1) -- kalau Index dibikin global, entry pertama
		// project ke-2/3/dst gak akan pernah index==1 lagi, dan Garansi gak
		// akan pernah ke-trigger buat project itu.
		for i := 1; i <= jumlahBast; i++ {
			noReferensi := GenerateBastKode(tx, kategori, kodePerusahaan, now.Year(), int(now.Month()))

			entry := BastEntry{
				ID:          uuid.New().String(),
				BastID:      bast.ID,
				Index:       i,
				NoReferensi: noReferensi,
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			if err := tx.Create(&entry).Error; err != nil {
				fmt.Println(">>> Gagal membuat BastEntry:", err)
				return err
			}

			fmt.Println(">>> BastEntry dibuat:", entry.ID, "kode:", noReferensi, "index:", i)

			if i == 1 {
				if err := CreateBastEntryActivity(tx, &entry, adminProyekActivity.PegawaiID, kategori); err != nil {
					return err
				}
			}
		}
	}

	if !anyBastDibuat {
		fmt.Println(">>> Gak ada BAST yang dibuat (total BAST belum diisi semua), skip update step")
		return nil
	}

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

// ─── Activity BastEntry (Admin Proyek) DITERIMA ───────────────────────────────
// (1) Kalau SEMUA entry BAST udah DITERIMA → tandai BAST SELESAI.
// (2) Kalau entry PERTAMA (paling awal dibuat) udah DITERIMA → auto-buat
//     Garansi (status BELUM_DIKONFIGURASI), TANPA nunggu entry lain selesai.
//     BAST & Garansi memang didesain bisa berjalan paralel/parsial — entry
//     BAST lain boleh masih on progress selagi Garansi udah mulai jalan.

func handleBastEntryDiterima(tx *gorm.DB, a *Activity) error {
	// Cek apakah activity ini adalah "activity admin proyek" milik sebuah BastEntry.
	var entry BastEntry
	if err := tx.Where("activity_admin_proyek_id = ?", a.ID).First(&entry).Error; err != nil {
		// Bukan activity BastEntry, skip diam-diam.
		return nil
	}

	var bast Bast
	if err := tx.Where("id = ?", entry.BastID).First(&bast).Error; err != nil {
		fmt.Println(">>> BAST tidak ditemukan untuk entry:", entry.ID)
		return nil
	}

	namaPegawai := ""
	var pegawai Pegawai
	if err := tx.Where("id = ?", a.PegawaiID).First(&pegawai).Error; err == nil {
		namaPegawai = pegawai.Nama
	}

	now := time.Now()

	// ── (1) Tandai BAST SELESAI kalau semua entry-nya udah DITERIMA ──────────
	if bast.Status != StatusSelesai {
		var totalEntries, doneEntries int64
		tx.Model(&BastEntry{}).Where("bast_id = ?", bast.ID).Count(&totalEntries)
		tx.Model(&BastEntry{}).
			Joins(`JOIN "Activity" ON "Activity".id = "BastEntry".activity_admin_proyek_id`).
			Where(`"BastEntry".bast_id = ? AND "Activity".status = ?`, bast.ID, StatusDiterima).
			Count(&doneEntries)

		if totalEntries > 0 && doneEntries >= totalEntries {
			fmt.Println(">>> Semua entry BAST DITERIMA, BAST SELESAI:", bast.ID)
			bast.Status = StatusSelesai
			bast.LogAktivitas = append(bast.LogAktivitas, LogBast{
				Aksi:        "BAST Selesai",
				Keterangan:  "Semua entry BAST telah selesai diserahterimakan",
				PegawaiID:   a.PegawaiID,
				NamaPegawai: namaPegawai,
				CreatedAt:   now,
			})

			if err := tx.Model(&Bast{}).Where("id = ?", bast.ID).
				Select("status", "log_aktivitas").
				Updates(Bast{Status: bast.Status, LogAktivitas: bast.LogAktivitas}).Error; err != nil {
				fmt.Println(">>> Gagal update status BAST jadi SELESAI:", err)
				return err
			}
		} else {
			fmt.Println(">>> BAST belum semua entry selesai:", doneEntries, "/", totalEntries)
		}
	}

	var followUp FollowUp
	if err := tx.Where("tracking_penawaran_id = ?", bast.TrackingPenawaranID).First(&followUp).Error; err != nil {
		fmt.Println(">>> FollowUp tidak ditemukan, skip:", err)
		return nil
	}
	if followUp.ActivityAdminProyekID == nil || *followUp.ActivityAdminProyekID == "" {
		fmt.Println(">>> FollowUp belum punya Admin Proyek, skip")
		return nil
	}
	var adminProyekActivity Activity
	if err := tx.Where("id = ?", *followUp.ActivityAdminProyekID).First(&adminProyekActivity).Error; err != nil {
		fmt.Println(">>> Activity Admin Proyek tidak ditemukan, skip:", err)
		return nil
	}
	picID := adminProyekActivity.PegawaiID

	// ── (2) Entry ini DITERIMA → buatin daily entry berikutnya (kalau ada) ───
	// Berlaku buat SEMUA entry, bukan cuma entry pertama — BAST jalan
	// berurutan satu-satu, bukan sekaligus. PAC & FIRE (2 Bast berbeda)
	// tetep jalan paralel karena ini di-scope per Bast (per kategori).
	if err := AdvanceBastEntryIfReady(tx, &entry, picID, bast.Kategori); err != nil {
		fmt.Println(">>> Error AdvanceBastEntryIfReady:", err)
		return err
	}

	// ── (3) Entry pertama (Index==1) BAST DITERIMA → auto-buat Garansi
	// (berjalan paralel dengan entry BAST lainnya, gak perlu nunggu BAST tuntas) ──
	if entry.Index != 1 {
		fmt.Println(">>> Bukan entry pertama BAST, skip pembuatan Garansi")
		return nil
	}

	// Guard idempotency Garansi — di-scope per (tracking, kategori) karena
	// satu tracking bisa punya sampai 2 Garansi (PAC & FIRE), masing-masing
	// dipicu Bast kategorinya sendiri-sendiri.
	var existingGaransi Garansi
	if tx.Where("tracking_penawaran_id = ? AND kategori_bast = ?", bast.TrackingPenawaranID, bast.Kategori).First(&existingGaransi).Error == nil {
		fmt.Println(">>> Garansi kategori", bast.Kategori, "sudah ada, skip:", existingGaransi.ID)
		return nil
	}

	garansi := Garansi{
		ID:                  uuid.New().String(),
		TrackingPenawaranID: bast.TrackingPenawaranID,
		KategoriBast:        bast.Kategori,
		BastID:              bast.ID,
		PICID:               picID,
		Status:              StatusGaransiBelumDikonfigurasi,
		LogAktivitas: []LogGaransi{
			{
				Aksi:        "Buat Garansi",
				Keterangan:  fmt.Sprintf("Garansi %s otomatis dibuat setelah entry pertama BAST selesai (berjalan paralel dengan entry BAST lainnya), menunggu konfigurasi tahun & bulan mulai", bast.Kategori),
				PegawaiID:   a.PegawaiID,
				NamaPegawai: namaPegawai,
				CreatedAt:   now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := tx.Create(&garansi).Error; err != nil {
		fmt.Println(">>> Gagal membuat Garansi:", err)
		return err
	}

	fmt.Println(">>> Garansi dibuat:", garansi.ID, "untuk tracking:", bast.TrackingPenawaranID, "PIC:", picID)

	if err := tx.Model(&TrackingPenawaran{}).
		Where("id = ?", bast.TrackingPenawaranID).
		Updates(map[string]interface{}{
			"step_saat_ini": StepGaransi,
			"status":        StatusOnProgress,
		}).Error; err != nil {
		fmt.Println(">>> Gagal update TrackingPenawaran step Garansi:", err)
		return err
	}

	return nil
}

// ─── Activity kunjungan Garansi bulan berjalan DITERIMA → tandai bulan itu
// selesai, lalu (kalau tanggal kunjungan udah terisi) buat daily bulan berikutnya ──

func handleGaransiMonthDiterima(tx *gorm.DB, a *Activity) error {
	var month GaransiMonth
	if err := tx.Where("activity_id = ?", a.ID).First(&month).Error; err != nil {
		// Bukan activity kunjungan Garansi, skip diam-diam.
		return nil
	}

	// Guard idempotency: kalau bulan ini udah pernah ditandai selesai, jangan diproses ulang.
	if month.ActivitySelesai {
		fmt.Println(">>> GaransiMonth sudah ditandai selesai, skip:", month.ID)
		return nil
	}

	namaPegawai := ""
	var pegawai Pegawai
	if err := tx.Where("id = ?", a.PegawaiID).First(&pegawai).Error; err == nil {
		namaPegawai = pegawai.Nama
	}

	now := time.Now()
	month.ActivitySelesai = true
	month.LogAktivitas = append(month.LogAktivitas, LogGaransi{
		Aksi:        "Kunjungan Selesai",
		Keterangan:  fmt.Sprintf("Daily kunjungan garansi bulan ke-%d disetujui selesai", month.BulanKe),
		PegawaiID:   a.PegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   now,
	})

	if month.TanggalKunjungan != nil {
		month.Status = StatusDiterima
	}

	if err := tx.Model(&GaransiMonth{}).Where("id = ?", month.ID).
		Select("activity_selesai", "status", "log_aktivitas").
		Updates(GaransiMonth{
			ActivitySelesai: month.ActivitySelesai,
			Status:          month.Status,
			LogAktivitas:    month.LogAktivitas,
		}).Error; err != nil {
		fmt.Println(">>> Gagal update GaransiMonth setelah DITERIMA:", err)
		return err
	}

	return AdvanceGaransiIfReady(tx, &month, a.PegawaiID, namaPegawai)
}

// ─── Activity penagihan Termin DITERIMA → tandai termin itu ActivitySelesai,
// lalu (kalau udah SudahDibayar juga) buat daily termin berikutnya ──────────

func handleItemTerminDiterima(tx *gorm.DB, a *Activity) error {
	var item ItemTermin
	if err := tx.Where("activity_id = ?", a.ID).First(&item).Error; err != nil {
		// Bukan activity penagihan termin, skip diam-diam.
		return nil
	}

	// Guard idempotency: kalau termin ini udah pernah ditandai selesai, jangan diproses ulang.
	if item.ActivitySelesai {
		fmt.Println(">>> ItemTermin sudah ditandai selesai, skip:", item.ID)
		return nil
	}

	item.ActivitySelesai = true
	if err := tx.Model(&ItemTermin{}).Where("id = ?", item.ID).
		Update("activity_selesai", true).Error; err != nil {
		fmt.Println(">>> Gagal update ItemTermin.ActivitySelesai:", err)
		return err
	}

	return AdvanceTerminIfReady(tx, &item)
}

// ─── Resume Pengantaran Hold ───────────────────────────────────────────────
// Dipanggil saat termin 1 lunas (SudahDibayar) dan kondisi SESUDAH_DP.
// Membuat ActivityPengantaran yang sebelumnya di-hold karena pembayaran DP
// belum diterima. Logic pembuatan activity sama dengan handlePembelianBarangDiterima
// -- PIC-nya Supervisi PGA (dari Activity Pembelian), bukan Admin Proyek.

func ResumePengantaranHold(tx *gorm.DB, impl *Implementasi, followUp *FollowUp) error {
	if impl.ActivityPembelianID == nil || *impl.ActivityPembelianID == "" {
		fmt.Println(">>> ResumePengantaranHold: Activity Pembelian belum ada, skip")
		return nil
	}

	var pembelianActivity Activity
	if err := tx.Preload("Pegawai").Where("id = ?", *impl.ActivityPembelianID).First(&pembelianActivity).Error; err != nil {
		fmt.Println(">>> ResumePengantaranHold: Activity Pembelian tidak ditemukan, skip:", err)
		return nil
	}

	now := time.Now()
	pengantaranActivity := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     pembelianActivity.PegawaiID,
		Kategori:      KategoriAkomodasiProject,
		Judul:         "Pengantaran Barang Implementasi",
		Deskripsi:     "Activity pengantaran barang (dilanjutkan setelah termin 1 DP lunas)",
		WaktuMulai:    now,
		TargetSelesai: now.Add(48 * time.Hour),
		Status:        StatusOnProgress,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := tx.Create(&pengantaranActivity).Error; err != nil {
		fmt.Println(">>> ResumePengantaranHold: Gagal membuat Activity Pengantaran:", err)
		return err
	}

	fmt.Println(">>> ResumePengantaranHold: Activity Pengantaran dibuat:", pengantaranActivity.ID)

	impl.LogAktivitas = append(impl.LogAktivitas, LogImplementasi{
		Aksi:        "Buat Activity Pengantaran (Resume)",
		Keterangan:  fmt.Sprintf("Activity pengantaran dilanjutkan setelah termin 1 DP lunas, deadline 2 hari, PIC: %s (Supervisi PGA)", pembelianActivity.Pegawai.Nama),
		PegawaiID:   pembelianActivity.PegawaiID,
		NamaPegawai: pembelianActivity.Pegawai.Nama,
		CreatedAt:   now,
	})

	if err := tx.Model(&Implementasi{}).
		Where("id = ?", impl.ID).
		Select("activity_pengantaran_id", "log_aktivitas").
		Updates(Implementasi{
			ActivityPengantaranID: &pengantaranActivity.ID,
			LogAktivitas:          impl.LogAktivitas,
		}).Error; err != nil {
		fmt.Println(">>> ResumePengantaranHold: Gagal update Implementasi.ActivityPengantaranID:", err)
		return err
	}

	return nil
}