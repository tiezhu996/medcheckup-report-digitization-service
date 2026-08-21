package repository

import (
	"errors"
	"sync"

	"github.com/blueship581/gbcheckup/internal/model"
	"github.com/blueship581/gbcheckup/internal/util"
	"gorm.io/gorm"
)

// PackageRepository 套餐仓储。
//
// cache 用于加速按 ID 读取，但并发 HTTP 请求会同时读写该 map，必须用 mu 保护，
// 否则会触发 "concurrent map read and map write" 崩溃。cache 与对外返回的指针
// 必须互相独立（store 时拷贝、read 时拷贝），避免调用方与缓存共享同一对象后
// 被后续 Update 改动而读到半新半旧的值。
type PackageRepository struct {
	db    *gorm.DB
	mu    sync.RWMutex
	cache map[uint]*model.Package
}

// NewPackageRepository 构造套餐仓储。
func NewPackageRepository(db *gorm.DB) *PackageRepository {
	return &PackageRepository{db: db, cache: map[uint]*model.Package{}}
}

func (r *PackageRepository) Create(pkg *model.Package) error {
	if err := r.db.Create(pkg).Error; err != nil {
		return err
	}
	cached := *pkg
	r.mu.Lock()
	r.cache[pkg.ID] = &cached
	r.mu.Unlock()
	return nil
}

func (r *PackageRepository) FindByID(id uint) (*model.Package, error) {
	r.mu.RLock()
	if p, ok := r.cache[id]; ok {
		cp := *p
		r.mu.RUnlock()
		return &cp, nil
	}
	r.mu.RUnlock()

	var pkg model.Package
	if err := r.db.First(&pkg, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, util.ErrNotFound
		}
		return nil, err
	}
	cached := pkg // 缓存一份独立拷贝，与返回值互不别名
	r.mu.Lock()
	r.cache[id] = &cached
	r.mu.Unlock()
	return &pkg, nil
}

func (r *PackageRepository) List(status string, page, pageSize int) ([]model.Package, int64, error) {
	q := r.db.Model(&model.Package{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var pkgs []model.Package
	q2 := r.db.Order("id asc")
	if status != "" {
		q2 = q2.Where("status = ?", status)
	}
	err := q2.Offset((page-1)*pageSize).Limit(pageSize).Find(&pkgs).Error
	return pkgs, total, err
}

func (r *PackageRepository) Update(pkg *model.Package) error {
	if err := r.db.Save(pkg).Error; err != nil {
		return err
	}
	cached := *pkg
	r.mu.Lock()
	r.cache[pkg.ID] = &cached
	r.mu.Unlock()
	return nil
}

func (r *PackageRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&model.Package{}).Count(&count).Error
	return count, err
}

// CountGroupByType 按套餐类型统计成交量。
func (r *PackageRepository) CountGroupByType() ([]model.NameCount, error) {
	var rows []model.NameCount
	err := r.db.Model(&model.Package{}).Select("name, count(*) as count").Group("name").Scan(&rows).Error
	return rows, err
}
