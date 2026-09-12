package repository

import (
	"errors"
	"fmt"

	"gbevent/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RegistrationGroupRepository 团体报名仓储。
type RegistrationGroupRepository struct {
	db *gorm.DB
}

// NewRegistrationGroupRepository 构造团体报名仓储。
func NewRegistrationGroupRepository(db *gorm.DB) *RegistrationGroupRepository {
	return &RegistrationGroupRepository{db: db}
}

// CreateTx 在事务内创建团体报名记录。
func (r *RegistrationGroupRepository) CreateTx(tx *gorm.DB, g *model.RegistrationGroup) error {
	if err := tx.Create(g).Error; err != nil {
		return fmt.Errorf("create registration group: %w", err)
	}
	return nil
}

// FindByID 按 ID 查询团体报名。
func (r *RegistrationGroupRepository) FindByID(id uint64) (*model.RegistrationGroup, error) {
	return r.findByID(r.db, id, false)
}

// FindByIDForUpdate 在事务内锁定团体报名行。
func (r *RegistrationGroupRepository) FindByIDForUpdate(tx *gorm.DB, id uint64) (*model.RegistrationGroup, error) {
	return r.findByID(tx, id, true)
}

func (r *RegistrationGroupRepository) findByID(db *gorm.DB, id uint64, forUpdate bool) (*model.RegistrationGroup, error) {
	var g model.RegistrationGroup
	q := db
	if forUpdate {
		q = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.First(&g, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find registration group by id: %w", err)
	}
	return &g, nil
}

// ListByUser 分页查询某用户提交的团体报名。
func (r *RegistrationGroupRepository) ListByUser(userID uint64, page, pageSize int) ([]model.RegistrationGroup, int64, error) {
	var list []model.RegistrationGroup
	var total int64
	q := r.db.Model(&model.RegistrationGroup{}).Where("user_id = ?", userID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count user registration groups: %w", err)
	}
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("list user registration groups: %w", err)
	}
	return list, total, nil
}

// UpdateTx 在事务内更新团体报名。
func (r *RegistrationGroupRepository) UpdateTx(tx *gorm.DB, g *model.RegistrationGroup) error {
	if err := tx.Save(g).Error; err != nil {
		return fmt.Errorf("update registration group: %w", err)
	}
	return nil
}
