package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DetectBastKategori nentuin BAST apa aja yang perlu dibuat buat satu
// tracking, dilihat dari JenisPenawaran-nya. Cuma 2 hal yang dicek:
// "PAC Montair" -> PAC, "Generator FirePro" -> FIRE. Item jenis penawaran
// lain gak ngaruh sama sekali ke pembentukan BAST. Kalau dua-duanya ada,
// hasilnya [PAC, FIRE] (2 Bast). Kalau gak ada dua-duanya, hasilnya
// [UMUM] (1 Bast generik, kayak perilaku lama).
func DetectBastKategori(jenisPenawaran []JenisPenawaran) []KategoriBast {
	hasPAC := false
	hasFire := false
	for _, j := range jenisPenawaran {
		switch {
		case strings.EqualFold(strings.TrimSpace(string(j)), "PAC Montair"):
			hasPAC = true
		case strings.EqualFold(strings.TrimSpace(string(j)), "Generator FirePro"):
			hasFire = true
		}
	}

	var kategori []KategoriBast
	if hasPAC {
		kategori = append(kategori, KategoriBastPAC)
	}
	if hasFire {
		kategori = append(kategori, KategoriBastFire)
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
//   UMUM: BAST-YYMM-XXX-###
//   PAC : BAST-PAC-YYMM-XXX-###
//   FIRE: BAST-FIR-YYMM-XXX-###
// Nomor urut (###, 3 digit) reset per kombinasi kode+bulan+tahun+kategori —
// dihitung dari BastEntry yang udah ada dengan prefix kode yang sama persis.
func GenerateBastKode(tx *gorm.DB, kategori KategoriBast, kodePerusahaan string, tahun, bulan int) string {
	prefix := "BAST"
	switch kategori {
	case KategoriBastPAC:
		prefix = "BAST-PAC"
	case KategoriBastFire:
		prefix = "BAST-FIR"
	}

	base := fmt.Sprintf("%s-%02d%02d-%s", prefix, tahun%100, bulan, kodePerusahaan)

	var count int64
	tx.Model(&BastEntry{}).Where("no_referensi LIKE ?", base+"-%").Count(&count)

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

	if err := tx.Create(&activity).Error; err != nil {
		fmt.Println(">>> Gagal membuat Activity BastEntry:", err)
		return err
	}

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
