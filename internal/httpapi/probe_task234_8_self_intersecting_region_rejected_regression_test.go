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

func TestBug08SelfIntersectingRegionIsRejected(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "bug08.db")); if err != nil { t.Fatalf("open db: %v", err) }; defer db.Close()
	app := service.New(db); batch, err := app.CreateBatch("BUG08", "basalt", "field"); if err != nil { t.Fatalf("batch: %v", err) }
	img, _, err := app.Image.Import(image.ImportInput{BatchID: batch.ID, Name: "ppl", Mode: "PPL", SHA256: "bug08-ppl", Width: 100, Height: 100, AvgBrightness: 100}); if err != nil { t.Fatalf("image: %v", err) }
	body := bytes.NewBufferString(`{"batch_id":`+strconv.FormatInt(batch.ID,10)+`,"label":"bowtie","vertices":[{"x":10,"y":10},{"x":40,"y":40},{"x":40,"y":10},{"x":10,"y":40},{"x":10,"y":10}]}`)
	rr := httptest.NewRecorder(); httpapi.New(app).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/images/"+strconv.FormatInt(img.ID,10)+"/regions", body))
	if rr.Code != http.StatusBadRequest { t.Fatalf("self-intersecting region status = %d, want 400: %s", rr.Code, rr.Body.String()) }
}
