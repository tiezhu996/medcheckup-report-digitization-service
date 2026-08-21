package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/blueship581/gbcheckup/internal/constants"
	"github.com/blueship581/gbcheckup/internal/model"
	"github.com/blueship581/gbcheckup/internal/repository"
	"github.com/blueship581/gbcheckup/internal/util"
)

// EnterpriseService 团检企业与订单服务。
type EnterpriseService struct {
	entRepo *repository.EnterpriseRepository
	orderRepo *repository.GroupOrderRepository
	pkgRepo *repository.PackageRepository
	log     *slog.Logger
}

// NewEnterpriseService 构造团检服务。
func NewEnterpriseService(entRepo *repository.EnterpriseRepository, orderRepo *repository.GroupOrderRepository, pkgRepo *repository.PackageRepository, log *slog.Logger) *EnterpriseService {
	return &EnterpriseService{entRepo: entRepo, orderRepo: orderRepo, pkgRepo: pkgRepo, log: log}
}

// CreateEnterprise 创建企业。
func (s *EnterpriseService) CreateEnterprise(ctx context.Context, e *model.Enterprise) (*model.Enterprise, error) {
	if err := s.entRepo.Create(e); err != nil {
		return nil, util.LogError(s.log, constants.LOG_ENTERPRISE_CREATED, fmt.Errorf("create enterprise: %w", err))
	}
	s.log.InfoContext(ctx, constants.LOG_ENTERPRISE_CREATED, "enterprise_id", e.ID)
	return e, nil
}

// ListEnterprises 企业列表。
func (s *EnterpriseService) ListEnterprises(ctx context.Context, page, pageSize int) ([]model.Enterprise, int64, error) {
	return s.entRepo.List(page, pageSize)
}

// CreateOrder 创建团检订单。
func (s *EnterpriseService) CreateOrder(ctx context.Context, enterpriseID, packageID uint, count int) (*model.GroupOrder, error) {
	if _, err := s.entRepo.FindByID(enterpriseID); err != nil {
		return nil, util.NotFoundError("团检企业（Enterprise）不存在", err)
	}
	if _, err := s.pkgRepo.FindByID(packageID); err != nil {
		return nil, util.NotFoundError(constants.MsgPackageNotFound, err)
	}
	order := &model.GroupOrder{EnterpriseID: enterpriseID, PackageID: packageID, ExamineeCount: count, Status: constants.GroupOrderPending}
	if err := s.orderRepo.Create(order); err != nil {
		return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_CREATED, fmt.Errorf("create group order: %w", err))
	}
	s.log.InfoContext(ctx, constants.LOG_GROUP_ORDER_CREATED, "order_id", order.ID)
	return order, nil
}

// ListOrders 团检订单列表。
func (s *EnterpriseService) ListOrders(ctx context.Context, page, pageSize int) ([]model.GroupOrder, int64, error) {
	return s.orderRepo.List(page, pageSize)
}

// DeliverReports 报告批量交付。
//
// 业务规则：
//   - 订单必须存在，否则返回 404；
//   - 订单必须处于待交付态（pending），已交付（done）的订单不可重复交付，返回 409；
//   - 订单人数必须大于 0，否则返回 500（订单不应存在 0 人，属数据异常）。
//
// 任一交付步骤失败都会以 AppError 形式向上抛出，绝不吞错。
func (s *EnterpriseService) DeliverReports(ctx context.Context, orderID uint) (*model.GroupOrder, error) {
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		if errors.Is(err, util.ErrNotFound) {
			return nil, util.NotFoundError("团检订单（GroupOrder）不存在", err)
		}
		return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVERED, fmt.Errorf("find group order: %w", err))
	}
	if order.Status != constants.GroupOrderPending {
		return nil, util.ConflictError("团检订单（GroupOrder.status）不可重复交付", errors.New("order not pending"))
	}
	if order.ExamineeCount <= 0 {
		return nil, util.InternalError("团检订单（GroupOrder.examinee_count）人数为 0，无法交付", errors.New("order has no examinees"))
	}
	// 逐人交付：事务内更新交付状态，任一步失败整体回滚。
	iterations := order.ExamineeCount
	for i := 0; i < iterations; i++ {
		if err := s.orderRepo.Deliver(order); err != nil {
			return nil, util.LogError(s.log, constants.LOG_GROUP_ORDER_DELIVERED, fmt.Errorf("deliver step %d: %w", i, err))
		}
	}
	s.log.InfoContext(ctx, constants.LOG_GROUP_ORDER_DELIVERED, "order_id", orderID)
	return order, nil
}
