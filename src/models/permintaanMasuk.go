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

type LogBoq struct {
	Aksi        string    `json:"aksi"`
	Keterangan  string    `json:"keterangan"`
	PegawaiID   string    `json:"pegawaiId"`
	NamaPegawai string    `json:"namaPegawai"`
	CreatedAt   time.Time `json:"createdAt"`
}

type LogReviewInternal struct {
    Aksi        string    `json:"aksi"`
    Keterangan  string    `json:"keterangan"`
    PegawaiID   string    `json:"pegawaiId"`
    NamaPegawai string    `json:"namaPegawai"`
    CreatedAt   time.Time `json:"createdAt"`
}

type LogPersetujuanManajemen struct {
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
	Status		   StatusActivity   `gorm:"not null;default:ON_PROGRESS;index"      json:"status"`

	PermintaanMasuk      *PermintaanMasuk      `gorm:"foreignKey:TrackingPenawaranID" json:"permintaanMasuk,omitempty"`
	PenyusunanBoQ        *PenyusunanBoQ        `gorm:"foreignKey:TrackingPenawaranID" json:"penyusunanBoQ,omitempty"`
	ReviewInternal       *ReviewInternal       `gorm:"foreignKey:TrackingPenawaranID" json:"reviewInternal,omitempty"`
	PersetujuanManajemen *PersetujuanManajemen `gorm:"foreignKey:TrackingPenawaranID" json:"persetujuanManajemen,omitempty"`
	FollowUp             *FollowUp             `gorm:"foreignKey:TrackingPenawaranID" json:"followUp,omitempty"`
	Implementasi         *Implementasi         `gorm:"foreignKey:TrackingPenawaranID" json:"implementasi,omitempty"`
	Accounting 			 *TerminPembayaran `gorm:"foreignKey:TrackingPenawaranID" json:"accounting,omitempty"`
	Bast 				 *Bast `gorm:"foreignKey:TrackingPenawaranID" json:"bast,omitempty"`

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
    ID                  string            `gorm:"primaryKey" json:"id"`
    TrackingPenawaranID string            `gorm:"not null;uniqueIndex;index" json:"trackingPenawaranId"`
    TrackingPenawaran   TrackingPenawaran `gorm:"foreignKey:TrackingPenawaranID;references:ID" json:"trackingPenawaran,omitempty"`
    PembuatID           *string           `gorm:"index" json:"pembuatId,omitempty"`
    Pembuat             *Pegawai          `gorm:"foreignKey:PembuatID;references:ID" json:"pembuat,omitempty"`
    ActivityID          *string           `gorm:"index" json:"activityId,omitempty"`
    Activity            *Activity         `gorm:"foreignKey:ActivityID;references:ID" json:"activity,omitempty"`
    EstimasiHarga       *float64          `gorm:"default:null" json:"estimasiHarga,omitempty"`
    Harga1              *float64          `gorm:"default:null" json:"harga1,omitempty"`
    Harga2              *float64          `gorm:"default:null" json:"harga2,omitempty"`
    Harga3              *float64          `gorm:"default:null" json:"harga3,omitempty"`
    Status              StatusActivity    `gorm:"not null;default:ON_PROGRESS;index" json:"status"`
    LogAktivitas        []LogBoq          `gorm:"serializer:json;default:'[]'" json:"logs"`
    Dokumen             []PenawaranDokumen `gorm:"foreignKey:PenyusunanBoQID" json:"dokumen,omitempty"`
    CreatedAt           time.Time         `gorm:"index" json:"createdAt"`
    UpdatedAt           time.Time         ` json:"updatedAt"`
}
// ─── Step 3: Review Internal ──────────────────────────────────────────────────

type ReviewInternal struct {
	ID                  string                 `gorm:"primaryKey"                                   json:"id"`
	TrackingPenawaranID string                 `gorm:"not null;uniqueIndex;index"                   json:"trackingPenawaranId"`
	TrackingPenawaran   TrackingPenawaran      `gorm:"foreignKey:TrackingPenawaranID;references:ID" json:"trackingPenawaran,omitempty"`
	ActivityAdminID     *string                `gorm:"index"                                        json:"activityAdminId,omitempty"`
	ActivityAdmin       *Activity              `gorm:"foreignKey:ActivityAdminID;references:ID"     json:"activityAdmin,omitempty"`
	AccAdminDirektur    bool                   `gorm:"not null;default:false"                       json:"accAdminDirektur"`
	AccManajerOps       bool                   `gorm:"not null;default:false"                       json:"accManajerOps"`
	Status              StatusActivity         `gorm:"not null;default:ON_PROGRESS;index"           json:"status"`
	LogAktivitas        []LogReviewInternal    `gorm:"serializer:json;default:'[]'"                 json:"logs"`
	Dokumen             []PenawaranDokumen     `gorm:"foreignKey:ReviewInternalID"                  json:"dokumen,omitempty"`
	CreatedAt           time.Time              `gorm:"index"                                        json:"createdAt"`
	UpdatedAt           time.Time              `                                                      json:"updatedAt"`
}

type PersetujuanManajemen struct {
    ID                   string                     `gorm:"primaryKey"                                   json:"id"`
    TrackingPenawaranID  string                     `gorm:"not null;uniqueIndex;index"                   json:"trackingPenawaranId"`
    TrackingPenawaran    TrackingPenawaran          `gorm:"foreignKey:TrackingPenawaranID;references:ID" json:"trackingPenawaran,omitempty"`
    AccDirekturKomisaris bool                       `gorm:"not null;default:false"                       json:"accDirekturKomisaris"`
    Status               StatusActivity             `gorm:"not null;default:ON_PROGRESS;index"           json:"status"`
    LogAktivitas         []LogPersetujuanManajemen  `gorm:"serializer:json;default:'[]'"                 json:"logs"`
    Dokumen              []PenawaranDokumen         `gorm:"foreignKey:PersetujuanManajemenID"            json:"dokumen,omitempty"`
    ActivityAdminID      *string                    `gorm:"index"                                        json:"activityAdminId,omitempty"`
    ActivityAdmin        *Activity                  `gorm:"foreignKey:ActivityAdminID;references:ID"     json:"activityAdmin,omitempty"`
    CreatedAt            time.Time                  `gorm:"index"                                        json:"createdAt"`
    UpdatedAt            time.Time                  `                                                     json:"updatedAt"`
}

// ─── Dokumen ──────────────────────────────────────────────────────────────────

type PenawaranDokumen struct {
	ID                     string    `gorm:"primaryKey"     json:"id"`
	NamaFile               string    `gorm:"not null"       json:"namaFile"`
	Path                   string    `gorm:"not null"       json:"path"`
	Kategori               string    `gorm:"size:100;default:''" json:"kategori"`
	UploadedBy             string    `gorm:"not null;index" json:"uploadedBy"`
	Pegawai                Pegawai   `gorm:"foreignKey:UploadedBy;references:ID"          json:"pegawai,omitempty"`

	PermintaanMasukID      *string   `gorm:"index" json:"permintaanMasukId,omitempty"`
	PenyusunanBoQID        *string   `gorm:"index" json:"penyusunanBoQId,omitempty"`
	ReviewInternalID       *string   `gorm:"index" json:"reviewInternalId,omitempty"`
	PersetujuanManajemenID *string   `gorm:"index" json:"persetujuanManajemenId,omitempty"`
	FollowUpID             *string   `gorm:"index" json:"followUpId,omitempty"`
	ImplementasiID         *string   `gorm:"index" json:"implementasiId,omitempty"`

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

type TerminPembayaran struct {
	ID                  string            `gorm:"primaryKey"                                   json:"id"`
	TrackingPenawaranID string            `gorm:"not null;uniqueIndex;index"                   json:"trackingPenawaranId"`
	TrackingPenawaran   TrackingPenawaran `gorm:"foreignKey:TrackingPenawaranID;references:ID" json:"trackingPenawaran,omitempty"`
	CreatedBy           string            `gorm:"not null;index"                               json:"createdBy"`
	Pegawai             Pegawai           `gorm:"foreignKey:CreatedBy;references:ID"           json:"pegawai,omitempty"`
	Status              StatusActivity    `gorm:"not null;default:ON_PROGRESS;index"           json:"status"`
	Items               []ItemTermin      `gorm:"foreignKey:TerminPembayaranID"                json:"items,omitempty"`
	CreatedAt           time.Time         `gorm:"index"                                        json:"createdAt"`
	UpdatedAt           time.Time         `                                                     json:"updatedAt"`
}

type ItemTermin struct {
	ID                string    `gorm:"primaryKey"         json:"id"`
	TerminPembayaranID string   `gorm:"not null;index"     json:"terminPembayaranId"`
	Index             int       `gorm:"not null"           json:"index"`
	NamaTermin        string    `gorm:"not null"           json:"namaTermin"`
	Persentase        float64   `gorm:"not null"           json:"persentase"`
	SudahDibayar      bool      `gorm:"not null;default:false" json:"sudahDibayar"`
	TanggalDibayar    *time.Time `                          json:"tanggalDibayar,omitempty"`
	Keterangan        *string   `                          json:"keterangan,omitempty"`
	Deadline          *time.Time `gorm:"index"              json:"deadline,omitempty"`
	CreatedAt         time.Time `gorm:"index"              json:"createdAt"`
	UpdatedAt         time.Time `                          json:"updatedAt"`
}

func (TerminPembayaran) TableName() string { return "TerminPembayaran" }
func (ItemTermin) TableName() string       { return "ItemTermin" }

// ─── Step 5: Follow Up ────────────────────────────────────────────────────────

type LogFollowUp struct {
	Aksi        string    `json:"aksi"`
	Keterangan  string    `json:"keterangan"`
	PegawaiID   string    `json:"pegawaiId"`
	NamaPegawai string    `json:"namaPegawai"`
	CreatedAt   time.Time `json:"createdAt"`
}

type FollowUp struct {
	ID                  string            `gorm:"primaryKey"                                   json:"id"`
	TrackingPenawaranID string            `gorm:"not null;uniqueIndex;index"                   json:"trackingPenawaranId"`
	TrackingPenawaran   TrackingPenawaran `gorm:"foreignKey:TrackingPenawaranID;references:ID" json:"trackingPenawaran,omitempty"`
	AdminID             *string           `gorm:"index"                                        json:"adminId,omitempty"`
	Admin               *Pegawai          `gorm:"foreignKey:AdminID;references:ID"             json:"admin,omitempty"`
	ActivityAdminID     *string           `gorm:"index"                                        json:"activityAdminId,omitempty"`
	ActivityAdmin       *Activity         `gorm:"foreignKey:ActivityAdminID;references:ID"     json:"activityAdmin,omitempty"`
	SalesID             *string           `gorm:"index"                                        json:"salesId,omitempty"`
	Sales               *Pegawai          `gorm:"foreignKey:SalesID;references:ID"             json:"sales,omitempty"`
	ActivitySalesID     *string           `gorm:"index"                                        json:"activitySalesId,omitempty"`
	ActivitySales       *Activity         `gorm:"foreignKey:ActivitySalesID;references:ID"     json:"activitySales,omitempty"`
	ActivityAdminProyekID *string         `gorm:"index"                                        json:"activityAdminProyekId,omitempty"`
	ActivityAdminProyek *Activity         `gorm:"foreignKey:ActivityAdminProyekID;references:ID" json:"activityAdminProyek,omitempty"`
	Status              StatusActivity    `gorm:"not null;default:'ON_PROGRESS'"               json:"status"`
	Stage               int               `gorm:"not null;default:1"                           json:"stage"`
	TotalBAST 			*int 			  `gorm:"index" 									   json:"totalBast,omitempty"`
	LogAktivitas        []LogFollowUp     `gorm:"serializer:json;default:'[]'"                 json:"logs"`
	Dokumen             []PenawaranDokumen `gorm:"foreignKey:FollowUpID"                       json:"dokumen,omitempty"`
	CreatedAt           time.Time         `gorm:"index"                                        json:"createdAt"`
	UpdatedAt           time.Time         `                                                     json:"updatedAt"`
}

// ─── Table Names ──────────────────────────────────────────────────────────────

func (TrackingPenawaran) TableName() string    { return "TrackingPenawaran" }
func (PermintaanMasuk) TableName() string      { return "PermintaanMasuk" }
func (PenyusunanBoQ) TableName() string        { return "PenyusunanBoQ" }
func (ReviewInternal) TableName() string       { return "ReviewInternal" }
func (PersetujuanManajemen) TableName() string { return "PersetujuanManajemen" }
func (FollowUp) TableName() string             { return "FollowUp" }
func (PenawaranDokumen) TableName() string     { return "PenawaranDokumen" }
func (PenawaranChat) TableName() string        { return "PenawaranChat" }
func (Implementasi) TableName() string         { return "Implementasi" }
func (ImplementasiBarang) TableName() string   { return "ImplementasiBarang" }

// ─── Step 6 Models ────────────────────────────────────────────────────────────

type LogImplementasi struct {
	Aksi        string    `json:"aksi"`
	Keterangan  string    `json:"keterangan"`
	PegawaiID   string    `json:"pegawaiId"`
	NamaPegawai string    `json:"namaPegawai"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Implementasi struct {
	ID                  string            `gorm:"primaryKey"                                   json:"id"`
	TrackingPenawaranID string            `gorm:"not null;uniqueIndex;index"                   json:"trackingPenawaranId"`
	TrackingPenawaran   TrackingPenawaran `gorm:"foreignKey:TrackingPenawaranID;references:ID" json:"trackingPenawaran,omitempty"`
	NoPO                string            `gorm:"default:''"                                   json:"noPO"`
	TanggalPO           *time.Time        `                                                    json:"tanggalPO,omitempty"`
	NoWO                string            `gorm:"default:''"                                   json:"noWO"`
	TanggalWO           *time.Time        `                                                    json:"tanggalWO,omitempty"`
	NoDO                string            `gorm:"default:''"                                   json:"noDO"`
	TanggalDO           *time.Time        `                                                    json:"tanggalDO,omitempty"`
	WaktuPengerjaan     *time.Time        `                                                    json:"waktuPengerjaan,omitempty"`
	Status              StatusActivity    `gorm:"not null;default:ON_PROGRESS;index"           json:"status"`
	LogAktivitas        []LogImplementasi `gorm:"serializer:json;default:'[]'"                 json:"logs"`
	Barang              []ImplementasiBarang `gorm:"foreignKey:ImplementasiID"                json:"barang,omitempty"`
	Dokumen             []PenawaranDokumen `gorm:"foreignKey:ImplementasiID"                  json:"dokumen,omitempty"`

	// Daily Activities
	ActivityPembelianID    *string           `gorm:"index"                                        json:"activityPembelianId,omitempty"`
	ActivityPembelian      *Activity         `gorm:"foreignKey:ActivityPembelianID;references:ID"  json:"activityPembelian,omitempty"`
	ActivityPengantaranID  *string           `gorm:"index"                                        json:"activityPengantaranId,omitempty"`
	ActivityPengantaran    *Activity         `gorm:"foreignKey:ActivityPengantaranID;references:ID" json:"activityPengantaran,omitempty"`
	ActivityInstalasiID    *string           `gorm:"index"                                        json:"activityInstalasiId,omitempty"`
	ActivityInstalasi      *Activity         `gorm:"foreignKey:ActivityInstalasiID;references:ID"  json:"activityInstalasi,omitempty"`

	CreatedAt           time.Time         `gorm:"index"                                        json:"createdAt"`
	UpdatedAt           time.Time         `                                                     json:"updatedAt"`
}

type ImplementasiBarang struct {
	ID                  string     `gorm:"primaryKey"         json:"id"`
	ImplementasiID      string     `gorm:"not null;index"     json:"implementasiId"`
	NamaBarang          string     `gorm:"not null"           json:"namaBarang"`
	Status              string     `gorm:"not null"           json:"status"` // "Ready" | "Perlu Beli | "Indent | "PO" | "Pending" | "Pengiriman" 
	Qty                 float64    `gorm:"not null"           json:"qty"`
	Satuan              string     `gorm:"not null"           json:"satuan"`
	HargaSatuan         float64    `gorm:"not null"           json:"hargaSatuan"`
	Metode              string     `gorm:"not null"           json:"metode"`
	EstimasiKedatangan  *time.Time `                         json:"estimasiKedatangan,omitempty"`
	CreatedAt           time.Time  `gorm:"index"              json:"createdAt"`
	UpdatedAt           time.Time  `                          json:"updatedAt"`
}

// ─── Log BAST ─────────────────────────────────────────────────────────────────

type LogBast struct {
	Aksi        string    `json:"aksi"`
	Keterangan  string    `json:"keterangan"`
	PegawaiID   string    `json:"pegawaiId"`
	NamaPegawai string    `json:"namaPegawai"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ─── BAST (Berita Acara Serah Terima) ──────────────────────────────────────────

type Bast struct {
	ID                  string            `gorm:"primaryKey"                                   json:"id"`
	TrackingPenawaranID string            `gorm:"not null;uniqueIndex;index"                   json:"trackingPenawaranId"`
	TrackingPenawaran   TrackingPenawaran `gorm:"foreignKey:TrackingPenawaranID;references:ID" json:"trackingPenawaran,omitempty"`

	Status       StatusActivity `gorm:"not null;default:ON_PROGRESS;index" json:"status"`
	LogAktivitas []LogBast      `gorm:"serializer:json;default:'[]'"       json:"logs"`

	Entries []BastEntry `gorm:"foreignKey:BastID" json:"entries,omitempty"`

	CreatedAt time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time `             json:"updatedAt"`
}

type BastEntry struct {
	ID     string `gorm:"primaryKey"     json:"id"`
	BastID string `gorm:"not null;index" json:"bastId"`
	Bast   Bast   `gorm:"foreignKey:BastID;references:ID" json:"-"`

	NoReferensi        string     `gorm:"default:''" json:"noReferensi"`
	TanggalTerbit      *time.Time `                   json:"tanggalTerbit,omitempty"`
	TanggalSerahTerima *time.Time `                   json:"tanggalSerahTerima,omitempty"`

	ActivityAdminProyekID *string   `gorm:"index"                                          json:"activityAdminProyekId,omitempty"`
	ActivityAdminProyek   *Activity `gorm:"foreignKey:ActivityAdminProyekID;references:ID" json:"activityAdminProyek,omitempty"`

	CreatedAt time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time `             json:"updatedAt"`
}

func (Bast) TableName() string      { return "Bast" }
func (BastEntry) TableName() string { return "BastEntry" }

// ─── Hooks ────────────────────────────────────────────────────────────────────

func (p *PermintaanMasuk) BeforeCreate(tx *gorm.DB) error { return nil }
