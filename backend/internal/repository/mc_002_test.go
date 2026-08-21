package repository_test

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


func mc002Result(t *testing.T) (uint, *repository.ExamResultRepository, *service.ExamResultService) {
	t.Helper()
	db := newTestDB(t)
	item := model.PackageItem{ItemName: "血糖", RefValueRange: "3.9-6.1", Department: "检验科"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	examinee := model.Examinee{Name: "张三", IDCardNo: "110101199001011234"}
	if err := db.Create(&examinee).Error; err != nil {
		t.Fatal(err)
	}
	res := model.ExamResult{ExamineeID: examinee.ID, PackageItemID: item.ID, Status: "pending", DoctorID: 1}
	if err := db.Create(&res).Error; err != nil {
		t.Fatal(err)
	}
	resultRepo := repository.NewExamResultRepository(db)
	svc := service.NewExamResultService(resultRepo, repository.NewRegistrationRepository(db), repository.NewAbnormalMetricRepository(db), testLogger())
	return res.ID, resultRepo, svc
}

func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestMedCtxListCancel(t *testing.T) {
	resID, _, svc := mc002Result(t)
	for i := 0; i < 10; i++ {
		if _, err := svc.ListByRegistration(canceledCtx(), resID); err == nil {
			t.Fatalf("round %d ListByRegistration ignored cancelled ctx", i)
		}
	}
}

func TestMedCtxReviewCancel(t *testing.T) {
	resID, _, svc := mc002Result(t)
	for i := 0; i < 10; i++ {
		if err := svc.Review(canceledCtx(), resID); err == nil {
			t.Fatalf("round %d Review ignored cancelled ctx", i)
		}
	}
}

func TestMedCtxPendingCancel(t *testing.T) {
	_, _, svc := mc002Result(t)
	for i := 0; i < 10; i++ {
		if _, _, err := svc.ListPending(canceledCtx(), 1, 20); err == nil {
			t.Fatalf("round %d ListPending ignored cancelled ctx", i)
		}
	}
}

func TestMedCtxStaleBind(t *testing.T) {
	resID, _, svc := mc002Result(t)
	ctx1, cancel1 := context.WithCancel(context.Background())
	if _, err := svc.Enter(ctx1, resID, 1, service.EnterInput{ResultValue: "5.2"}); err != nil {
		t.Fatalf("first Enter error = %v", err)
	}
	cancel1()
	ctx2 := context.Background()
	for i := 0; i < 10; i++ {
		if err := svc.Review(ctx2, resID); err != nil {
			t.Fatalf("round %d Review after first ctx cancelled error = %v (stale ctx reused)", i, err)
		}
	}
}

func TestMedCtxHandlerAbort(t *testing.T) {
	r, db, token := buildTestApp(t)
	item := model.PackageItem{ItemName: "血常规", RefValueRange: "3.5-9.5", Department: "检验科"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	examinee := model.Examinee{Name: "王五", IDCardNo: "110101199001011236"}
	if err := db.Create(&examinee).Error; err != nil {
		t.Fatal(err)
	}
	res := model.ExamResult{ExamineeID: examinee.ID, PackageItemID: item.ID, Status: "pending", DoctorID: 1}
	if err := db.Create(&res).Error; err != nil {
		t.Fatal(err)
	}
	body := `{"result_value":"7.0","result_text":"","image_url":""}`
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/exam-results/%d/enter", res.ID), bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var updated model.ExamResult
	if err := db.First(&updated, res.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status == "entered" {
		t.Fatalf("exam result updated despite cancelled request context (status=%s, http=%d)", updated.Status, w.Code)
	}
}
