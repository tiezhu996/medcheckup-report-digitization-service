package config_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/blueship581/gbcheckup/internal/config"
	"github.com/blueship581/gbcheckup/internal/handler"
	"github.com/blueship581/gbcheckup/internal/middleware"
	"github.com/blueship581/gbcheckup/internal/model"
	"github.com/blueship581/gbcheckup/internal/repository"
	"github.com/blueship581/gbcheckup/internal/router"
	"github.com/blueship581/gbcheckup/internal/service"
	"github.com/blueship581/gbcheckup/internal/util"
	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Package{}, &model.PackageItem{}, &model.Examinee{},
		&model.Registration{}, &model.ExamResult{}, &model.Report{}, &model.AbnormalMetric{},
		&model.Enterprise{}, &model.GroupOrder{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func buildTestApp(t *testing.T) (*gin.Engine, *gorm.DB, string) {
	t.Helper()
	db := newTestDB(t)
	log := testLogger()
	cfg := config.Config{Port: "0", JWTSecret: "test-secret", JWTExpireHours: 72,
		RateLimitPerMin: 100000, UploadDir: t.TempDir()}
	userRepo := repository.NewUserRepository(db)
	pkgRepo := repository.NewPackageRepository(db)
	itemRepo := repository.NewPackageItemRepository(db)
	examineeRepo := repository.NewExamineeRepository(db)
	regRepo := repository.NewRegistrationRepository(db)
	resultRepo := repository.NewExamResultRepository(db)
	reportRepo := repository.NewReportRepository(db)
	metricRepo := repository.NewAbnormalMetricRepository(db)
	entRepo := repository.NewEnterpriseRepository(db)
	orderRepo := repository.NewGroupOrderRepository(db)
	userSvc := service.NewUserService(userRepo, cfg.JWTSecret, cfg.JWTExpireHours, log)
	pkgSvc := service.NewPackageService(pkgRepo, itemRepo, log)
	examineeSvc := service.NewExamineeService(examineeRepo, log)
	regSvc := service.NewRegistrationService(regRepo, examineeRepo, pkgRepo, itemRepo, resultRepo, log)
	resultSvc := service.NewExamResultService(resultRepo, regRepo, metricRepo, log)
	reportSvc := service.NewReportService(reportRepo, resultRepo, regRepo, log)
	metricSvc := service.NewAbnormalMetricService(metricRepo, log)
	entSvc := service.NewEnterpriseService(entRepo, orderRepo, pkgRepo, log)
	statsSvc := service.NewStatsService(pkgRepo, regRepo, reportRepo, resultRepo, metricRepo, itemRepo, log)
	h := router.Handlers{
		User:         handler.NewUserHandler(userSvc, log),
		Package:      handler.NewPackageHandler(pkgSvc, log),
		Examinee:     handler.NewExamineeHandler(examineeSvc, log),
		Registration: handler.NewRegistrationHandler(regSvc, log),
		ExamResult:   handler.NewExamResultHandler(resultSvc, log),
		Report:       handler.NewReportHandler(reportSvc, log),
		Abnormal:     handler.NewAbnormalMetricHandler(metricSvc, log),
		Enterprise:   handler.NewEnterpriseHandler(entSvc, log),
		Stats:        handler.NewStatsHandler(statsSvc, log),
	}
	admin := model.User{Phone: "13800000001", PasswordHash: "x", Name: "管理员", Role: "admin"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	token, err := util.GenerateToken(cfg.JWTSecret, 72, admin.ID, admin.Phone, admin.Role)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	r := router.New(cfg, log, h, middleware.NewRateLimiter(cfg.RateLimitPerMin), cfg.UploadDir)
	return r, db, token
}

func doRequest(r *gin.Engine, method, path, token, body string) *httptest.ResponseRecorder {
	var buf *bytes.Buffer
	if body == "" {
		buf = bytes.NewBuffer(nil)
	} else {
		buf = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

var _ = http.StatusOK


func mc008Stats(t *testing.T, withEmpty bool) *service.StatsService {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "mc008.db") + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(4)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Package{}, &model.PackageItem{}, &model.Examinee{},
		&model.Registration{}, &model.ExamResult{}, &model.Report{}, &model.AbnormalMetric{},
		&model.Enterprise{}, &model.GroupOrder{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	itemRepo := repository.NewPackageItemRepository(db)
	if err := db.Create(&model.PackageItem{ItemName: "血常规", ItemGroup: "检验", RefValueRange: "3.5-9.5", Department: "检验科", SortOrder: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.PackageItem{ItemName: "心电图", ItemGroup: "功能", RefValueRange: "正常", Department: "心电室", SortOrder: 2}).Error; err != nil {
		t.Fatal(err)
	}
	if withEmpty {
		if err := db.Create(&model.PackageItem{ItemName: "未知项目", ItemGroup: "", RefValueRange: "", Department: "", SortOrder: 3}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return service.NewStatsService(repository.NewPackageRepository(db), repository.NewRegistrationRepository(db),
		repository.NewReportRepository(db), repository.NewExamResultRepository(db),
		repository.NewAbnormalMetricRepository(db), itemRepo, testLogger())
}

func TestMedStatsExportParallel(t *testing.T) {
	svc := mc008Stats(t, false)
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, err := svc.ExportDailyReport(context.Background())
			results[idx] = err
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range results {
		if err != nil {
			t.Fatalf("export %d error: %v", i, err)
		}
	}
}

func TestMedStatsExportNoHang(t *testing.T) {
	svc := mc008Stats(t, true)
	start := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-start
		_, err := svc.ExportDailyReport(context.Background())
		done <- err
	}()
	close(start)
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("ExportDailyReport with invalid department returned nil error")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("ExportDailyReport hung on error path")
	}
}

func TestMedStatsHandlerNoHang(t *testing.T) {
	r, db, token := buildTestApp(t)
	if err := db.Create(&model.PackageItem{ItemName: "未知项目", ItemGroup: "", RefValueRange: "", Department: "", SortOrder: 3}).Error; err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	done := make(chan int, 1)
	go func() {
		<-start
		req := httptest.NewRequest("GET", "/api/v1/stats/export", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		done <- w.Code
	}()
	close(start)
	select {
	case code := <-done:
		if code != 500 {
			t.Fatalf("export endpoint status=%d, want 500", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("export endpoint hung on error path")
	}
}
