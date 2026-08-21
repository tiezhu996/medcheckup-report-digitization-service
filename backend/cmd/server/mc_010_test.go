package main_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"fmt"

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


func TestMedPkgArchivedHttp(t *testing.T) {
	r, _, token := buildTestApp(t)
	for i := 0; i < 10; i++ {
		body := fmt.Sprintf(`{"name":"套餐%d","package_type":"entry","price":100,"status":"archived","description":"归档"}`, i)
		w := doRequest(r, "POST", "/api/v1/packages", token, body)
		if w.Code != 201 {
			t.Fatalf("round %d create archived status=%d body=%s", i, w.Code, w.Body.String())
		}
		w = doRequest(r, "PUT", "/api/v1/packages/1", token, body)
		if w.Code != 200 {
			t.Fatalf("round %d update archived status=%d body=%s", i, w.Code, w.Body.String())
		}
	}
}

func TestMedPkgListArchivedHttp(t *testing.T) {
	r, _, token := buildTestApp(t)
	w := doRequest(r, "POST", "/api/v1/packages", token, `{"name":"归档套餐","package_type":"annual","price":500,"status":"archived","description":""}`)
	if w.Code != 201 {
		t.Fatalf("create archived status=%d", w.Code)
	}
	for i := 0; i < 10; i++ {
		w := doRequest(r, "GET", "/api/v1/packages?status=archived&page=1&page_size=20", token, "")
		if w.Code != 200 {
			t.Fatalf("round %d list archived status=%d body=%s", i, w.Code, w.Body.String())
		}
	}
}
