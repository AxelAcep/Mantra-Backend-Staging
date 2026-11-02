package config

import (
	"log"
	"mantra/src/models"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func ConnectDB() {
	dsn := os.Getenv("DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatal("Gagal koneksi ke database:", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("Gagal ambil sql.DB:", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	err = db.AutoMigrate(
		&models.Pegawai{},
		&models.User{},
		&models.Activity{},
		&models.ActivityKolaborator{},
		&models.ActivityReschedule{},
		&models.ActivityDokumen{},
		&models.ActivityChat{},
		&models.Notifikasi{},
		&models.KPIPegawai{},
		&models.Perusahaan{},
		&models.TrackingPenawaran{},
		&models.PermintaanMasuk{},
		&models.PenyusunanBoQ{},
		&models.ReviewInternal{},
		&models.PersetujuanManajemen{},
		&models.FollowUp{},
		&models.Implementasi{},
		&models.ImplementasiBarang{},
		&models.PenawaranDokumen{},
		&models.PenawaranChat{},
		&models.TerminPembayaran{},
		&models.ItemTermin{},
		&models.Bast{},
		&models.BastEntry{},
		&models.Garansi{},
		&models.GaransiMonth{},
		&models.Barang{},
	)

	if err != nil {
		log.Fatal("Gagal migrasi database:", err)
	}

	// Composite indexes (IF NOT EXISTS = aman dijalankan berulang kali)
	if err := createCompositeIndexes(db); err != nil {
		log.Println("Warning: gagal membuat composite index:", err)
	}

	DB = db
	log.Println("Database terhubung & migrasi selesai!")
}

func createCompositeIndexes(db *gorm.DB) error {
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_activity_pegawai_status ON "Activity" ("pegawai_id", "status")`,
		`CREATE INDEX IF NOT EXISTS idx_activity_overdue ON "Activity" ("status", "target_selesai")`,
		`CREATE INDEX IF NOT EXISTS idx_notifikasi_pegawai_read ON "Notifikasi" ("pegawai_id", "is_read")`,
		`DROP INDEX IF EXISTS idx_kpi_unique`,
		`CREATE UNIQUE INDEX idx_kpi_unique ON "KPIPegawai" ("pegawai_id", "bulan", "tahun", "minggu")`,
		// Cleanup notifikasi aksi pengadaan yang sudah tidak relevan (bug
		// lama: notif "menunggu aksi" tidak dipensiunkan saat aksi selesai).
		// Idempotent — hanya menyentuh baris yang masih unread.

		// Persetujuan Manajemen: tracking sudah melewati step PERSETUJUAN_MANAJEMEN.
		`UPDATE "Notifikasi" SET "is_read" = true
		 WHERE "is_read" = false
		   AND "tipe" = 'PENAWARAN'
		   AND "judul" LIKE 'Persetujuan Manajemen Menunggu Approval%'
		   AND "tracking_penawaran_id" IN (
		     SELECT id FROM "TrackingPenawaran" WHERE "step_saat_ini" <> 'PERSETUJUAN_MANAJEMEN'
		   )`,
		// Review Internal: tracking sudah tidak di step REVIEW_INTERNAL
		// (lanjut ke Persetujuan Manajemen atau mundur ke Penyusunan BoQ).
		// Prefix "Review Internal - " tidak menyentuh "Review Internal Selesai - ".
		`UPDATE "Notifikasi" SET "is_read" = true
		 WHERE "is_read" = false
		   AND "tipe" = 'PENAWARAN'
		   AND "judul" LIKE 'Review Internal - %'
		   AND "tracking_penawaran_id" IN (
		     SELECT id FROM "TrackingPenawaran" WHERE "step_saat_ini" <> 'REVIEW_INTERNAL'
		   )`,
		// Pengisian Harga BoQ: tracking sudah tidak di step PENYUSUNAN_BOQ.
		`UPDATE "Notifikasi" SET "is_read" = true
		 WHERE "is_read" = false
		   AND "tipe" = 'PENAWARAN'
		   AND "judul" LIKE 'Pengisian Harga BoQ%'
		   AND "tracking_penawaran_id" IN (
		     SELECT id FROM "TrackingPenawaran" WHERE "step_saat_ini" <> 'PENYUSUNAN_BOQ'
		   )`,
		// Konfirmasi Dokumen PO: FollowUp tidak lagi menunggu konfirmasi
		// Direktur/Komisaris (stage ≠ 5 — sudah ACC ke 6 atau ditolak balik 4).
		`UPDATE "Notifikasi" SET "is_read" = true
		 WHERE "is_read" = false
		   AND "tipe" = 'PENAWARAN'
		   AND "judul" LIKE 'Konfirmasi Dokumen PO Menunggu%'
		   AND NOT EXISTS (
		     SELECT 1 FROM "FollowUp" f
		     WHERE f."tracking_penawaran_id" = "Notifikasi"."tracking_penawaran_id"
		       AND f."stage" = 5
		   )`,
		// Lengkapi Data Order: data order pada Implementasi sudah lengkap.
		`UPDATE "Notifikasi" SET "is_read" = true
		 WHERE "is_read" = false
		   AND "tipe" = 'PENAWARAN'
		   AND "judul" LIKE 'Lengkapi Data Order%'
		   AND "tracking_penawaran_id" IN (
		     SELECT "tracking_penawaran_id" FROM "Implementasi"
		     WHERE "no_po" <> '' AND "no_wo" <> '' AND "waktu_pengerjaan" IS NOT NULL
		   )`,
		// Siapkan Termin Pembayaran: TerminPembayaran sudah dibuat.
		`UPDATE "Notifikasi" SET "is_read" = true
		 WHERE "is_read" = false
		   AND "tipe" = 'PENAWARAN'
		   AND "judul" LIKE 'Siapkan Termin Pembayaran%'
		   AND "tracking_penawaran_id" IN (
		     SELECT "tracking_penawaran_id" FROM "TerminPembayaran"
		   )`,
		// Konfigurasi Garansi: semua Garansi pada tracking sudah dikonfigurasi
		// (masih ada Garansi = "kuning menunggu konfigurasi" → belum dipensiunkan).
		`UPDATE "Notifikasi" SET "is_read" = true
		 WHERE "is_read" = false
		   AND "tipe" = 'PENAWARAN'
		   AND "judul" LIKE 'Konfigurasi Garansi%'
		   AND EXISTS (
		     SELECT 1 FROM "Garansi" g
		     WHERE g."tracking_penawaran_id" = "Notifikasi"."tracking_penawaran_id"
		   )
		   AND NOT EXISTS (
		     SELECT 1 FROM "Garansi" g
		     WHERE g."tracking_penawaran_id" = "Notifikasi"."tracking_penawaran_id"
		       AND g."status" = 'BELUM_DIKONFIGURASI'
		   )`,
	}

	for i, sql := range indexes {
		if err := db.Exec(sql).Error; err != nil {
			// Lanjut ke statement berikutnya: index IF NOT EXISTS dan
			// cleanup notifikasi bersifat idempotent — satu kegagalan tidak
			// boleh mematikan sisa index/cleanup di belakangnya.
			log.Printf("Warning: statement migrasi ke-%d gagal: %v", i+1, err)
		}
	}
	return nil
}
