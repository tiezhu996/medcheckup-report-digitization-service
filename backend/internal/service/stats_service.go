package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/blueship581/gbcheckup/internal/constants"
	"github.com/blueship581/gbcheckup/internal/model"
	"github.com/blueship581/gbcheckup/internal/repository"
)

// StatsService 运营统计服务。
type StatsService struct {
	pkgRepo     *repository.PackageRepository
	regRepo     *repository.RegistrationRepository
	reportRepo  *repository.ReportRepository
	resultRepo  *repository.ExamResultRepository
	metricRepo  *repository.AbnormalMetricRepository
	itemRepo    *repository.PackageItemRepository
	log         *slog.Logger
}

// NewStatsService 构造统计服务。
func NewStatsService(pkgRepo *repository.PackageRepository, regRepo *repository.RegistrationRepository, reportRepo *repository.ReportRepository, resultRepo *repository.ExamResultRepository, metricRepo *repository.AbnormalMetricRepository, itemRepo *repository.PackageItemRepository, log *slog.Logger) *StatsService {
	return &StatsService{pkgRepo: pkgRepo, regRepo: regRepo, reportRepo: reportRepo, resultRepo: resultRepo, metricRepo: metricRepo, itemRepo: itemRepo, log: log}
}

// Dashboard 运营统计。
func (s *StatsService) Dashboard(ctx context.Context) (*model.DashboardStats, error) {
	stats := &model.DashboardStats{}
	stats.PackageCount, _ = s.pkgRepo.Count()
	stats.RegistrationCount, _ = s.regRepo.Count()
	stats.ReportCount, _ = s.reportRepo.Count()
	stats.AbnormalCount, _ = s.metricRepo.Count()
	revenue, _ := s.regRepo.RevenueByMonth()
	total := 0.0
	for _, m := range revenue {
		total += m.Amount
	}
	stats.Revenue = total
	stats.MonthlyRevenue = revenue
	stats.PackageSold, _ = s.regRepo.CountGroupByPackage()
	stats.DeptWorkload, _ = s.itemRepo.CountGroupByDepartment()
	stats.AbnormalTop, _ = s.resultRepo.CountAbnormalGroupByItem()
	s.log.InfoContext(ctx, constants.LOG_STATS_DASHBOARD, "registrations", stats.RegistrationCount)
	return stats, nil
}

// ExportDailyReport 导出运营日报：按科室并发统计工作量。
func (s *StatsService) ExportDailyReport(ctx context.Context) ([]model.NameCount, error) {
	depts, err := s.itemRepo.CountGroupByDepartment()
	if err != nil {
		return nil, err
	}
	type exportResult struct {
		name  string
		count int64
		err   error
	}
	// 缓冲等于科室数：每个 goroutine 都能无阻塞地写入结果，
	// 即使主循环提前 return，剩余 goroutine 仍可安全发送，关闭协程通过
	// wg.Wait() 保证在所有发送完成后才 close(out)，杜绝 send on closed channel。
	out := make(chan exportResult, len(depts))
	var wg sync.WaitGroup
	for _, d := range depts {
		wg.Add(1)
		go func(dept model.NameCount) {
			defer wg.Done()
			if dept.Name == "" {
				out <- exportResult{err: errors.New("invalid department name")}
				return
			}
			out <- exportResult{name: dept.Name, count: dept.Count}
		}(d)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	rows := make([]model.NameCount, 0, len(depts))
	for r := range out {
		if r.err != nil {
			return nil, r.err
		}
		rows = append(rows, model.NameCount{Name: r.name, Count: r.count})
	}
	return rows, nil
}
