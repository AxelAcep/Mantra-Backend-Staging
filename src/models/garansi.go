package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ─── Enums ────────────────────────────────────────────────────────────────────

type StatusGaransi string

const (
	StatusGaransiBelumDikonfigurasi StatusGaransi = "BELUM_DIKONFIGURASI"
	StatusGaransiOnProgress         StatusGaransi = "ON_PROGRESS"
	StatusGaransiSelesai            StatusGaransi = "SELESAI"
)

type KategoriGaransi string

const (
	KategoriPACDalamKota  KategoriGaransi = "PAC_DALAM_KOTA"
	KategoriPACLuarKota   KategoriGaransi = "PAC_LUAR_KOTA"
	KategoriFireDalamKota KategoriGaransi = "FIRE_DALAM_KOTA"
	KategoriFireLuarKota  KategoriGaransi = "FIRE_LUAR_KOTA"
	// Battery/UPS — periode 4x setahun dalam kota, 2x setahun luar kota.
	KategoriBatteryDalamKota KategoriGaransi = "BATTERY_DALAM_KOTA"
	KategoriBatteryLuarKota  KategoriGaransi = "BATTERY_LUAR_KOTA"
	// KategoriUmum dipakai kalau Bast pemicunya UMUM (gak ada PAC/FIRE
	// kedetect dari Jenis Penawaran) — garansi bulanan biasa, gak ada
	// pembedaan dalam/luar kota.
	KategoriUmum     KategoriGaransi = "UMUM"
	KategoriTidakAda KategoriGaransi = "TIDAK_ADA"
)

// AllowedKategoriGaransi nentuin pilihan kategori garansi (manual, dalam/luar
// kota) yang boleh dipilih user — dibatasin sesuai KategoriBast si Garansi
// (yang udah otomatis ke-detect dari Bast pemicunya). "Tidak Ada" selalu jadi
// opsi tambahan di semua kategori, buat override "proyek ini emang gak ada
// garansi" kapan pun dibutuhkan.
func AllowedKategoriGaransi(kategoriBast KategoriBast) []KategoriGaransi {
	switch kategoriBast {
	case KategoriBastPAC:
		return []KategoriGaransi{KategoriPACDalamKota, KategoriPACLuarKota, KategoriTidakAda}
	case KategoriBastFire:
		return []KategoriGaransi{KategoriFireDalamKota, KategoriFireLuarKota, KategoriTidakAda}
	case KategoriBastBattery:
		return []KategoriGaransi{KategoriBatteryDalamKota, KategoriBatteryLuarKota, KategoriTidakAda}
	default: // UMUM
		return []KategoriGaransi{KategoriUmum, KategoriTidakAda}
	}
}

// ─── Log Garansi ──────────────────────────────────────────────────────────────

type LogGaransi struct {
	Aksi        string    `json:"aksi"`
	Keterangan  string    `json:"keterangan"`
	PegawaiID   string    `json:"pegawaiId"`
	NamaPegawai string    `json:"namaPegawai"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ─── Garansi ──────────────────────────────────────────────────────────────────
// Auto-dibuat ketika BAST selesai (semua BastEntry DITERIMA), status awal
// BELUM_DIKONFIGURASI sampai user input LamaTahun + BulanMulai/TahunMulai.

type Garansi struct {
	ID                  string            `gorm:"primaryKey"                                                              json:"id"`
	TrackingPenawaranID string            `gorm:"not null;index;uniqueIndex:idx_garansi_tracking_kategoribast"           json:"trackingPenawaranId"`
	TrackingPenawaran   TrackingPenawaran `gorm:"foreignKey:TrackingPenawaranID;references:ID"                           json:"trackingPenawaran,omitempty"`
	BastID              string            `gorm:"not null;index"                                                          json:"bastId"`
	Bast                Bast              `gorm:"foreignKey:BastID;references:ID"                                        json:"bast,omitempty"`

	// Kategori inti (PAC/FIRE/UMUM) — otomatis ke-detect dari Bast yang
	// men-trigger Garansi ini (persis mekanisme DetectBastKategori di BAST).
	// Satu tracking bisa punya sampai 2 Garansi (PAC & FIRE) kalau BAST-nya
	// juga kebentuk 2 — unique per kombinasi tracking+kategori, bukan per
	// tracking doang.
	KategoriBast KategoriBast `gorm:"not null;default:UMUM;uniqueIndex:idx_garansi_tracking_kategoribast" json:"kategoriBast"`

	PICID string  `gorm:"not null;index"                  json:"picId"`
	PIC   Pegawai `gorm:"foreignKey:PICID;references:ID" json:"pic,omitempty"`

	Status StatusGaransi `gorm:"not null;default:BELUM_DIKONFIGURASI;index" json:"status"`

	// Pilihan manual dalam/luar kota (atau UMUM/Tidak Ada) — opsinya
	// dibatasin sesuai KategoriBast, lihat AllowedKategoriGaransi.
	KategoriGaransi KategoriGaransi `gorm:"not null;default:''" json:"kategoriGaransi"`

	// Diisi manual lewat endpoint konfigurasi timeline
	LamaTahun  *int `gorm:"default:null" json:"lamaTahun,omitempty"`
	BulanMulai *int `gorm:"default:null" json:"bulanMulai,omitempty"` // 1-12
	TahunMulai *int `gorm:"default:null" json:"tahunMulai,omitempty"`

	LogAktivitas []LogGaransi `gorm:"serializer:json;default:'[]'" json:"logs"`

	Months []GaransiMonth `gorm:"foreignKey:GaransiID" json:"months,omitempty"`

	CreatedAt time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time `             json:"updatedAt"`
}

// ─── Garansi Month (slot timeline per bulan) ──────────────────────────────────
// Semua slot digenerate sekaligus saat konfigurasi, tapi daily (Activity) hanya
// dibuat bulan berjalan — bulan berikutnya baru dibuat setelah bulan berjalan
// DITERIMA (approval manager) DAN TanggalKunjungan sudah terisi.

type GaransiMonth struct {
	ID        string  `gorm:"primaryKey"     json:"id"`
	GaransiID string  `gorm:"not null;index;uniqueIndex:idx_garansi_bulan_ke" json:"garansiId"`
	Garansi   Garansi `gorm:"foreignKey:GaransiID;references:ID" json:"-"`

	BulanKe int `gorm:"not null;uniqueIndex:idx_garansi_bulan_ke" json:"bulanKe"` // urutan ke berapa (1..N)
	Bulan   int `gorm:"not null"                                  json:"bulan"`  // bulan kalender (1-12)
	Tahun   int `gorm:"not null"                                  json:"tahun"`

	TanggalKunjungan *time.Time `json:"tanggalKunjungan,omitempty"`

	ActivityID *string   `gorm:"index"                                json:"activityId,omitempty"`
	Activity   *Activity `gorm:"foreignKey:ActivityID;references:ID" json:"activity,omitempty"`

	// Status slot: PENDING (belum ada daily) | ON_PROGRESS (daily jalan) | DITERIMA (bulan ini tuntas)
	Status StatusActivity `gorm:"not null;default:PENDING;index" json:"status"`
	// ActivitySelesai true begitu daily bulan ini DITERIMA — dipakai bareng
	// TanggalKunjungan untuk nentuin kapan bulan berikutnya boleh dibuat.
	ActivitySelesai bool `gorm:"not null;default:false" json:"activitySelesai"`

	LogAktivitas []LogGaransi `gorm:"serializer:json;default:'[]'" json:"logs"`

	CreatedAt time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time `             json:"updatedAt"`
}

func (Garansi) TableName() string      { return "Garansi" }
func (GaransiMonth) TableName() string { return "GaransiMonth" }

// KategoriSlotPattern mengembalikan jumlah kunjungan per tahun dan interval
// bulan untuk setiap kategori garansi.
func KategoriSlotPattern(kategori KategoriGaransi) (kunjunganPerTahun int, bulanOffsets []int) {
	switch kategori {
	case KategoriPACDalamKota:
		return 12, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	case KategoriPACLuarKota:
		return 2, []int{1, 7}
	case KategoriFireDalamKota:
		return 4, []int{1, 4, 7, 10}
	case KategoriFireLuarKota:
		return 2, []int{1, 7}
	// Battery/UPS — 4x setahun dalam kota, 2x setahun luar kota.
	case KategoriBatteryDalamKota:
		return 4, []int{1, 4, 7, 10}
	case KategoriBatteryLuarKota:
		return 2, []int{1, 7}
	case KategoriUmum:
		return 12, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	default:
		return 0, nil
	}
}

// ─── Helpers bersama (dipakai controller & hook) ──────────────────────────────

func namaBulanIndo(bulan int) string {
	nama := []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	if bulan < 1 || bulan > 12 {
		return ""
	}
	return nama[bulan-1]
}

func lastDayOfGaransiMonth(tahun, bulan int) time.Time {
	firstOfNext := time.Date(tahun, time.Month(bulan)+1, 1, 23, 59, 59, 0, time.Local)
	return firstOfNext.AddDate(0, 0, -1)
}

// CreateGaransiMonthActivity membuat daily Activity untuk satu slot GaransiMonth
// (judul/deskripsi otomatis sesuai bulan ke berapa) dan meng-update slotnya.
// Dipanggil saat konfigurasi timeline (bulan ke-1) maupun otomatis dari hook
// AfterUpdate (bulan ke-2..N, lewat AdvanceGaransiIfReady).
func CreateGaransiMonthActivity(tx *gorm.DB, month *GaransiMonth, picID, pegawaiID, namaPegawai string) error {
	now := time.Now()
	deadline := lastDayOfGaransiMonth(month.Tahun, month.Bulan)

	// Resolve Garansi dulu — dipakai buat TerkaitPO (Nomor Penawaran) dan
	// notifikasi di bawah.
	var garansiForNotif Garansi
	trackingIDGaransi := ""
	if err := tx.First(&garansiForNotif, "id = ?", month.GaransiID).Error; err == nil {
		trackingIDGaransi = garansiForNotif.TrackingPenawaranID
	}

	activity := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     picID,
		TerkaitPO:     nomorPenawaranTracking(tx, trackingIDGaransi),
		Kategori:      KategoriAkomodasiProject,
		Judul:         fmt.Sprintf("Kunjungan Garansi Bulan ke-%d", month.BulanKe),
		Deskripsi:     fmt.Sprintf("Activity otomatis kunjungan garansi bulan ke-%d (%s %d)", month.BulanKe, namaBulanIndo(month.Bulan), month.Tahun),
		WaktuMulai:    now,
		TargetSelesai: deadline,
		Status:        StatusOnProgress,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := tx.Create(&activity).Error; err != nil {
		fmt.Println(">>> Gagal membuat Activity Garansi bulan ke-", month.BulanKe, ":", err)
		return err
	}

	// Notifikasi Tahap 8 (target_hari_ini.md): Daily garansi (per bulan, per
	// kategori) ke Admin Proyek (pemilik) + MO.
	NotifTugasPengadaan(tx, &activity, trackingIDGaransi, "")

	month.ActivityID = &activity.ID
	month.Status = StatusOnProgress
	month.LogAktivitas = append(month.LogAktivitas, LogGaransi{
		Aksi:        "Buat Daily Kunjungan",
		Keterangan:  fmt.Sprintf("Daily kunjungan garansi bulan ke-%d otomatis dibuat", month.BulanKe),
		PegawaiID:   pegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   now,
	})

	if err := tx.Model(&GaransiMonth{}).Where("id = ?", month.ID).
		Select("activity_id", "status", "log_aktivitas").
		Updates(GaransiMonth{
			ActivityID:   month.ActivityID,
			Status:       month.Status,
			LogAktivitas: month.LogAktivitas,
		}).Error; err != nil {
		fmt.Println(">>> Gagal update GaransiMonth setelah buat Activity:", err)
		return err
	}

	fmt.Println(">>> Daily Garansi dibuat:", activity.ID, "bulan ke:", month.BulanKe)
	return nil
}

// AdvanceGaransiIfReady mengecek kondisi "daily bulan ini DITERIMA DAN tanggal
// kunjungan sudah terisi" lalu membuat daily bulan berikutnya (atau menandai
// Garansi SELESAI kalau ini bulan terakhir). Dipanggil dari dua titik:
// (1) hook AfterUpdate saat Activity bulan berjalan disetujui selesai,
// (2) endpoint update tanggal kunjungan — karena approval & tanggal kunjungan
// bisa terisi di urutan mana pun.
func AdvanceGaransiIfReady(tx *gorm.DB, month *GaransiMonth, pegawaiID, namaPegawai string) error {
	if !month.ActivitySelesai || month.TanggalKunjungan == nil {
		fmt.Println(">>> GaransiMonth belum siap lanjut ke bulan berikutnya:", month.ID)
		return nil
	}

	var garansi Garansi
	if err := tx.Where("id = ?", month.GaransiID).First(&garansi).Error; err != nil {
		fmt.Println(">>> Garansi tidak ditemukan, skip:", err)
		return nil
	}

	// Guard idempotency: kalau Garansi ini udah pernah ditandai tuntas, jangan
	// diproses ulang (termasuk gak bikin dobel daily penawaran maintenance).
	if garansi.Status == StatusGaransiSelesai {
		fmt.Println(">>> Garansi udah tuntas sebelumnya, skip:", garansi.ID)
		return nil
	}

	var nextMonth GaransiMonth
	err := tx.Where("garansi_id = ? AND bulan_ke = ?", month.GaransiID, month.BulanKe+1).First(&nextMonth).Error
	if err != nil {
		// Tidak ada bulan berikutnya → ini bulan terakhir, Garansi tuntas.
		fmt.Println(">>> Bulan terakhir garansi tuntas:", garansi.ID)
		if err := tx.Model(&Garansi{}).Where("id = ?", garansi.ID).Update("status", StatusGaransiSelesai).Error; err != nil {
			return err
		}
		return CreatePenawaranMaintenanceActivity(tx, &garansi, pegawaiID, namaPegawai)
	}

	// Guard idempotency: kalau bulan berikutnya udah punya daily, jangan dobel.
	if nextMonth.ActivityID != nil && *nextMonth.ActivityID != "" {
		fmt.Println(">>> Daily bulan berikutnya udah ada, skip:", *nextMonth.ActivityID)
		return nil
	}

	return CreateGaransiMonthActivity(tx, &nextMonth, garansi.PICID, pegawaiID, namaPegawai)
}

// CreatePenawaranMaintenanceActivity dipanggil begitu sebuah Garansi tuntas
// (bulan terakhir DITERIMA) — bikin daily buat Admin Proyek supaya nawarin
// kontrak maintenance ke customer, sekarang garansinya udah abis.
func CreatePenawaranMaintenanceActivity(tx *gorm.DB, garansi *Garansi, pegawaiID, namaPegawai string) error {
	var followUp FollowUp
	if err := tx.Where("tracking_penawaran_id = ?", garansi.TrackingPenawaranID).First(&followUp).Error; err != nil {
		fmt.Println(">>> FollowUp tidak ditemukan, skip pembuatan daily penawaran maintenance:", err)
		return nil
	}
	if followUp.ActivityAdminProyekID == nil || *followUp.ActivityAdminProyekID == "" {
		fmt.Println(">>> FollowUp belum punya Admin Proyek, skip pembuatan daily penawaran maintenance")
		return nil
	}
	var adminProyekActivity Activity
	if err := tx.Where("id = ?", *followUp.ActivityAdminProyekID).First(&adminProyekActivity).Error; err != nil {
		fmt.Println(">>> Activity Admin Proyek tidak ditemukan, skip pembuatan daily penawaran maintenance:", err)
		return nil
	}

	kategoriLabel := ""
	if garansi.KategoriBast != KategoriBastUmum {
		kategoriLabel = fmt.Sprintf(" %s", garansi.KategoriBast)
	}

	now := time.Now()
	activity := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     adminProyekActivity.PegawaiID,
		TerkaitPO:     nomorPenawaranTracking(tx, garansi.TrackingPenawaranID),
		Kategori:      KategoriAkomodasiProject,
		Judul:         fmt.Sprintf("Penawaran Maintenance%s ke Customer", kategoriLabel),
		Deskripsi:     "Activity otomatis setelah masa garansi tuntas — tawarkan kontrak maintenance ke customer.",
		WaktuMulai:    now,
		TargetSelesai: now.Add(7 * 24 * time.Hour),
		Status:        StatusOnProgress,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := tx.Create(&activity).Error; err != nil {
		fmt.Println(">>> Gagal membuat Activity Penawaran Maintenance:", err)
		return err
	}

	// Notifikasi Tahap 8 (target_hari_ini.md): Daily penawaran maintenance
	// ke Admin Proyek (pemilik) + MO.
	NotifTugasPengadaan(tx, &activity, garansi.TrackingPenawaranID, "")

	garansi.LogAktivitas = append(garansi.LogAktivitas, LogGaransi{
		Aksi:        "Buat Daily Penawaran Maintenance",
		Keterangan:  "Garansi tuntas, daily penawaran maintenance ke customer otomatis dibuat untuk Admin Proyek",
		PegawaiID:   pegawaiID,
		NamaPegawai: namaPegawai,
		CreatedAt:   now,
	})
	if err := tx.Model(&Garansi{}).Where("id = ?", garansi.ID).
		Select("log_aktivitas").
		Updates(Garansi{LogAktivitas: garansi.LogAktivitas}).Error; err != nil {
		fmt.Println(">>> Gagal update log Garansi setelah buat daily penawaran maintenance:", err)
		return err
	}

	fmt.Println(">>> Daily Penawaran Maintenance dibuat:", activity.ID, "untuk pegawai:", activity.PegawaiID)
	return nil
}
