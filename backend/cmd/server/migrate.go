package main

import (
	"log/slog"

	"gbevent/internal/model"

	"gorm.io/gorm"
)

// migrateLegacySchema 处理旧版本库结构到团体报名模块的兼容升级：
// 早期 registrations 表在 (activity_id, user_id) 上有唯一索引，团体报名会让同一用户
// 产生多行记录，因此需要把该唯一索引降级为普通索引（AutoMigrate 不会自动变更既有索引）。
func migrateLegacySchema(db *gorm.DB, logger *slog.Logger) error {
	const legacyIndex = "uk_registrations_activity_user"
	var cnt int64
	err := db.Raw("SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?",
		model.Registration{}.TableName(), legacyIndex).Scan(&cnt).Error
	if err != nil {
		return err
	}
	if cnt > 0 {
		if err := db.Migrator().DropIndex(&model.Registration{}, legacyIndex); err != nil {
			return err
		}
		logger.Info("legacy unique index dropped for group registration", "index", legacyIndex)
	}
	// 重新声明同名普通（非唯一）复合索引；AutoMigrate 也会幂等创建。
	if err := db.AutoMigrate(&model.Registration{}); err != nil {
		return err
	}
	return nil
}
