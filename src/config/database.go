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
		&models.PenawaranDokumen{},
		&models.PenawaranChat{},
		&models.TerminPembayaran{},
		&models.ItemTermin{},
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
	}

	for _, sql := range indexes {
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	return nil
}
