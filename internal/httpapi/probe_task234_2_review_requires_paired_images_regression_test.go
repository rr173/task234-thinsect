package httpapi_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"task234-thinsect/internal/httpapi"
	"task234-thinsect/internal/image"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

func TestBug02ReviewRequiresPairedImages(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "bug02.db"))
	if err != nil { t.Fatalf("open db: %v", err) }
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("BUG02", "basalt", "field")
	if err != nil { t.Fatalf("create batch: %v", err) }
	if _, _, err := app.Image.Import(imageInput(batch.ID)); err != nil { t.Fatalf("import PPL: %v", err) }
	handler := httpapi.New(app)
	body := bytes.NewBufferString(`{"to":"review"}`)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/batches/"+strconv.FormatInt(batch.ID, 10)+"/advance", body))
	if rr.Code != http.StatusConflict {
		t.Fatalf("advancing without XPL status = %d, want 409: %s", rr.Code, rr.Body.String())
	}
	got, err := app.GetBatch(batch.ID)
	if err != nil { t.Fatalf("get batch: %v", err) }
	if got.Status != "importing" {
		t.Fatalf("failed review transition must preserve importing, got %s", got.Status)
	}
}

func imageInput(batchID int64) image.ImportInput {
	return image.ImportInput{BatchID: batchID, Name: "ppl", Mode: "PPL", SHA256: "bug02-ppl", Width: 100, Height: 100, AvgBrightness: 100}
}
