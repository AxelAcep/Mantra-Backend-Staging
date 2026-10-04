package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Tipe notifikasi — DAILY_ACTIVITY (default, notif kolaborator/reschedule
// generic) vs PENAWARAN (notifikasi tugas/proses pengadaan barang, Tahap 1–9).
const (
	NotifTipeDailyActivity = "DAILY_ACTIVITY"
	NotifTipePengadaan     = "PENAWARAN"
)

// labelTahapan memetakan StepPenawaran ke label tampilan untuk notifikasi
// pengadaan (mis. daily Presales di tahap PENYUSUNAN_BOQ → "Penyusunan BOQ").
func labelTahapan(step StepPenawaran) string {
	switch step {
	case StepPermintaanMasuk:
		return "Permintaan Masuk"
	case StepPenyusunanBoQ:
		return "Penyusunan BOQ"
	case StepReviewInternal:
		return "Review Internal"
	case StepPersetujuanManajemen:
		return "Persetujuan Manajemen"
	case StepFollowUp:
		return "Follow Up"
	case StepImplementasi:
		return "Implementasi"
	case StepBAST:
		return "BAST"
	case StepGaransi:
		return "Garansi"
	case StepPembayaran:
		return "Pembayaran"
	default:
		return ""
	}
}

// konteksPengadaan menampung data tampilan notifikasi pengadaan yang
// di-resolve dari tracking (nomor penawaran, perusahaan, lokasi, tahapan).
type konteksPengadaan struct {
	trackingID    string
	terkaitPO     string
	perusahaan    string
	lokasiProyek  string
	tahapan       string
}

// resolveKonteksPengadaan mencari konteks proses pengadaan dari nomor
// referensi (TerkaitPO activity — bisa NomorPenawaran di tahap awal atau
// nomor PO/WO di tahap implementasi). Best-effort: field yang tidak
// ditemukan dikembalikan kosong.
func resolveKonteksPengadaan(tx *gorm.DB, terkaitPO string) konteksPengadaan {
	k := konteksPengadaan{terkaitPO: terkaitPO}
	if terkaitPO == "" {
		return k
	}

	var tracking TrackingPenawaran
	if err := tx.Preload("Perusahaan").
		Where("nomor_penawaran = ?", terkaitPO).
		First(&tracking).Error; err == nil {
		k.trackingID = tracking.ID
		k.perusahaan = tracking.Perusahaan.Nama
		k.lokasiProyek = tracking.LokasiProyek
		k.tahapan = labelTahapan(tracking.StepSaatIni)
		return k
	}

	var impl Implementasi
	if err := tx.Where("no_wo = ? OR no_po = ?", terkaitPO, terkaitPO).
		First(&impl).Error; err == nil {
		k.trackingID = impl.TrackingPenawaranID
		var tracking2 TrackingPenawaran
		if err := tx.Preload("Perusahaan").
			First(&tracking2, "id = ?", impl.TrackingPenawaranID).Error; err == nil {
			k.perusahaan = tracking2.Perusahaan.Nama
			k.lokasiProyek = tracking2.LokasiProyek
			k.tahapan = labelTahapan(tracking2.StepSaatIni)
		}
	}

	return k
}

// resolveKonteksByTrackingID melengkapi konteks (nomor penawaran bila
// TerkaitPO kosong, perusahaan, lokasi, tahapan) dari trackingID — dipakai
// saat activity tidak membawa TerkaitPO/Perusahaan, mis. daily
// pengantaran/instalasi/BAST/garansi.
func resolveKonteksByTrackingID(tx *gorm.DB, k *konteksPengadaan) {
	if k.trackingID == "" {
		return
	}
	var tracking TrackingPenawaran
	if err := tx.Preload("Perusahaan").First(&tracking, "id = ?", k.trackingID).Error; err == nil {
		if k.terkaitPO == "" {
			k.terkaitPO = tracking.NomorPenawaran
		}
		if k.perusahaan == "" {
			k.perusahaan = tracking.Perusahaan.Nama
		}
		if k.lokasiProyek == "" {
			k.lokasiProyek = tracking.LokasiProyek
		}
		if k.tahapan == "" {
			k.tahapan = labelTahapan(tracking.StepSaatIni)
		}
	}
}

// insertNotifikasiPengadaan menyimpan satu baris notifikasi tipe PENAWARAN.
// Best-effort: error di-log, tidak menggagalkan proses bisnis utama
// (dipanggil juga dari dalam GORM hook transaksional).
func insertNotifikasiPengadaan(tx *gorm.DB, pegawaiID string, activityID *string, k konteksPengadaan, judul, pesan string) {
	if pegawaiID == "" {
		return
	}
	notif := Notifikasi{
		ID:                  "NTF-" + uuid.New().String(),
		PegawaiID:           pegawaiID,
		ActivityID:          activityID,
		TrackingPenawaranID: k.trackingID,
		Judul:               judul,
		Pesan:               pesan,
		TerkaitPO:           k.terkaitPO,
		Perusahaan:          k.perusahaan,
		LokasiProyek:        k.lokasiProyek,
		Tahapan:             k.tahapan,
		Tipe:                NotifTipePengadaan,
		IsRead:              false,
		CreatedAt:           time.Now(),
	}
	if err := tx.Create(&notif).Error; err != nil {
		fmt.Println(">>> Gagal membuat notifikasi pengadaan untuk", pegawaiID, ":", err)
	}
}

// fanoutMONotifikasiPengadaan mengirim salinan notifikasi ke SEMUA pegawai
// divisi MANAGER_OPERASIONAL (target_hari_ini.md: "MO sudah pasti menerima
// semua Notif"). seen dipakai untuk dedupe terhadap penerima utama/ekstra.
func fanoutMONotifikasiPengadaan(tx *gorm.DB, seen map[string]bool, activityID *string, k konteksPengadaan, judul, pesan string) {
	var moList []Pegawai
	if err := tx.Where("divisi = ?", DivisiManagerOperasional).Find(&moList).Error; err != nil {
		fmt.Println(">>> Gagal fan-out notif ke MO:", err)
		return
	}
	for _, p := range moList {
		if seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		insertNotifikasiPengadaan(tx, p.ID, activityID, k, judul, pesan)
	}
}

// FindKadivDivisi mencari ID pegawai dengan role SUPERVISI pada divisi tsb
// (Kadiv = kepala divisi). Mengembalikan "" bila belum ada — caller cukup
// meneruskan hasilnya sebagai extra recipient (kosong di-skip oleh helper).
func FindKadivDivisi(tx *gorm.DB, divisi Divisi) string {
	var p Pegawai
	err := tx.
		Joins(`JOIN "User" ON "User".pegawai_id = "Pegawai".id`).
		Where(`"Pegawai".divisi = ? AND "User".role = ?`, divisi, RoleSupervisi).
		First(&p).Error
	if err != nil {
		return ""
	}
	return p.ID
}

// FindSupervisiIDsByDivisi mengembalikan SEMUA ID pegawai dengan role
// SUPERVISI pada divisi tsb (slice, bukan single seperti FindKadivDivisi) —
// dipakai buat fan-out notifikasi kadiv (mis. Kadiv Sales/Presales untuk
// approval Review Internal). Query tanpa First() supaya semua akun kadiv
// menerima; log bila tidak ditemukan untuk debugging.
func FindSupervisiIDsByDivisi(tx *gorm.DB, divisi Divisi) []string {
	var pegawaiList []Pegawai
	err := tx.
		Joins(`JOIN "User" ON "User".pegawai_id = "Pegawai".id`).
		Where(`"Pegawai".divisi = ? AND "User".role = ?`, divisi, RoleSupervisi).
		Find(&pegawaiList).Error
	if err != nil {
		fmt.Println(">>> Gagal cari SUPERVISI divisi", divisi, ":", err)
		return nil
	}
	if len(pegawaiList) == 0 {
		fmt.Println(">>> Tidak ada akun SUPERVISI di divisi", divisi, "- notif kadiv tidak terkirim")
		return nil
	}
	ids := make([]string, 0, len(pegawaiList))
	for _, p := range pegawaiList {
		ids = append(ids, p.ID)
	}
	return ids
}

// FindPegawaiIDsByDivisi mengembalikan ID semua pegawai pada divisi tsb —
// dipakai buat fan-out notifikasi ke seluruh akun satu divisi (mis. semua
// Direktur/Komisaris untuk approval persetujuan manajemen & dokumen PO).
func FindPegawaiIDsByDivisi(tx *gorm.DB, divisiList ...Divisi) []string {
	var pegawaiList []Pegawai
	if err := tx.Where("divisi IN ?", divisiList).Find(&pegawaiList).Error; err != nil {
		fmt.Println(">>> Gagal cari pegawai by divisi:", err)
		return nil
	}
	ids := make([]string, 0, len(pegawaiList))
	for _, p := range pegawaiList {
		ids = append(ids, p.ID)
	}
	return ids
}

// NotifTugasPengadaan membuat notifikasi tugas pengadaan untuk pemilik
// activity + penerima ekstra + fan-out MO.
//
// trackingID menautkan link /penawaran/{id} (detail proses pengadaan);
// activityID menautkan link /dailyactivity/{id} (task yang di-assign).
// Bila trackingID/perusahaan/lokasiProyek kosong, di-resolve otomatis dari
// act.TerkaitPO; tahapan diambil dari stepSaatIni tracking.
func NotifTugasPengadaan(tx *gorm.DB, act *Activity, trackingID, lokasiProyek string, extraPegawaiIDs ...string) {
	k := konteksPengadaan{trackingID: trackingID, lokasiProyek: lokasiProyek}
	if act.TerkaitPO != nil {
		k.terkaitPO = *act.TerkaitPO
	}
	if act.Perusahaan != nil {
		k.perusahaan = *act.Perusahaan
	}

	if k.trackingID == "" || k.perusahaan == "" || k.lokasiProyek == "" {
		r := resolveKonteksPengadaan(tx, k.terkaitPO)
		if k.trackingID == "" {
			k.trackingID = r.trackingID
		}
		if k.perusahaan == "" {
			k.perusahaan = r.perusahaan
		}
		if k.lokasiProyek == "" {
			k.lokasiProyek = r.lokasiProyek
		}
		if k.tahapan == "" {
			k.tahapan = r.tahapan
		}
	}
	resolveKonteksByTrackingID(tx, &k)

	seen := map[string]bool{}
	if act.PegawaiID != "" {
		seen[act.PegawaiID] = true
		insertNotifikasiPengadaan(tx, act.PegawaiID, &act.ID, k, act.Judul, act.Deskripsi)
	}
	for _, id := range extraPegawaiIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		insertNotifikasiPengadaan(tx, id, &act.ID, k, act.Judul, act.Deskripsi)
	}

	fanoutMONotifikasiPengadaan(tx, seen, &act.ID, k, act.Judul, act.Deskripsi)
}

// NotifPengadaanEvent membuat notifikasi peristiwa pengadaan tanpa task baru
// (mis. approval/ditolak Direktur). Penerima eksplisit + fan-out MO.
// activityID boleh nil — link daily activity tidak tampil di card bila kosong.
func NotifPengadaanEvent(tx *gorm.DB, judul, pesan, trackingID, terkaitPO, perusahaan, lokasiProyek string, activityID *string, pegawaiIDs ...string) {
	k := konteksPengadaan{trackingID: trackingID, terkaitPO: terkaitPO, perusahaan: perusahaan, lokasiProyek: lokasiProyek}

	if k.trackingID == "" || k.perusahaan == "" || k.lokasiProyek == "" {
		r := resolveKonteksPengadaan(tx, k.terkaitPO)
		if k.trackingID == "" {
			k.trackingID = r.trackingID
		}
		if k.perusahaan == "" {
			k.perusahaan = r.perusahaan
		}
		if k.lokasiProyek == "" {
			k.lokasiProyek = r.lokasiProyek
		}
		if k.tahapan == "" {
			k.tahapan = r.tahapan
		}
	}
	resolveKonteksByTrackingID(tx, &k)

	seen := map[string]bool{}
	for _, id := range pegawaiIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		insertNotifikasiPengadaan(tx, id, activityID, k, judul, pesan)
	}

	fanoutMONotifikasiPengadaan(tx, seen, activityID, k, judul, pesan)
}
