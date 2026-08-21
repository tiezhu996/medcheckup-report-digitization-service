package service_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"context"
	"fmt"
	"path/filepath"
	"sync"

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


func mc001TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "mc001.db") + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	if err := db.AutoMigrate(&model.User{}, &model.Package{}, &model.PackageItem{}, &model.Examinee{},
		&model.Registration{}, &model.ExamResult{}, &model.Report{}, &model.AbnormalMetric{},
		&model.Enterprise{}, &model.GroupOrder{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestMedPkgCacheConcurrent(t *testing.T) {
	db := mc001TestDB(t)
	pkgRepo := repository.NewPackageRepository(db)
	pkg := model.Package{Name: "入职体检", PackageType: "entry", Price: 300, Status: "active"}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := pkgRepo.FindByID(pkg.ID); err != nil {
		t.Fatal(err)
	}
	svc := service.NewPackageService(pkgRepo, repository.NewPackageItemRepository(db), testLogger())
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 60; i++ {
			if _, err := svc.Update(context.Background(), pkg.ID, fmt.Sprintf("套餐%d", i), "entry", 300+float64(i), "active", ""); err != nil {
				t.Errorf("update error: %v", err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 60; i++ {
			if p, err := pkgRepo.FindByID(pkg.ID); err == nil && p.Price < 0 {
				t.Errorf("negative price observed: %v", p.Price)
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 60; i++ {
			n := model.Package{Name: fmt.Sprintf("新套餐%d", i), PackageType: "entry", Price: 100, Status: "active"}
			if err := pkgRepo.Create(&n); err != nil {
				t.Errorf("create error: %v", err)
			}
			m := model.Package{Name: fmt.Sprintf("直建套餐%d", i), PackageType: "entry", Price: 100, Status: "active"}
			if err := db.Create(&m).Error; err != nil {
				t.Errorf("direct create error: %v", err)
			}
			if p, err := pkgRepo.FindByID(m.ID); err != nil || p == nil || p.Price != 100 {
				t.Errorf("find direct-created package: err=%v p=%v", err, p)
			}
		}
	}()
	close(start)
	wg.Wait()
}

func TestMedPkgSnapshotStable(t *testing.T) {
	db := mc001TestDB(t)
	pkgRepo := repository.NewPackageRepository(db)
	pkg := model.Package{Name: "年度体检", PackageType: "annual", Price: 800, Status: "active"}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	svc := service.NewPackageService(pkgRepo, repository.NewPackageItemRepository(db), testLogger())
	got, err := pkgRepo.FindByID(pkg.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := got.Price
	if _, err := svc.Update(context.Background(), pkg.ID, "高端年度体检", "premium", 1600, "inactive", "调整后"); err != nil {
		t.Fatal(err)
	}
	if got.Price != before {
		t.Fatalf("caller-held package reference mutated by later update: before=%v after=%v", before, got.Price)
	}
	latest, err := pkgRepo.FindByID(pkg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Price != 1600 {
		t.Fatalf("latest price = %v, want 1600", latest.Price)
	}
}

func TestMedPkgUpdateLatest(t *testing.T) {
	db := mc001TestDB(t)
	pkgRepo := repository.NewPackageRepository(db)
	pkg := model.Package{Name: "入职体检", PackageType: "entry", Price: 300, Status: "active"}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	svc := service.NewPackageService(pkgRepo, repository.NewPackageItemRepository(db), testLogger())
	got, err := svc.Update(context.Background(), pkg.ID, "入职体检升级版", "entry", 500, "active", "调整价格")
	if err != nil {
		t.Fatal(err)
	}
	if got.Price != 500 {
		t.Fatalf("Update() returned price=%v, want 500 (stale snapshot)", got.Price)
	}
	if got.Name != "入职体检升级版" {
		t.Fatalf("Update() returned name=%q, want 入职体检升级版", got.Name)
	}
}

func TestMedPkgHandlerConcurrent(t *testing.T) {
	r, db, token := buildTestApp(t)
	pkg := model.Package{Name: "入职体检", PackageType: "entry", Price: 300, Status: "active"}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/packages/%d", pkg.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("warm cache status=%d", w.Code)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 60; i++ {
			body := fmt.Sprintf(`{"name":"套餐%d","package_type":"entry","price":%d,"status":"active","description":""}`, i, 300+i)
			req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/packages/%d", pkg.ID), bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != 200 {
				t.Errorf("PUT /packages/%d status=%d body=%s", pkg.ID, w.Code, w.Body.String())
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 60; i++ {
			req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/packages/%d", pkg.ID), nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != 200 {
				t.Errorf("GET /packages/%d status=%d body=%s", pkg.ID, w.Code, w.Body.String())
			}
		}
	}()
	close(start)
	wg.Wait()
}
