package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"mantra/src/models"
)

// Cleanup SATU KALI PAKAI: reset FollowUp.ActivityAdminProyekID yang salah
// kepasang sama dengan ActivityAdminID (sisa fallback lama yang udah
// dihapus). Cuma nyentuh kolom activity_admin_proyek_id + log_aktivitas di
// tabel FollowUp — TIDAK menghapus/mengubah Activity/daily apa pun.
// Guard: hanya baris yang Stage < 3 (dijamin belum pernah lewat
// AssignAdminProyek beneran, karena endpoint itu selalu bump Stage ke 3).
func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL kosong")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatal("gagal konek DB:", err)
	}

	var rows []models.FollowUp
	err = db.
		Where(`activity_admin_id IS NOT NULL AND activity_admin_id = activity_admin_proyek_id AND stage < 3`).
		Find(&rows).Error
	if err != nil {
		log.Fatal("query error:", err)
	}

	fmt.Println("Jumlah FollowUp yang mau dibersihin:", len(rows))

	now := time.Now()
	for _, f := range rows {
		f.LogAktivitas = append(f.LogAktivitas, models.LogFollowUp{
			Aksi:        "Cleanup Data",
			Keterangan:  "activity_admin_proyek_id direset (dikosongkan) karena kepasang sama dengan activity_admin_id secara keliru (sisa fallback lama yang sudah dihapus). Daily Admin Sekretariat & datanya tidak diubah/dihapus.",
			PegawaiID:   "SYSTEM",
			NamaPegawai: "System Cleanup",
			CreatedAt:   now,
		})

		// Select paksa activity_admin_proyek_id ikut ke-UPDATE walau nilainya
		// zero-value (nil) di struct literal ini — itu yang bikin kolomnya
		// jadi NULL. log_aktivitas ikut lewat Select juga biar serializer:json
		// ke-apply (bukan pakai Update(kolom, value) tunggal, lihat catatan
		// bug serupa di garansi_controller.go/bast_controller.go).
		err := db.Model(&models.FollowUp{}).
			Where("id = ?", f.ID).
			Select("activity_admin_proyek_id", "log_aktivitas").
			Updates(models.FollowUp{LogAktivitas: f.LogAktivitas}).Error
		if err != nil {
			fmt.Println("GAGAL update FollowUp", f.ID, ":", err)
			continue
		}
		fmt.Println("OK dibersihin:", f.ID, "tracking:", f.TrackingPenawaranID)
	}

	fmt.Println("Selesai.")
}
