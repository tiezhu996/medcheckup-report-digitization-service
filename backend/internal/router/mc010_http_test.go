package router

import (
	"fmt"
	"testing"
)

func TestMC010ArchivedStatusAccepted(t *testing.T) {
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

func TestMC010InvalidStatusRejected(t *testing.T) {
	r, _, token := buildTestApp(t)
	for i := 0; i < 10; i++ {
		body := `{"name":"坏套餐","package_type":"entry","price":100,"status":"weird","description":""}`
		w := doRequest(r, "POST", "/api/v1/packages", token, body)
		if w.Code != 400 {
			t.Fatalf("round %d invalid status=%d body=%s, want 400", i, w.Code, w.Body.String())
		}
	}
}

func TestMC010ListArchivedWorks(t *testing.T) {
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
