package model_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"context"
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


func mc007Registration(t *testing.T, db *gorm.DB) uint {
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

func mc007Svc(db *gorm.DB) *service.RegistrationService {
	return service.NewRegistrationService(repository.NewRegistrationRepository(db), repository.NewExamineeRepository(db),
		repository.NewPackageRepository(db), repository.NewPackageItemRepository(db), repository.NewExamResultRepository(db), testLogger())
}

func TestMedRegFlowForward(t *testing.T) {
	for i := 0; i < 10; i++ {
		db := newTestDB(t)
		regID := mc007Registration(t, db)
		svc := mc007Svc(db)
		if _, err := svc.UpdateStatus(context.Background(), regID, "in_progress"); err != nil {
			t.Fatalf("round %d registered->in_progress error = %v", i, err)
		}
		if _, err := svc.UpdateStatus(context.Background(), regID, "completed"); err != nil {
			t.Fatalf("round %d in_progress->completed error = %v", i, err)
		}
	}
}

func TestMedRegNoBackward(t *testing.T) {
	db := newTestDB(t)
	regID := mc007Registration(t, db)
	svc := mc007Svc(db)
	if _, err := svc.UpdateStatus(context.Background(), regID, "in_progress"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateStatus(context.Background(), regID, "completed"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := svc.UpdateStatus(context.Background(), regID, "registered"); err == nil {
			t.Fatalf("round %d completed->registered should be rejected", i)
		}
	}
}

func TestMedRegListInProgress(t *testing.T) {
	db := newTestDB(t)
	regID1 := mc007Registration(t, db)
	regRepo := repository.NewRegistrationRepository(db)
	if err := regRepo.UpdateStatus(regID1, "in_progress"); err != nil {
		t.Fatal(err)
	}
	examinee := model.Examinee{Name: "李四", IDCardNo: "110101199001011235"}
	if err := db.Create(&examinee).Error; err != nil {
		t.Fatal(err)
	}
	pkg := model.Package{Name: "年度体检", PackageType: "annual", Price: 800, Status: "active"}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	reg2 := model.Registration{ExamineeID: examinee.ID, PackageID: pkg.ID, GuideNo: "GUIDE202608210002", Status: "registered"}
	if err := db.Create(&reg2).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		items, total, err := regRepo.List("in_progress", 1, 20)
		if err != nil {
			t.Fatalf("round %d List(in_progress) error = %v", i, err)
		}
		if total != 2 {
			t.Fatalf("round %d in_progress total = %d, want 2 (registered + in_progress)", i, total)
		}
		if len(items) != 2 {
			t.Fatalf("round %d in_progress items = %d, want 2", i, len(items))
		}
	}
}

func TestMedRegUpdateRefreshed(t *testing.T) {
	db := newTestDB(t)
	regID := mc007Registration(t, db)
	svc := mc007Svc(db)
	got, err := svc.UpdateStatus(context.Background(), regID, "in_progress")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "in_progress" {
		t.Fatalf("UpdateStatus returned status=%q, want in_progress (stale registration returned)", got.Status)
	}
}

func TestMedRegHttpForward(t *testing.T) {
	r, db, token := buildTestApp(t)
	examinee := model.Examinee{Name: "王五", IDCardNo: "110101199001011236"}
	if err := db.Create(&examinee).Error; err != nil {
		t.Fatal(err)
	}
	pkg := model.Package{Name: "高端体检", PackageType: "premium", Price: 2000, Status: "active"}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		reg := model.Registration{ExamineeID: examinee.ID, PackageID: pkg.ID, GuideNo: fmt.Sprintf("GUIDE20260821000%d", i), Status: "registered"}
		if err := db.Create(&reg).Error; err != nil {
			t.Fatal(err)
		}
		w := doRequest(r, "PUT", fmt.Sprintf("/api/v1/registrations/%d/status", reg.ID), token, "{\"status\":\"in_progress\"}")
		if w.Code != 200 {
			t.Fatalf("round %d status in_progress endpoint=%d body=%s", i, w.Code, w.Body.String())
		}
		w = doRequest(r, "PUT", fmt.Sprintf("/api/v1/registrations/%d/status", reg.ID), token, "{\"status\":\"completed\"}")
		if w.Code != 200 {
			t.Fatalf("round %d status completed endpoint=%d body=%s", i, w.Code, w.Body.String())
		}
	}
}
