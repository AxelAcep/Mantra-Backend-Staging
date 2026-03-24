package models

import "time"

// ─── Enum ─────────────────────────────────────────────────────────────────────

type NilaiKPI string

const (
	NilaiKPIBaik  NilaiKPI = "BAIK"
	NilaiKPICukup NilaiKPI = "CUKUP"
	NilaiKPIBuruk NilaiKPI = "BURUK"
)

// ─── Model ────────────────────────────────────────────────────────────────────
// Satu row = satu pegawai di satu bulan/tahun
// Baik/Cukup/Buruk di-increment tiap kali master add KPI

type KPIPegawai struct {
	ID        string    `gorm:"primaryKey"                              json:"id"`
	PegawaiID string    `gorm:"not null;uniqueIndex:idx_kpi_unique"     json:"pegawaiId"`
	Pegawai   Pegawai   `gorm:"foreignKey:PegawaiID;references:ID"      json:"pegawai,omitempty"`
	Bulan     int       `gorm:"not null;uniqueIndex:idx_kpi_unique"     json:"bulan"`
	Tahun     int       `gorm:"not null;uniqueIndex:idx_kpi_unique"     json:"tahun"`
	Minggu    int       `gorm:"not null;uniqueIndex:idx_kpi_unique"     json:"minggu"` // ← NEW: 1-4
	Baik      int       `gorm:"default:0;not null"                      json:"baik"`
	Cukup     int       `gorm:"default:0;not null"                      json:"cukup"`
	Buruk     int       `gorm:"default:0;not null"                      json:"buruk"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (KPIPegawai) TableName() string { return "KPIPegawai" }

func (k *KPIPegawai) GetField(nilai NilaiKPI) int {
	switch nilai {
	case NilaiKPIBaik:
		return k.Baik
	case NilaiKPICukup:
		return k.Cukup
	default:
		return k.Buruk
	}
}
