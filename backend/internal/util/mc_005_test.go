package util_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"context"
	"encoding/json"
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


func mc005Metrics(t *testing.T, n int) (uint, *repository.AbnormalMetricRepository) {
	t.Helper()
	db := newTestDB(t)
	examinee := model.Examinee{Name: "张三", IDCardNo: "110101199001011234"}
	if err := db.Create(&examinee).Error; err != nil {
		t.Fatal(err)
	}
	item := model.PackageItem{ItemName: "血糖", RefValueRange: "3.9-6.1", Department: "检验科"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		m := model.AbnormalMetric{ExamineeID: examinee.ID, PackageItemID: item.ID, AbnormalLevel: "mild",
			Value: fmt.Sprintf("%d", 10+i), RefValueRange: "3.9-6.1", TrendJSON: "[]", FollowUpStatus: "pending"}
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	return examinee.ID, repository.NewAbnormalMetricRepository(db)
}

func TestMedMetricPageSnapshot(t *testing.T) {
	examineeID, repo := mc005Metrics(t, 25)
	svc := service.NewAbnormalMetricService(repo, testLogger())
	for i := 0; i < 10; i++ {
		page1, _, err := svc.List(context.Background(), examineeID, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		wantIDs := make([]uint, len(page1))
		for j := range page1 {
			wantIDs[j] = page1[j].ID
		}
		if _, _, err := svc.List(context.Background(), examineeID, 2, 10); err != nil {
			t.Fatal(err)
		}
		for j := range page1 {
			if page1[j].ID != wantIDs[j] {
				t.Fatalf("round %d page1 snapshot corrupted: item[%d] id=%d want %d", i, j, page1[j].ID, wantIDs[j])
			}
		}
	}
}

func TestMedMetricLastPageRef(t *testing.T) {
	examineeID, repo := mc005Metrics(t, 25)
	svc := service.NewAbnormalMetricService(repo, testLogger())
	if _, _, err := svc.List(context.Background(), examineeID, 1, 10); err != nil {
		t.Fatal(err)
	}
	held := svc.LastPage()
	firstID := held[0].ID
	for i := 0; i < 10; i++ {
		if _, _, err := svc.List(context.Background(), examineeID, 2, 10); err != nil {
			t.Fatal(err)
		}
		if held[0].ID != firstID {
			t.Fatalf("round %d held LastPage reference corrupted: first id %d want %d", i, held[0].ID, firstID)
		}
	}
}

func TestMedMetricHandlerNoAlias(t *testing.T) {
	r, db, token := buildTestApp(t)
	examinee := model.Examinee{Name: "李四", IDCardNo: "110101199001011235"}
	if err := db.Create(&examinee).Error; err != nil {
		t.Fatal(err)
	}
	item := model.PackageItem{ItemName: "肝功能", RefValueRange: "0-40", Department: "检验科"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		m := model.AbnormalMetric{ExamineeID: examinee.ID, PackageItemID: item.ID, AbnormalLevel: "mild",
			Value: fmt.Sprintf("%d", 10+i), RefValueRange: "0-40", TrendJSON: "[]", FollowUpStatus: "pending"}
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		w1 := doRequest(r, "GET", fmt.Sprintf("/api/v1/abnormal-metrics?examinee_id=%d&page=1&page_size=10", examinee.ID), token, "")
		if w1.Code != 200 {
			t.Fatalf("round %d page1 status=%d", i, w1.Code)
		}
		w2 := doRequest(r, "GET", fmt.Sprintf("/api/v1/abnormal-metrics?examinee_id=%d&page=2&page_size=10", examinee.ID), token, "")
		if w2.Code != 200 {
			t.Fatalf("round %d page2 status=%d", i, w2.Code)
		}
		var resp struct {
			Data struct {
				List     []model.AbnormalMetric `json:"list"`
				LastPage []model.AbnormalMetric `json:"last_page"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w2.Body.Bytes(), &resp); err != nil {
			t.Fatalf("round %d unmarshal: %v", i, err)
		}
		if len(resp.Data.LastPage) > 0 && len(resp.Data.List) > 0 && resp.Data.LastPage[0].ID == resp.Data.List[0].ID {
			t.Fatalf("round %d last_page aliased to current page: last=%d list=%d", i, resp.Data.LastPage[0].ID, resp.Data.List[0].ID)
		}
	}
}
