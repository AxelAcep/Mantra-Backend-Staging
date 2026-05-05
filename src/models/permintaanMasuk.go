package models

import (
	"time"

	"gorm.io/gorm"
)

// ─── Enums ────────────────────────────────────────────────────────────────────

type JenisPenawaran string

const (
	JenisPACMontair      JenisPenawaran = "PAC_MONTAIR"
	JenisGenerator       JenisPenawaran = "GENERATOR"
	JenisFirePro         JenisPenawaran = "FIRE_PRO"
	JenisConventionalSys JenisPenawaran = "CONVENTIONAL_SYS"
	JenisAddressableSys  JenisPenawaran = "ADDRESSABLE_SYS"
	JenisStandAloneBTA   JenisPenawaran = "STAND_ALONE_BTA"
	JenisBattery         JenisPenawaran = "BATTERY"
	JenisChiller         JenisPenawaran = "CHILLER"
	JenisUPS             JenisPenawaran = "UPS"
	JenisACSplitStanding JenisPenawaran = "AC_SPLIT_STANDING"
)

type StepPenawaran string

const (
	StepPermintaanMasuk      StepPenawaran = "PERMINTAAN_MASUK"
	StepPenyusunanBoQ        StepPenawaran = "PENYUSUNAN_BOQ"
	StepReviewInternal       StepPenawaran = "REVIEW_INTERNAL"
	StepPersetujuanManajemen StepPenawaran = "PERSETUJUAN_MANAJEMEN"
	StepFollowUp             StepPenawaran = "FOLLOW_UP"
	StepImplementasi         StepPenawaran = "IMPLEMENTASI"
	StepBAST                 StepPenawaran = "BAST"
	StepPembayaran           StepPenawaran = "PEMBAYARAN"
	StepGaransi              StepPenawaran = "GARANSI"
)

// ─── Log Aktivitas ────────────────────────────────────────────────────────────

type LogAktivitas struct {
	Aksi        string    `json:"aksi"`
	Keterangan  string    `json:"keterangan"`
	PegawaiID   string    `json:"pegawaiId"`
	NamaPegawai string    `json:"namaPegawai"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ─── Tracking Penawaran (Master) ──────────────────────────────────────────────

type TrackingPenawaran struct {
	ID             string           `gorm:"primaryKey"                              json:"id"`
	NomorPenawaran string           `gorm:"not null;uniqueIndex"                    json:"nomorPenawaran"`
	NomorPO        *string          `gorm:"index"                                   json:"nomorPO,omitempty"`
	PerusahaanID   string           `gorm:"not null;index"                          json:"perusahaanId"`
	Perusahaan     Perusahaan       `gorm:"foreignKey:PerusahaanID;references:ID"   json:"perusahaan,omitempty"`
	MarketingID    string           `gorm:"not null;index"                          json:"marketingId"`
	Marketing      Pegawai          `gorm:"foreignKey:MarketingID;references:ID"    json:"marketing,omitempty"`
	LokasiProyek   string           `gorm:"not null"                                json:"lokasiProyek"`
	CustomerName   string           `gorm:"not null"                                json:"customerName"`
	CustomerPhone  string           `gorm:"not null"                                json:"customerPhone"`
	CustomerEmail  string           `gorm:"not null"                                json:"customerEmail"`
	JenisPenawaran []JenisPenawaran `gorm:"serializer:json;not null"                json:"jenisPenawaran"`
	StepSaatIni    StepPenawaran    `gorm:"not null;default:PERMINTAAN_MASUK;index" json:"stepSaatIni"`

	PermintaanMasuk      *PermintaanMasuk      `gorm:"foreignKey:TrackingPenawaranID" json:"permintaanMasuk,omitempty"`
	PenyusunanBoQ        *PenyusunanBoQ        `gorm:"foreignKey:TrackingPenawaranID" json:"penyusunanBoQ,omitempty"`
	ReviewInternal       *ReviewInternal       `gorm:"foreignKey:TrackingPenawaranID" json:"reviewInternal,omitempty"`
	PersetujuanManajemen *PersetujuanManajemen `gorm:"foreignKey:TrackingPenawaranID" json:"persetujuanManajemen,omitempty"`

	Chat []PenawaranChat `gorm:"foreignKey:TrackingPenawaranID" json:"chat,omitempty"`

	CreatedAt time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time `             json:"updatedAt"`
}

// ─── Step 1: Permintaan Masuk ─────────────────────────────────────────────────

type PermintaanMasuk struct {
	ID                  string            `gorm:"primaryKey"                                    json:"id"`
	TrackingPenawaranID string            `gorm:"not null;uniqueIndex;index"                    json:"trackingPenawaranId"`
	TrackingPenawaran   TrackingPenawaran `gorm:"foreignKey:TrackingPenawaranID;references:ID"  json:"trackingPenawaran,omitempty"`
	PreSalesID          *string           `gorm:"index"                                         json:"preSalesId,omitempty"`
	PreSales            *Pegawai          `gorm:"foreignKey:PreSalesID;references:ID"           json:"preSales,omitempty"`
	ActivityID          *string           `gorm:"index"                                         json:"activityId,omitempty"`
	Activity            *Activity         `gorm:"foreignKey:ActivityID;references:ID"           json:"activity,omitempty"`
	Status              StatusActivity    `gorm:"not null;default:ON_PROGRESS;index"            json:"status"`
	Logs                []LogAktivitas    `gorm:"serializer:json;default:'[]'"                  json:"logs"`
	Dokumen             []PenawaranDokumen `gorm:"foreignKey:PermintaanMasukID"                 json:"dokumen,omitempty"`
	CreatedAt           time.Time         `gorm:"index"                                         json:"createdAt"`
	UpdatedAt           time.Time         `                                                      json:"updatedAt"`
}

// ─── Step 2: Penyusunan BoQ ───────────────────────────────────────────────────

type PenyusunanBoQ struct {
	ID                  string            `gorm:"primaryKey"                                    json:"id"`
	TrackingPenawaranID string            `gorm:"not null;uniqueIndex;index"                    json:"trackingPenawaranId"`
	TrackingPenawaran   TrackingPenawaran `gorm:"foreignKey:TrackingPenawaranID;references:ID"  json:"trackingPenawaran,omitempty"`
	PembuatID           *string           `gorm:"index"                                         json:"pembuatId,omitempty"`
	Pembuat             *Pegawai          `gorm:"foreignKey:PembuatID;references:ID"            json:"pembuat,omitempty"`
	ActivityID          *string           `gorm:"index"                                         json:"activityId,omitempty"`
	Activity            *Activity         `gorm:"foreignKey:ActivityID;references:ID"           json:"activity,omitempty"`
	EstimasiHarga       *float64          `gorm:"default:null"                                  json:"estimasiHarga,omitempty"`
	Status              StatusActivity    `gorm:"not null;default:ON_PROGRESS;index"            json:"status"`
	Dokumen             []PenawaranDokumen `gorm:"foreignKey:PenyusunanBoQID"                   json:"dokumen,omitempty"`
	CreatedAt           time.Time         `gorm:"index"                                         json:"createdAt"`
	UpdatedAt           time.Time         `                                                      json:"updatedAt"`
}

// ─── Step 3: Review Internal ──────────────────────────────────────────────────

type ReviewInternal struct {
	ID                  string            `gorm:"primaryKey"                                    json:"id"`
	TrackingPenawaranID string            `gorm:"not null;uniqueIndex;index"                    json:"trackingPenawaranId"`
	TrackingPenawaran   TrackingPenawaran `gorm:"foreignKey:TrackingPenawaranID;references:ID"  json:"trackingPenawaran,omitempty"`
	AccAdminDirektur    bool              `gorm:"not null;default:false"                        json:"accAdminDirektur"`
	AccManajerOps       bool              `gorm:"not null;default:false"                        json:"accManajerOps"`
	Status              StatusActivity    `gorm:"not null;default:ON_PROGRESS;index"            json:"status"`
	Dokumen             []PenawaranDokumen `gorm:"foreignKey:ReviewInternalID"                  json:"dokumen,omitempty"`
	CreatedAt           time.Time         `gorm:"index"                                         json:"createdAt"`
	UpdatedAt           time.Time         `                                                      json:"updatedAt"`
}

// ─── Step 4: Persetujuan Manajemen ────────────────────────────────────────────

type PersetujuanManajemen struct {
	ID                  string            `gorm:"primaryKey"                                    json:"id"`
	TrackingPenawaranID string            `gorm:"not null;uniqueIndex;index"                    json:"trackingPenawaranId"`
	TrackingPenawaran   TrackingPenawaran `gorm:"foreignKey:TrackingPenawaranID;references:ID"  json:"trackingPenawaran,omitempty"`
	AccDirekturUtama    bool              `gorm:"not null;default:false"                        json:"accDirekturUtama"`
	Status              StatusActivity    `gorm:"not null;default:ON_PROGRESS;index"            json:"status"`
	Dokumen             []PenawaranDokumen `gorm:"foreignKey:PersetujuanManajemenID"            json:"dokumen,omitempty"`
	CreatedAt           time.Time         `gorm:"index"                                         json:"createdAt"`
	UpdatedAt           time.Time         `                                                      json:"updatedAt"`
}

// ─── Dokumen ──────────────────────────────────────────────────────────────────

type PenawaranDokumen struct {
	ID                     string    `gorm:"primaryKey"     json:"id"`
	NamaFile               string    `gorm:"not null"       json:"namaFile"`
	Path                   string    `gorm:"not null"       json:"path"`
	UploadedBy             string    `gorm:"not null;index" json:"uploadedBy"`
	Pegawai                Pegawai   `gorm:"foreignKey:UploadedBy;references:ID"          json:"pegawai,omitempty"`

	PermintaanMasukID      *string   `gorm:"index" json:"permintaanMasukId,omitempty"`
	PenyusunanBoQID        *string   `gorm:"index" json:"penyusunanBoQId,omitempty"`
	ReviewInternalID       *string   `gorm:"index" json:"reviewInternalId,omitempty"`
	PersetujuanManajemenID *string   `gorm:"index" json:"persetujuanManajemenId,omitempty"`

	ActivityID             *string   `gorm:"index" json:"activityId,omitempty"`
	Activity               *Activity `gorm:"foreignKey:ActivityID;references:ID" json:"activity,omitempty"`

	CreatedAt              time.Time `json:"createdAt"`
}

// ─── Chat ─────────────────────────────────────────────────────────────────────

type PenawaranChat struct {
	ID                  string    `gorm:"primaryKey"                         json:"id"`
	TrackingPenawaranID string    `gorm:"not null;index"                     json:"trackingPenawaranId"`
	PegawaiID           string    `gorm:"not null;index"                     json:"pegawaiId"`
	Pegawai             Pegawai   `gorm:"foreignKey:PegawaiID;references:ID" json:"pegawai,omitempty"`
	Pesan               string    `gorm:"not null"                           json:"pesan"`
	ReadBy              []string  `gorm:"serializer:json;default:'[]'"       json:"readBy"`
	CreatedAt           time.Time `gorm:"index"                              json:"createdAt"`
}

// ─── Table Names ──────────────────────────────────────────────────────────────

func (TrackingPenawaran) TableName() string    { return "TrackingPenawaran" }
func (PermintaanMasuk) TableName() string      { return "PermintaanMasuk" }
func (PenyusunanBoQ) TableName() string        { return "PenyusunanBoQ" }
func (ReviewInternal) TableName() string       { return "ReviewInternal" }
func (PersetujuanManajemen) TableName() string { return "PersetujuanManajemen" }
func (PenawaranDokumen) TableName() string     { return "PenawaranDokumen" }
func (PenawaranChat) TableName() string        { return "PenawaranChat" }

// ─── Hooks ────────────────────────────────────────────────────────────────────

func (p *PermintaanMasuk) BeforeCreate(tx *gorm.DB) error { return nil }
