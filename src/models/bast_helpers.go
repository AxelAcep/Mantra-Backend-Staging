package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DetectBastKategori nentuin BAST/garansi kategori apa aja yang kebentuk
// dari Jenis Penawaran tracking — 3 grup (sinkron sama frontend
// utils/bast-kategori.ts):
//   PAC    : PAC Montair, Chiller, AC Split/Standing  (garansi 12/2)
//   FIRE   : Generator FirePro, Conventional Sys, Addressable Sys,
//            Stand Alone/BTA                          (garansi firepro)
//   BATTERY: Battery, UPS                             (garansi 4/2)
// Bisa return 1–3 kategori. Kalau gak ada satupun yang kedetect -> [UMUM].
func DetectBastKategori(jenisPenawaran []JenisPenawaran) []KategoriBast {
	hasPAC := false
	hasFire := false
	hasBattery := false
	for _, j := range jenisPenawaran {
		norm := strings.ToLower(strings.TrimSpace(string(j)))
		norm = strings.ReplaceAll(norm, "_", " ")
		norm = strings.ReplaceAll(norm, "/", " ")
		switch {
		case strings.Contains(norm, "pac montair"),
			strings.Contains(norm, "chiller"),
			strings.Contains(norm, "ac split"):
			hasPAC = true
		case strings.Contains(norm, "generator"),
			strings.Contains(norm, "fire"),
			strings.Contains(norm, "conventional"),
			strings.Contains(norm, "addressable"),
			strings.Contains(norm, "stand alone"),
			strings.Contains(norm, "bta"):
			hasFire = true
		case strings.Contains(norm, "battery"),
			strings.Contains(norm, "ups"):
			hasBattery = true
		}
	}

	var kategori []KategoriBast
	if hasPAC {
		kategori = append(kategori, KategoriBastPAC)
	}
	if hasFire {
		kategori = append(kategori, KategoriBastFire)
	}
	if hasBattery {
		kategori = append(kategori, KategoriBastBattery)
	}
	if len(kategori) == 0 {
		kategori = append(kategori, KategoriBastUmum)
	}
	return kategori
}

// KodePerusahaanFromNama ngambil 3 huruf pertama nama perusahaan (spasi
// dibuang dulu), uppercase — dipakai buat auto-generate kode BAST.
// Misal "PT Cahaya Abadi" -> "PTC".
func KodePerusahaanFromNama(nama string) string {
	cleaned := strings.ToUpper(strings.ReplaceAll(nama, " ", ""))
	for len(cleaned) < 3 {
		cleaned += "X"
	}
	runes := []rune(cleaned)
	return string(runes[:3])
}

// GenerateBastKode bikin kode/No. Referensi BAST otomatis, format:
//   UMUM   : BAST-YYMM-XXX-###
//   PAC    : BAST-PAC-YYMM-XXX-###
//   FIRE   : BAST-FIR-YYMM-XXX-###
//   BATTERY: BAST-BAT-YYMM-XXX-###
// Nomor urut (###, 3 digit) GLOBAL beneran — dibagi bareng antar kategori
// (PAC/FIRE/BATTERY/UMUM), antar bulan/tahun, DAN antar penawaran/project. Gak
// pernah reset lagi, pindah ke project lain pun nomornya lanjut terus.
func GenerateBastKode(tx *gorm.DB, kategori KategoriBast, kodePerusahaan string, tahun, bulan int) string {
	prefix := "BAST"
	switch kategori {
	case KategoriBastPAC:
		prefix = "BAST-PAC"
	case KategoriBastFire:
		prefix = "BAST-FIR"
	case KategoriBastBattery:
		prefix = "BAST-BAT"
	}

	// Nomor urut (###) di belakang sekarang GLOBAL beneran -- dibagi bareng
	// antar kategori (PAC/FIRE/UMUM), antar bulan/tahun, DAN antar
	// perusahaan/project. Jadi biarpun ganti ke penawaran/project lain,
	// nomornya tetep lanjut, gak pernah reset ke 001 lagi. Bagian
	// bulan+tahun+kode perusahaan di kode/no. referensi cuma buat label,
	// gak lagi nentuin scope hitungan.
	var count int64
	tx.Model(&BastEntry{}).Count(&count)

	base := fmt.Sprintf("%s-%02d%02d-%s", prefix, tahun%100, bulan, kodePerusahaan)
	return fmt.Sprintf("%s-%03d", base, count+1)
}

// CreateBastEntryActivity bikin daily Activity (Pembuatan BAST) buat satu
// BastEntry dan nyimpen ActivityAdminProyekID-nya. Dipanggil buat entry
// pertama tiap Bast (langsung pas Bast dibuat) maupun otomatis dari
// AdvanceBastEntryIfReady (entry ke-2 dst, satu-satu berurutan).
func CreateBastEntryActivity(tx *gorm.DB, entry *BastEntry, picID string, kategori KategoriBast) error {
	now := time.Now()

	judul := fmt.Sprintf("Pembuatan BAST #%d", entry.Index)
	if kategori != KategoriBastUmum {
		judul = fmt.Sprintf("Pembuatan BAST %s #%d", kategori, entry.Index)
	}

	activity := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     picID,
		Kategori:      KategoriAkomodasiProject,
		Judul:         judul,
		Deskripsi:     "Activity otomatis pembuatan BAST setelah instalasi barang diterima",
		WaktuMulai:    now,
		TargetSelesai: now.Add(48 * time.Hour),
		Status:        StatusOnProgress,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// TerkaitPO = Nomor Penawaran — resolve lewat Bast-nya supaya tampilan
	// "Terkait" di detail daily activity konsisten + link /penawaran/{id}
	// bisa dibuka langsung.
	var bast Bast
	trackingID := ""
	if err := tx.First(&bast, "id = ?", entry.BastID).Error; err == nil {
		trackingID = bast.TrackingPenawaranID
	}
	activity.TerkaitPO = nomorPenawaranTracking(tx, trackingID)

	if err := tx.Create(&activity).Error; err != nil {
		fmt.Println(">>> Gagal membuat Activity BastEntry:", err)
		return err
	}

	// Notifikasi Tahap 7 (target_hari_ini.md): Daily BAST (per kategori,
	// per entry) ke Admin Proyek (pemilik) + MO. trackingID sudah di-resolve
	// di atas lewat Bast-nya.
	NotifTugasPengadaan(tx, &activity, trackingID, "")

	entry.ActivityAdminProyekID = &activity.ID
	if err := tx.Model(&BastEntry{}).Where("id = ?", entry.ID).
		Update("activity_admin_proyek_id", entry.ActivityAdminProyekID).Error; err != nil {
		fmt.Println(">>> Gagal update BastEntry.ActivityAdminProyekID:", err)
		return err
	}

	fmt.Println(">>> Daily BastEntry dibuat:", activity.ID, "entry ke:", entry.Index)
	return nil
}

// AdvanceBastEntryIfReady dipanggil begitu daily satu BastEntry DITERIMA —
// beda dari Garansi/Termin yang butuh 2 syarat, BAST cukup satu: begitu
// entry ke-N disetujui, langsung buatin daily entry ke-N+1 (kalau ada &
// belum punya daily).
func AdvanceBastEntryIfReady(tx *gorm.DB, entry *BastEntry, picID string, kategori KategoriBast) error {
	var nextEntry BastEntry
	err := tx.Where("bast_id = ? AND index = ?", entry.BastID, entry.Index+1).First(&nextEntry).Error
	if err != nil {
		fmt.Println(">>> Gak ada entry berikutnya buat Bast:", entry.BastID)
		return nil
	}

	if nextEntry.ActivityAdminProyekID != nil && *nextEntry.ActivityAdminProyekID != "" {
		fmt.Println(">>> Daily entry berikutnya udah ada, skip:", *nextEntry.ActivityAdminProyekID)
		return nil
	}

	return CreateBastEntryActivity(tx, &nextEntry, picID, kategori)
}
