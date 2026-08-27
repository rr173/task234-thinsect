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
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/segment"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

func TestBug05RelationDetectionIsIdempotent(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "bug05.db"))
	if err != nil { t.Fatalf("open db: %v", err) }
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("BUG05", "basalt", "field")
	if err != nil { t.Fatalf("batch: %v", err) }
	var imgID int64
	for _, in := range []image.ImportInput{
		{BatchID: batch.ID, Name: "ppl", Mode: "PPL", SHA256: "bug05-ppl", Width: 100, Height: 100, AvgBrightness: 100},
		{BatchID: batch.ID, Name: "xpl", Mode: "XPL", SHA256: "bug05-xpl", Width: 100, Height: 100, AvgBrightness: 80},
	} {
		img, _, err := app.Image.Import(in)
		if err != nil { t.Fatalf("image: %v", err) }
		if in.Mode == "PPL" { imgID = img.ID }
	}
	for _, poly := range []model.Polygon{
		{Vertices: []model.Point{{X: 10, Y: 10}, {X: 30, Y: 10}, {X: 30, Y: 30}, {X: 10, Y: 30}, {X: 10, Y: 10}}},
		{Vertices: []model.Point{{X: 31, Y: 10}, {X: 50, Y: 10}, {X: 50, Y: 30}, {X: 31, Y: 30}, {X: 31, Y: 10}}},
	} {
		if _, err := app.Segment.Import(segment.RegionInput{BatchID: batch.ID, ImageID: imgID, Label: "R", Polygon: poly}); err != nil { t.Fatalf("region: %v", err) }
	}
	if _, err := app.AdvanceBatch(batch.ID, model.BatchSegmenting); err != nil { t.Fatalf("segmenting: %v", err) }
	if _, err := app.AdvanceBatch(batch.ID, model.BatchReview); err != nil { t.Fatalf("review: %v", err) }
	handler := httpapi.New(app)
	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/batches/"+strconv.FormatInt(batch.ID, 10)+"/relations/detect", bytes.NewBufferString(`{}`)))
		if rr.Code != http.StatusOK { t.Fatalf("detect #%d status = %d: %s", i+1, rr.Code, rr.Body.String()) }
	}
	relations, err := app.Relation.ListByBatch(batch.ID)
	if err != nil { t.Fatalf("relations: %v", err) }
	if len(relations) != 1 { t.Fatalf("repeated detection created duplicates: %d", len(relations)) }
}
