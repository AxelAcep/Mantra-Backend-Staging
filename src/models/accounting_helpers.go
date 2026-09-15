package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Termin pembayaran jalan berurutan mirip bulan Garansi: cuma termin pertama
// yang langsung dapet daily begitu Accounting dibuat, termin berikutnya baru
// dibuatin daily-nya setelah termin sebelumnya tuntas — SudahDibayar DAN
// daily-nya DITERIMA, dua-duanya wajib.

// CreateItemTerminActivity bikin daily Activity buat satu ItemTermin dan
// nyimpen ActivityID-nya. Dipanggil saat Accounting dibuat (termin ke-1)
// maupun otomatis dari AdvanceTerminIfReady (termin ke-2 dst).
func CreateItemTerminActivity(tx *gorm.DB, item *ItemTermin, picID, nomorPenawaran string) error {
	now := time.Now()
	deadline := now.Add(7 * 24 * time.Hour)
	if item.Deadline != nil {
		deadline = *item.Deadline
	}

	activity := Activity{
		ID:            uuid.New().String(),
		PegawaiID:     picID,
		Kategori:      KategoriAkomodasiProject,
		Judul:         fmt.Sprintf("Penagihan Termin %d - %s", item.Index, item.NamaTermin),
		Deskripsi:     fmt.Sprintf("Activity otomatis penagihan termin pembayaran #%d (%s) untuk penawaran %s", item.Index, item.NamaTermin, nomorPenawaran),
		WaktuMulai:    now,
		TargetSelesai: deadline,
		Status:        StatusOnProgress,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := tx.Create(&activity).Error; err != nil {
		fmt.Println(">>> Gagal membuat Activity Termin:", err)
		return err
	}

	item.ActivityID = &activity.ID
	if err := tx.Model(&ItemTermin{}).Where("id = ?", item.ID).
		Update("activity_id", item.ActivityID).Error; err != nil {
		fmt.Println(">>> Gagal update ItemTermin.ActivityID:", err)
		return err
	}

	fmt.Println(">>> Daily Termin dibuat:", activity.ID, "termin ke:", item.Index)
	return nil
}

// AdvanceTerminIfReady dipanggil dari 2 titik: (1) saat item ditandai
// SudahDibayar, (2) saat daily-nya DITERIMA — karena urutan approval &
// pembayaran bisa kejadian di urutan mana pun. Kalau dua-duanya (SudahDibayar
// & ActivitySelesai) udah lengkap, baru bikin daily termin berikutnya (atau
// tandai TerminPembayaran SELESAI kalau ini termin terakhir).
func AdvanceTerminIfReady(tx *gorm.DB, item *ItemTermin) error {
	if !item.SudahDibayar || !item.ActivitySelesai {
		fmt.Println(">>> ItemTermin belum siap lanjut ke termin berikutnya:", item.ID)
		return nil
	}

	var termin TerminPembayaran
	if err := tx.Preload("TrackingPenawaran").Where("id = ?", item.TerminPembayaranID).First(&termin).Error; err != nil {
		fmt.Println(">>> TerminPembayaran tidak ditemukan:", err)
		return nil
	}

	var nextItem ItemTermin
	err := tx.Where("termin_pembayaran_id = ? AND index = ?", item.TerminPembayaranID, item.Index+1).First(&nextItem).Error
	if err != nil {
		// Gak ada termin berikutnya -> ini termin terakhir, TerminPembayaran tuntas.
		fmt.Println(">>> Termin terakhir tuntas, TerminPembayaran SELESAI:", termin.ID)
		return tx.Model(&TerminPembayaran{}).Where("id = ?", termin.ID).Update("status", StatusSelesai).Error
	}

	// Guard idempotency: kalau termin berikutnya udah punya daily, jangan dobel.
	if nextItem.ActivityID != nil && *nextItem.ActivityID != "" {
		fmt.Println(">>> Daily termin berikutnya udah ada, skip:", *nextItem.ActivityID)
		return nil
	}

	return CreateItemTerminActivity(tx, &nextItem, termin.CreatedBy, termin.TrackingPenawaran.NomorPenawaran)
}
