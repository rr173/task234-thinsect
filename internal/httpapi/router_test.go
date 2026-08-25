package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

func TestRouterExposesHealthAndBatchCreation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "http.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	handler := New(service.New(db))

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", health.Code)
	}

	body, _ := json.Marshal(map[string]string{"code": "HTTP-1", "rock_type": "basalt", "locality": "field"})
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/batches", bytes.NewReader(body)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", created.Code, created.Body.String())
	}
	var got struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(created.Body).Decode(&got); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if got.ID == 0 || got.Status != "importing" {
		t.Fatalf("unexpected created batch: %+v", got)
	}
}

func TestRouterMapsStateTransitionErrorsToConflict(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("HTTP-STATE", "basalt", "field")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	body := bytes.NewBufferString(`{"to":"review"}`)
	rr := httptest.NewRecorder()
	New(app).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/batches/"+strconv.FormatInt(batch.ID, 10)+"/advance", body))
	if rr.Code != http.StatusConflict {
		t.Fatalf("state transition status = %d, want 409: %s", rr.Code, rr.Body.String())
	}
}
