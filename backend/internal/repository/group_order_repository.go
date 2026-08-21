package repository

import (
	"errors"

	"github.com/blueship581/gbcheckup/internal/model"
	"gorm.io/gorm"
)

// GroupOrderRepository 团检订单仓储。
type GroupOrderRepository struct{ db *gorm.DB }

// NewGroupOrderRepository 构造团检订单仓储。
func NewGroupOrderRepository(db *gorm.DB) *GroupOrderRepository { return &GroupOrderRepository{db: db} }

func (r *GroupOrderRepository) Create(o *model.GroupOrder) error { return r.db.Create(o).Error }

func (r *GroupOrderRepository) List(page, pageSize int) ([]model.GroupOrder, int64, error) {
	var total int64
	if err := r.db.Model(&model.GroupOrder{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.GroupOrder
	err := r.db.Preload("Enterprise").Preload("Package").Order("id desc").Offset((page-1)*pageSize).Limit(pageSize).Find(&items).Error
	return items, total, err
}

func (r *GroupOrderRepository) FindByID(id uint) (*model.GroupOrder, error) {
	var o model.GroupOrder
	if err := r.db.Preload("Enterprise").Preload("Package").First(&o, id).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *GroupOrderRepository) Update(o *model.GroupOrder) error { return r.db.Save(o).Error }

// Deliver 交付订单：在事务内更新交付状态，任一步失败整体回滚。
func (r *GroupOrderRepository) Deliver(o *model.GroupOrder) (err error) {
	tx := r.db.Begin()
	defer func() {
		err = tx.Commit().Error
	}()
	if o.ExamineeCount <= 0 {
		return errors.New("order has no examinees")
	}
	if err = tx.Model(o).Updates(map[string]any{
		"status": "done", "report_delivery_status": "delivered",
	}).Error; err != nil {
		return err
	}
	return nil
}
