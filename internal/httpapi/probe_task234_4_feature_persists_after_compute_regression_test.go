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

func TestBug04ComputedFeatureIsVisibleFromStoredEndpoint(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "bug04.db"))
	if err != nil { t.Fatalf("open db: %v", err) }
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("BUG04", "basalt", "field")
	if err != nil { t.Fatalf("batch: %v", err) }
	var imgID int64
	for _, in := range []image.ImportInput{
		{BatchID: batch.ID, Name: "ppl", Mode: "PPL", SHA256: "bug04-ppl", Width: 100, Height: 100, AvgBrightness: 120, ColorR: 180, ColorG: 150, ColorB: 100},
		{BatchID: batch.ID, Name: "xpl", Mode: "XPL", SHA256: "bug04-xpl", Width: 100, Height: 100, AvgBrightness: 80},
	} {
		img, _, err := app.Image.Import(in)
		if err != nil { t.Fatalf("image: %v", err) }
		if in.Mode == "PPL" { imgID = img.ID }
	}
	region, err := app.Segment.Import(segment.RegionInput{BatchID: batch.ID, ImageID: imgID, Label: "R1", Polygon: model.Polygon{Vertices: []model.Point{{X: 10, Y: 10}, {X: 40, Y: 10}, {X: 40, Y: 40}, {X: 10, Y: 40}, {X: 10, Y: 10}}}})
	if err != nil { t.Fatalf("region: %v", err) }
	handler := httpapi.New(app)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/regions/"+strconv.FormatInt(region.ID, 10)+"/features", bytes.NewBufferString(`{}`)))
	if rr.Code != http.StatusOK { t.Fatalf("compute status = %d: %s", rr.Code, rr.Body.String()) }
	var computed struct{ ExtinctionRatio float64 `json:"extinction_ratio"`; AvgR float64 `json:"avg_r"` }
	if err := json.NewDecoder(rr.Body).Decode(&computed); err != nil { t.Fatalf("decode compute: %v", err) }
	stored := httptest.NewRecorder()
	handler.ServeHTTP(stored, httptest.NewRequest(http.MethodGet, "/api/regions/"+strconv.FormatInt(region.ID, 10)+"/features", nil))
	if stored.Code != http.StatusOK { t.Fatalf("stored feature status = %d: %s", stored.Code, stored.Body.String()) }
	var got struct{ ExtinctionRatio float64 `json:"extinction_ratio"`; AvgR float64 `json:"avg_r"` }
	if err := json.NewDecoder(stored.Body).Decode(&got); err != nil { t.Fatalf("decode stored: %v", err) }
	if got.ExtinctionRatio != computed.ExtinctionRatio || got.AvgR != computed.AvgR {
		t.Fatalf("computed feature was not persisted: computed=%+v stored=%+v", computed, got)
	}
}
