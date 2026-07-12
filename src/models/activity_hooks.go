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

	if a.Kategori != KategoriQuotation {
		fmt.Println(">>> Bukan QUOTATION, skip")
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

	fmt.Println(">>> Daily selesai, Review Internal langsung selesai")

	// Review Internal selesai
	review.AccAdminDirektur = true
	review.AccManajerOps = true
	review.Status = StatusSelesai
	appendReviewInternalLogDirect(&review, "Review Internal Selesai", "Otomatis selesai setelah daily Pengecekan Penawaran selesai", a.PegawaiID, namaPegawai)
	tx.Save(&review)

	// Update TrackingPenawaran ke step berikutnya
	tx.Model(&TrackingPenawaran{}).
		Where("id = ?", review.TrackingPenawaranID).
		Updates(map[string]interface{}{
			"step_saat_ini": StepPersetujuanManajemen,
			"status":        StatusOnProgress,
		})

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
		tx.Create(&persetujuan)
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