package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"task234-thinsect/internal/httpapi"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

func TestBug01DuplicateImageImportIsIdempotent(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "bug01.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("BUG01", "basalt", "field")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	handler := httpapi.New(app)
	request := func(name string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{
			"name": name, "mode": "PPL", "sha256": "same-image",
			"width": 100, "height": 80, "avg_brightness": 120,
		})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/batches/"+itoa(batch.ID)+"/images", bytes.NewReader(body)))
		return rr
	}
	first := request("first")
	if first.Code != http.StatusCreated {
		t.Fatalf("first import status = %d: %s", first.Code, first.Body.String())
	}
	second := request("renamed")
	if second.Code != http.StatusOK {
		t.Fatalf("duplicate import status = %d, want 200: %s", second.Code, second.Body.String())
	}
	var firstImage, secondImage struct{ ID int64 `json:"id"` }
	if err := json.NewDecoder(first.Body).Decode(&firstImage); err != nil {
		t.Fatalf("decode first: %v", err)
	}
	if err := json.NewDecoder(second.Body).Decode(&secondImage); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if firstImage.ID == 0 || firstImage.ID != secondImage.ID {
		t.Fatalf("duplicate should return the original image: first=%+v second=%+v", firstImage, secondImage)
	}
	images, err := app.Image.ListByBatch(batch.ID)
	if err != nil || len(images) != 1 {
		t.Fatalf("duplicate import should leave one row: len=%d err=%v", len(images), err)
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
