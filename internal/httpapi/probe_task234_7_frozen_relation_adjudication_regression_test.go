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

func TestBug07FrozenRelationCannotBeAdjudicated(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "bug07.db"))
	if err != nil { t.Fatalf("open db: %v", err) }
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("BUG07", "basalt", "field")
	if err != nil { t.Fatalf("batch: %v", err) }
	var imgID int64
	for _, in := range []image.ImportInput{{BatchID: batch.ID, Name: "ppl", Mode: "PPL", SHA256: "bug07-ppl", Width: 100, Height: 100, AvgBrightness: 100}, {BatchID: batch.ID, Name: "xpl", Mode: "XPL", SHA256: "bug07-xpl", Width: 100, Height: 100, AvgBrightness: 80}} {
		img, _, err := app.Image.Import(in); if err != nil { t.Fatalf("image: %v", err) }; if in.Mode == "PPL" { imgID = img.ID }
	}
	for _, x := range []float64{10, 31} { if _, err := app.Segment.Import(segment.RegionInput{BatchID: batch.ID, ImageID: imgID, Label: "R", Polygon: model.Polygon{Vertices: []model.Point{{X:x,Y:10},{X:x+20,Y:10},{X:x+20,Y:30},{X:x,Y:30},{X:x,Y:10}}}}); err != nil { t.Fatalf("region: %v", err) } }
	if _, err := app.AdvanceBatch(batch.ID, model.BatchSegmenting); err != nil { t.Fatalf("segmenting: %v", err) }
	if _, err := app.AdvanceBatch(batch.ID, model.BatchReview); err != nil { t.Fatalf("review: %v", err) }
	rels, err := app.Relation.Detect(batch.ID); if err != nil { t.Fatalf("detect: %v", err) }; if rels.Total != 1 { t.Fatalf("want one relation, got %+v", rels) }
	items, err := app.Relation.ListByBatch(batch.ID); if err != nil { t.Fatalf("list relation: %v", err) }
	v, err := app.Review.CreateVersion(batch.ID, "v1", "frozen"); if err != nil { t.Fatalf("version: %v", err) }; if _, err := app.Review.ShareVersion(v.ID); err != nil { t.Fatalf("share: %v", err) }; if _, err := app.Review.FreezeVersion(v.ID); err != nil { t.Fatalf("freeze: %v", err) }
	rr := httptest.NewRecorder(); body := bytes.NewBufferString(`{"confirmed":true,"note":"after freeze"}`)
	httpapi.New(app).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/relations/"+strconv.FormatInt(items[0].ID,10)+"/adjudicate", body))
	if rr.Code != http.StatusConflict { t.Fatalf("adjudicate after freeze status = %d, want 409: %s", rr.Code, rr.Body.String()) }
}
