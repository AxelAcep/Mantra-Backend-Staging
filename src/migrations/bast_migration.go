package migrations

import (
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mantra/src/models"
)

// MigrateBastToEntries memindahkan data lama (noReferensi, tanggalTerbit,
// tanggalSerahTerima, activityAdminProyekId) dari kolom root tabel Bast
// ke tabel BastEntry sebagai entry pertama. Idempotent: skip Bast yang
// sudah punya entries.
func MigrateBastToEntries(db *gorm.DB) error {
	type oldBastRow struct {
		ID                    string
		NoReferensi           string
		TanggalTerbit         *time.Time
		TanggalSerahTerima    *time.Time
		ActivityAdminProyekID *string
		CreatedAt             time.Time
	}

	var oldRows []oldBastRow

	err := db.Table("Bast").
		Select("id, no_referensi, tanggal_terbit, tanggal_serah_terima, activity_admin_proyek_id, created_at").
		Where("no_referensi != '' OR tanggal_terbit IS NOT NULL OR tanggal_serah_terima IS NOT NULL OR activity_admin_proyek_id IS NOT NULL").
		Scan(&oldRows).Error
	if err != nil {
		return err
	}

	migrated := 0
	skipped := 0

	for _, row := range oldRows {
		var count int64
		db.Model(&models.BastEntry{}).Where("bast_id = ?", row.ID).Count(&count)
		if count > 0 {
			skipped++
			continue
		}

		entry := models.BastEntry{
			ID:                    uuid.New().String(),
			BastID:                row.ID,
			NoReferensi:           row.NoReferensi,
			TanggalTerbit:         row.TanggalTerbit,
			TanggalSerahTerima:    row.TanggalSerahTerima,
			ActivityAdminProyekID: row.ActivityAdminProyekID,
			CreatedAt:             row.CreatedAt,
			UpdatedAt:             time.Now(),
		}

		if err := db.Create(&entry).Error; err != nil {
			log.Printf("gagal migrasi Bast ID %s: %v", row.ID, err)
			continue
		}
		migrated++
	}

	log.Printf("MigrateBastToEntries selesai: %d dimigrasi, %d dilewati (sudah ada entries)", migrated, skipped)
	return nil
}