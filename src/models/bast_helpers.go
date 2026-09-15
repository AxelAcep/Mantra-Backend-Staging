package models

import (
	"fmt"
	"strings"

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
