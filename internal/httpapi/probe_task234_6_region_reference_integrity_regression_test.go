package httpapi_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"task234-thinsect/internal/image"
	"task234-thinsect/internal/httpapi"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

func TestBug06RegionCannotReferenceMissingBatch(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "bug06.db"))
	if err != nil { t.Fatalf("open db: %v", err) }
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("BUG06", "basalt", "field")
	if err != nil { t.Fatalf("batch: %v", err) }
	other, err := app.CreateBatch("BUG06-OTHER", "basalt", "field")
	if err != nil { t.Fatalf("other batch: %v", err) }
	img, _, err := app.Image.Import(image.ImportInput{BatchID: batch.ID, Name: "ppl", Mode: "PPL", SHA256: "bug06-ppl", Width: 100, Height: 100, AvgBrightness: 100})
	if err != nil { t.Fatalf("image: %v", err) }
	body := bytes.NewBufferString(`{"batch_id":`+strconv.FormatInt(other.ID, 10)+`,"label":"orphan","vertices":[{"x":10,"y":10},{"x":30,"y":10},{"x":30,"y":30},{"x":10,"y":30},{"x":10,"y":10}]}`)
	rr := httptest.NewRecorder()
	httpapi.New(app).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/images/"+strconv.FormatInt(img.ID, 10)+"/regions", body))
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusNotFound {
		t.Fatalf("orphan region status = %d, want rejection: %s", rr.Code, rr.Body.String())
	}
}
