package handler_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"context"
	"errors"
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


func mc003Registration(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	examinee := model.Examinee{Name: "张三", IDCardNo: "110101199001011234"}
	if err := db.Create(&examinee).Error; err != nil {
		t.Fatal(err)
	}
	pkg := model.Package{Name: "入职体检", PackageType: "entry", Price: 300, Status: "active"}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	reg := model.Registration{ExamineeID: examinee.ID, PackageID: pkg.ID, GuideNo: "GUIDE202608210001", Status: "registered"}
	if err := db.Create(&reg).Error; err != nil {
		t.Fatal(err)
	}
	return reg.ID
}

func TestMedReportDraftCreate(t *testing.T) {
	db := newTestDB(t)
	regID := mc003Registration(t, db)
	svc := service.NewReportService(repository.NewReportRepository(db), repository.NewExamResultRepository(db), repository.NewRegistrationRepository(db), testLogger())
	for i := 0; i < 10; i++ {
		report, err := svc.DraftOrGet(context.Background(), regID)
		if err != nil {
			t.Fatalf("round %d DraftOrGet first call error = %v", i, err)
		}
		if report.ID == 0 || report.Status != "draft" {
			t.Fatalf("round %d draft invalid: %+v", i, report)
		}
		report2, err := svc.DraftOrGet(context.Background(), regID)
		if err != nil {
			t.Fatalf("round %d DraftOrGet second call error = %v", i, err)
		}
		if report2.ID != report.ID {
			t.Fatalf("round %d second call returned different draft id %d != %d", i, report2.ID, report.ID)
		}
	}
}

func TestMedReportGet404(t *testing.T) {
	db := newTestDB(t)
	svc := service.NewReportService(repository.NewReportRepository(db), repository.NewExamResultRepository(db), repository.NewRegistrationRepository(db), testLogger())
	for i := 0; i < 10; i++ {
		_, err := svc.Get(context.Background(), 999999)
		if err == nil {
			t.Fatalf("round %d Get missing report returned nil error", i)
		}
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.HTTPStatus != 404 {
			t.Fatalf("round %d Get missing report error=%v, want 404 AppError", i, err)
		}
	}
}

func TestMedReportDraftReg404(t *testing.T) {
	db := newTestDB(t)
	svc := service.NewReportService(repository.NewReportRepository(db), repository.NewExamResultRepository(db), repository.NewRegistrationRepository(db), testLogger())
	for i := 0; i < 10; i++ {
		_, err := svc.DraftOrGet(context.Background(), 999999)
		if err == nil {
			t.Fatalf("round %d DraftOrGet missing registration returned nil error", i)
		}
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.HTTPStatus != 404 {
			t.Fatalf("round %d DraftOrGet missing registration error=%v, want 404 AppError", i, err)
		}
	}
}

func TestMedReportDraftHttp(t *testing.T) {
	r, db, token := buildTestApp(t)
	examinee := model.Examinee{Name: "李四", IDCardNo: "110101199001011235"}
	if err := db.Create(&examinee).Error; err != nil {
		t.Fatal(err)
	}
	pkg := model.Package{Name: "年度体检", PackageType: "annual", Price: 800, Status: "active"}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	reg := model.Registration{ExamineeID: examinee.ID, PackageID: pkg.ID, GuideNo: "GUIDE202608210002", Status: "registered"}
	if err := db.Create(&reg).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		w := doRequest(r, "POST", fmt.Sprintf("/api/v1/reports/draft?registration_id=%d", reg.ID), token, "")
		if w.Code != 201 {
			t.Fatalf("round %d draft endpoint status=%d body=%s", i, w.Code, w.Body.String())
		}
	}
}
