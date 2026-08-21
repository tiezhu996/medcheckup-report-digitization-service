package repository

import (
	"context"
	"errors"

	"github.com/blueship581/gbcheckup/internal/model"
	"github.com/blueship581/gbcheckup/internal/util"
	"gorm.io/gorm"
)

// ExamResultRepository 检查结果仓储。
//
// ctx 仅在构造时绑定（通过 WithCtx/WithTx），绝不在构造之后被改写：
// 该仓储实例由服务在启动时构造一次、所有请求共享，任何请求级的 ctx 都必须经
// WithCtx 返回新实例再使用，否则一个超时 ctx 会永久污染共享单例，后续请求全雪崩。
type ExamResultRepository struct {
	db  *gorm.DB
	ctx context.Context
}

// NewExamResultRepository 构造检查结果仓储。
func NewExamResultRepository(db *gorm.DB) *ExamResultRepository { return &ExamResultRepository{db: db} }

// WithCtx 返回绑定到 ctx 的仓储副本，不修改自身。共享单例上调用安全。
func (r *ExamResultRepository) WithCtx(ctx context.Context) *ExamResultRepository {
	return &ExamResultRepository{db: r.db, ctx: ctx}
}

func (r *ExamResultRepository) g() *gorm.DB {
	if r.ctx != nil {
		return r.db.WithContext(r.ctx)
	}
	return r.db
}

// WithTx 使用事务连接构造仓储，保留父级 ctx 以便事务内读取仍走请求 ctx。
func (r *ExamResultRepository) WithTx(tx *gorm.DB) *ExamResultRepository {
	return &ExamResultRepository{db: tx, ctx: r.ctx}
}

// Transaction 在事务内执行 fn，任一步返回 error 则整体回滚。
func (r *ExamResultRepository) Transaction(fn func(tx *gorm.DB) error) error {
	return r.g().Transaction(fn)
}

func (r *ExamResultRepository) Create(res *model.ExamResult) error { return r.db.Create(res).Error }

func (r *ExamResultRepository) CreateBatch(items []model.ExamResult) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.Create(&items).Error
}

func (r *ExamResultRepository) FindByID(id uint) (*model.ExamResult, error) {
	var res model.ExamResult
	if err := r.g().Preload("PackageItem").First(&res, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, util.ErrNotFound
		}
		return nil, err
	}
	return &res, nil
}

func (r *ExamResultRepository) ListByRegistration(regID uint) ([]model.ExamResult, error) {
	var items []model.ExamResult
	err := r.g().Preload("PackageItem").Where("registration_id = ?", regID).Order("id asc").Find(&items).Error
	return items, err
}

func (r *ExamResultRepository) ListPending(page, pageSize int) ([]model.ExamResult, int64, error) {
	var total int64
	if err := r.g().Model(&model.ExamResult{}).Where("status != ?", "reviewed").Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.ExamResult
	err := r.g().Preload("PackageItem").Where("status != ?", "reviewed").Order("id asc").Offset((page-1)*pageSize).Limit(pageSize).Find(&items).Error
	return items, total, err
}

func (r *ExamResultRepository) Update(res *model.ExamResult) error { return r.g().Save(res).Error }

func (r *ExamResultRepository) CountAbnormal() (int64, error) {
	var count int64
	err := r.db.Model(&model.ExamResult{}).Where("is_abnormal = ?", true).Count(&count).Error
	return count, err
}

// CountAbnormalGroupByItem 异常指标 TOP 统计。
func (r *ExamResultRepository) CountAbnormalGroupByItem() ([]model.NameCount, error) {
	var rows []model.NameCount
	err := r.db.Model(&model.ExamResult{}).
		Select("package_items.item_name as name, count(exam_results.id) as count").
		Joins("JOIN package_items ON package_items.id = exam_results.package_item_id").
		Where("exam_results.is_abnormal = ?", true).
		Group("package_items.item_name").Order("count desc").Limit(10).Scan(&rows).Error
	return rows, err
}
