package models

import (
	"time"

	"gorm.io/gorm"
)

// ==========================================
// ENUMS (tidak diubah)
// ==========================================

type Role string
type Divisi string

const (
	RoleMaster    Role = "MASTER"
	RoleSupervisi Role = "SUPERVISI"
	RoleProjek    Role = "PROJEK"
	RoleKaryawan  Role = "KARYAWAN"
)

const (
	DivisiKomisaris          Divisi = "KOMISARIS"
	DivisiDirektur           Divisi = "DIREKTUR"
	DivisiSekertaris         Divisi = "SEKERTARIS"
	DivisiAdminSekertariat   Divisi = "ADMIN_SEKERTARIAT"
	DivisiManagerOperasional Divisi = "MANAGER_OPERASIONAL"
	DivisiMonitoringControl  Divisi = "MONITORING_CONTROL_ADVISOR"
	DivisiProcurementGA      Divisi = "PROCUREMENT_GA"
	DivisiFinanceAccounting  Divisi = "FINANCE_ACCOUNTING"
	DivisiIT                 Divisi = "IT"
	DivisiSales              Divisi = "SALES"
	DivisiPresales           Divisi = "PRESALES"
	DivisiSekertariat        Divisi = "SEKERTARIAT"
	DivisiTechnicalSupport   Divisi = "TECHNICAL_SUPPORT"
	DivisiMaintenanceFire    Divisi = "MAINTENANCE_FIRE"
	DivisiMaintenancePAC     Divisi = "MAINTENANCE_PAC"
	DivisiCustomerCare       Divisi = "CUSTOMER_CARE"
	DivisiTechnician         Divisi = "TECHNICIAN"
)

type StatusActivity string
type StatusReschedule string
type KategoriActivity string

const (
	StatusOnProgress        StatusActivity = "ON_PROGRESS"
	StatusPending           StatusActivity = "PENDING"
	StatusPendingPegawai    StatusActivity = "PENDING_PEGAWAI"
	StatusDiterima          StatusActivity = "DITERIMA"
	StatusKonfirmasiSelesai StatusActivity = "KONFIRMASI_SELESAI"
	StatusDitolak           StatusActivity = "DITOLAK"
	StatusDibatalkan        StatusActivity = "DIBATALKAN"
	StatusPerluTindakan StatusActivity = "PERLU_TINDAKAN"
	StatusSelesai       StatusActivity = "SELESAI"

)

const (
	StatusReschedulePending  StatusReschedule = "PENDING"
	StatusRescheduleDiterima StatusReschedule = "DITERIMA"
	StatusRescheduleDitolak  StatusReschedule = "DITOLAK"
)

const (
	KategoriQuotation           KategoriActivity = "QUOTATION"
	KategoriDokumentasi         KategoriActivity = "DOKUMENTASI"
	KategoriReportProject       KategoriActivity = "REPORT_PROJECT"
	KategoriDrawing             KategoriActivity = "DRAWING"
	KategoriKurvaS              KategoriActivity = "KURVA_S"
	KategoriMSProject           KategoriActivity = "MS_PROJECT"
	KategoriMonitorProgress     KategoriActivity = "MONITOR_PROGRESS"
	KategoriMonitorProject      KategoriActivity = "MONITOR_PROJECT"
	KategoriBillOfQuantity      KategoriActivity = "BILL_OF_QUANTITY"
	KategoriAkomodasiProject    KategoriActivity = "AKOMODASI_PROJECT"
	KategoriKoordinasiEksternal KategoriActivity = "KOORDINASI_EKSTERNAL"
	KategoriDokumenPendukung    KategoriActivity = "DOKUMEN_PENDUKUNG"
	KategoriWorkOrder           KategoriActivity = "WORK_ORDER"
	KategoriApprovalUser        KategoriActivity = "APPROVAL_USER"
	KategoriTechnicalAdvice     KategoriActivity = "TECHNICAL_ADVICE"
	KategoriLainLain            KategoriActivity = "LAIN_LAIN"
)

// ==========================================
// MODELS
// ==========================================

type Pegawai struct {
	ID        string     `gorm:"primaryKey" json:"id"`
	Nama      string     `gorm:"not null;index" json:"nama"`   // + index: sering dicari by nama
	Divisi    Divisi     `gorm:"not null;index" json:"divisi"` // + index: sering difilter by divisi
	User      *User      `gorm:"foreignKey:PegawaiID" json:"user,omitempty"`
	DeletedAt *time.Time `gorm:"index" json:"deletedAt,omitempty"` // + index: soft delete filter
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

type User struct {
	ID           string     `gorm:"primaryKey" json:"id"`
	Email        string     `gorm:"uniqueIndex;not null" json:"email"`
	Password     string     `gorm:"not null" json:"-"`
	Role         Role       `gorm:"default:KARYAWAN;not null;index" json:"role"` // + index: filter by role
	PegawaiID    string     `gorm:"uniqueIndex;not null" json:"pegawaiId"`
	Pegawai      Pegawai    `gorm:"foreignKey:PegawaiID" json:"pegawai,omitempty"`
	LastLogin    *time.Time `json:"lastLogin,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	ResetToken   string     `json:"-"`
	TokenExpires *time.Time `json:"-"`
	ActiveStatus *bool      `gorm:"column:active_status;default:true" json:"activeStatus"`
	BerjalanCount int64     `gorm:"-" json:"berjalanCount"`
}

type Activity struct {
	ID                     string                `gorm:"primaryKey" json:"id"`
	PegawaiID              string                `gorm:"not null;index" json:"pegawaiId"` // + index: list activity by pegawai
	Pegawai                Pegawai               `gorm:"foreignKey:PegawaiID;references:ID" json:"pegawai,omitempty"`
	ParentID               *string               `gorm:"index" json:"parentId,omitempty"` // + index: ambil children
	Parent                 *Activity             `gorm:"foreignKey:ParentID;references:ID" json:"parent,omitempty"`
	TerkaitPO              *string               `gorm:"index" json:"terkaitPO,omitempty"` // + index: filter by PO
	Perusahaan             *string               `gorm:"index" json:"perusahaan,omitempty"`
	Kategori               KategoriActivity      `gorm:"not null;index" json:"kategori"` // + index: filter by kategori
	Judul                  string                `gorm:"not null" json:"judul"`
	Deskripsi              string                `gorm:"not null" json:"deskripsi"`
	WaktuMulai             time.Time             `gorm:"not null;index" json:"waktuMulai"`    // + index: sort/range query
	TargetSelesai          time.Time             `gorm:"not null;index" json:"targetSelesai"` // + index: overdue detection
	WaktuSubmit            *time.Time            `json:"waktuSubmit,omitempty"`
	Status                 StatusActivity        `gorm:"default:ON_PROGRESS;not null;index" json:"status"` // + index: filter by status
	IsKonfirmasiKolaborasi bool                  `gorm:"default:false;not null" json:"isKonfirmasiKolaborasi"`
	Kolaborator            []ActivityKolaborator `gorm:"foreignKey:ActivityID" json:"kolaborator,omitempty"`
	Dokumen                []ActivityDokumen     `gorm:"foreignKey:ActivityID" json:"dokumen,omitempty"`
	Reschedule             []ActivityReschedule  `gorm:"foreignKey:ActivityID" json:"reschedule,omitempty"`
	Chat                   []ActivityChat        `gorm:"foreignKey:ActivityID" json:"chat,omitempty"`
	Children               []Activity            `gorm:"foreignKey:ParentID" json:"children,omitempty"`
	CreatedAt              time.Time             `gorm:"index" json:"createdAt"` // + index: sort by created
	UpdatedAt              time.Time             `json:"updatedAt"`
	AlasanPenolakan        *string               `gorm:"type:text" json:"alasanPenolakan,omitempty"`
	NilaiKPI               *NilaiKPI             `gorm:"type:string;index" json:"nilaiKPI,omitempty"`
	IsSupervised           bool                  `gorm:"default:false;not null" json:"isSupervised"`
}

// Index komposit untuk query yang paling sering: activity by pegawai + status (perlu tindakan, on progress, dll)
func (Activity) TableName() string                 { return "Activity" }
func (a *Activity) BeforeCreate(tx *gorm.DB) error { return nil } // placeholder jika perlu hook nanti

type ActivityKolaborator struct {
	ID              string         `gorm:"primaryKey" json:"id"`
	ActivityID      string         `gorm:"not null;index" json:"activityId"` // + index: ambil kolaborator by activity
	PegawaiID       string         `gorm:"not null;index" json:"pegawaiId"`  // + index: ambil activity kolaborasi by pegawai
	Pegawai         Pegawai        `gorm:"foreignKey:PegawaiID;references:ID" json:"pegawai,omitempty"`
	ChildActivityID *string        `gorm:"index" json:"childActivityId,omitempty"` // + index: join ke child activity
	ChildActivity   *Activity      `gorm:"foreignKey:ChildActivityID;references:ID" json:"childActivity,omitempty"`
	Judul           string         `gorm:"not null" json:"judul"`
	Status          StatusActivity `gorm:"default:ON_PROGRESS;not null;index" json:"status"` // + index: filter status kolaborator
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

type ActivityReschedule struct {
	ID                string           `gorm:"primaryKey" json:"id"`
	ActivityID        string           `gorm:"not null;index" json:"activityId"` // + index: ambil reschedule by activity
	TargetSelesaiBaru time.Time        `gorm:"not null" json:"targetSelesaiBaru"`
	Alasan            string           `gorm:"not null" json:"alasan"`
	Status            StatusReschedule `gorm:"default:PENDING;not null;index" json:"status"` // + index: filter pending reschedule
	AlasanPenolakan   *string          `json:"alasanPenolakan,omitempty"`
	CreatedAt         time.Time        `gorm:"index" json:"createdAt"` // + index: sort by latest
	UpdatedAt         time.Time        `json:"updatedAt"`
}

type ActivityDokumen struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	ActivityID string    `gorm:"not null;index" json:"activityId"` // + index: ambil dokumen by activity
	NamaFile   string    `gorm:"not null" json:"namaFile"`
	Path       string    `gorm:"not null" json:"path"`
	UploadedBy string    `gorm:"not null;index" json:"uploadedBy"` // + index: filter dokumen by uploader
	Pegawai    Pegawai   `gorm:"foreignKey:UploadedBy;references:ID" json:"pegawai,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ActivityChat struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	ActivityID string    `gorm:"not null;index" json:"activityId"` // + index: load chat by activity
	PegawaiID  string    `gorm:"not null;index" json:"pegawaiId"`  // + index: chat by pegawai
	Pegawai    Pegawai   `gorm:"foreignKey:PegawaiID;references:ID" json:"pegawai,omitempty"`
	Pesan      string    `gorm:"not null" json:"pesan"`
	ReadBy     []string  `gorm:"serializer:json;default:'[]'" json:"readBy"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"` // + index: sort chat chronologically
}

type Notifikasi struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	PegawaiID  string    `gorm:"not null;index" json:"pegawaiId"` // + index: notif by pegawai (paling sering)
	Pegawai    Pegawai   `gorm:"foreignKey:PegawaiID;references:ID" json:"pegawai,omitempty"`
	ActivityID *string   `gorm:"index" json:"activityId,omitempty"` // + index: notif by activity
	Activity   *Activity `gorm:"foreignKey:ActivityID;references:ID" json:"activity,omitempty"`
	Judul      string    `gorm:"not null" json:"judul"`
	Pesan      string    `gorm:"not null" json:"pesan"`
	IsRead     bool      `gorm:"default:false;not null;index" json:"isRead"` // + index: filter unread notif
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`                     // + index: sort notif terbaru
}

type Perusahaan struct {
	ID           string         `gorm:"primaryKey" json:"id"`
	Nama         string         `gorm:"not null" json:"nama"`
	Alamat       *string        `json:"alamat"`
	NomorTelepon *string        `gorm:"column:nomor_telepon" json:"nomor_telepon"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

func (Pegawai) TableName() string             { return "Pegawai" }
func (User) TableName() string                { return "User" }
func (ActivityKolaborator) TableName() string { return "ActivityKolaborator" }
func (ActivityReschedule) TableName() string  { return "ActivityReschedule" }
func (ActivityDokumen) TableName() string     { return "ActivityDokumen" }
func (ActivityChat) TableName() string        { return "ActivityChat" }
func (Notifikasi) TableName() string          { return "Notifikasi" }
func (Perusahaan) TableName() string          { return "Perusahaan" }
