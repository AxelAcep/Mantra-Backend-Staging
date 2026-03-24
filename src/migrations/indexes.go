package migrations

import "gorm.io/gorm"

func CreateCompositeIndexes(db *gorm.DB) error {
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_activity_pegawai_status 
            ON "Activity" ("pegawaiId", "status")`,

		`CREATE INDEX IF NOT EXISTS idx_activity_overdue 
            ON "Activity" ("status", "targetSelesai")`,

		`CREATE INDEX IF NOT EXISTS idx_notifikasi_pegawai_read 
            ON "Notifikasi" ("pegawaiId", "isRead")`,
	}

	for _, sql := range indexes {
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	return nil
}
