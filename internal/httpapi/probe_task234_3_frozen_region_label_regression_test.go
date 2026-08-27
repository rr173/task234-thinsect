package httpapi_test

import (
	"bytes"
	"encoding/json"
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

func TestBug03FrozenRegionCannotBeRelabeled(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "bug03.db"))
	if err != nil { t.Fatalf("open db: %v", err) }
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("BUG03", "basalt", "field")
	if err != nil { t.Fatalf("create batch: %v", err) }
	for _, in := range []image.ImportInput{
		{BatchID: batch.ID, Name: "ppl", Mode: "PPL", SHA256: "bug03-ppl", Width: 100, Height: 100, AvgBrightness: 100},
		{BatchID: batch.ID, Name: "xpl", Mode: "XPL", SHA256: "bug03-xpl", Width: 100, Height: 100, AvgBrightness: 80},
	} {
		if _, _, err := app.Image.Import(in); err != nil { t.Fatalf("import image: %v", err) }
	}
	if _, err := app.AdvanceBatch(batch.ID, model.BatchSegmenting); err != nil { t.Fatalf("segmenting: %v", err) }
	if _, err := app.AdvanceBatch(batch.ID, model.BatchReview); err != nil { t.Fatalf("review: %v", err) }
	region, err := app.Segment.Import(segment.RegionInput{BatchID: batch.ID, ImageID: 1, Label: "R1", Polygon: squareBug3()})
	if err != nil { t.Fatalf("region: %v", err) }
	version, err := app.Review.CreateVersion(batch.ID, "v1", "frozen")
	if err != nil { t.Fatalf("version: %v", err) }
	if _, err := app.Review.ShareVersion(version.ID); err != nil { t.Fatalf("share: %v", err) }
	if _, err := app.Review.FreezeVersion(version.ID); err != nil { t.Fatalf("freeze: %v", err) }

	body, _ := json.Marshal(map[string]string{"mineral_code": "quartz"})
	rr := httptest.NewRecorder()
	httpapi.New(app).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/regions/"+strconv.FormatInt(region.ID, 10)+"/label", bytes.NewReader(body)))
	if rr.Code != http.StatusConflict { t.Fatalf("label after freeze status = %d, want 409: %s", rr.Code, rr.Body.String()) }
	stored, err := app.Segment.Get(region.ID)
	if err != nil { t.Fatalf("get region: %v", err) }
	if stored.MineralCode != "" { t.Fatalf("frozen region was modified: %+v", stored) }
}

func squareBug3() model.Polygon {
	return model.Polygon{Vertices: []model.Point{{X: 10, Y: 10}, {X: 30, Y: 10}, {X: 30, Y: 30}, {X: 10, Y: 30}, {X: 10, Y: 10}}}
}
