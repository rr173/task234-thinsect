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
	"task234-thinsect/internal/image"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/segment"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

func TestBug10SplitLineageSurvivesRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bug10.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	app := service.New(db)
	batch, err := app.CreateBatch("BUG10", "basalt", "field")
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	img, _, err := app.Image.Import(image.ImportInput{BatchID: batch.ID, Name: "ppl", Mode: model.ImageModePPL, SHA256: "bug10-ppl", Width: 100, Height: 100, AvgBrightness: 100})
	if err != nil {
		t.Fatalf("image: %v", err)
	}
	parent, err := app.Segment.Import(segment.RegionInput{BatchID: batch.ID, ImageID: img.ID, Label: "merged", Polygon: squareBug10(10, 10, 60)})
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	if _, err := app.Segment.MarkMismerged(parent.ID); err != nil {
		t.Fatalf("mark mismerged: %v", err)
	}
	body, _ := json.Marshal(map[string]any{"parts": [][]model.Point{
		{{X: 12, Y: 12}, {X: 32, Y: 12}, {X: 32, Y: 32}, {X: 12, Y: 32}, {X: 12, Y: 12}},
		{{X: 40, Y: 12}, {X: 60, Y: 12}, {X: 60, Y: 32}, {X: 40, Y: 32}, {X: 40, Y: 12}},
	}})
	rr := httptest.NewRecorder()
	httpapi.New(app).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/regions/"+strconv.FormatInt(parent.ID, 10)+"/split", bytes.NewReader(body)))
	if rr.Code != http.StatusCreated {
		t.Fatalf("split status = %d: %s", rr.Code, rr.Body.String())
	}
	var immediate []struct {
		ParentRegionID *int64 `json:"parent_region_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&immediate); err != nil {
		t.Fatalf("decode split: %v", err)
	}
	if len(immediate) != 2 || immediate[0].ParentRegionID == nil || immediate[1].ParentRegionID == nil {
		t.Fatalf("split response lost parent provenance: %+v", immediate)
	}
	if *immediate[0].ParentRegionID != parent.ID || *immediate[1].ParentRegionID != parent.ID {
		t.Fatalf("split response points to wrong parent: %+v", immediate)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	db2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db2.Close()
	children, err := service.New(db2).Regions.ListByBatch(batch.ID)
	if err != nil {
		t.Fatalf("list after restart: %v", err)
	}
	if len(children) != 3 {
		t.Fatalf("expected parent plus two children after restart, got %d", len(children))
	}
	for _, child := range children[1:] {
		if child.ParentRegionID == nil || *child.ParentRegionID != parent.ID {
			t.Fatalf("persisted child lost parent provenance: %+v", child)
		}
	}
}

func squareBug10(x, y, size float64) model.Polygon {
	return model.Polygon{Vertices: []model.Point{{X: x, Y: y}, {X: x + size, Y: y}, {X: x + size, Y: y + size}, {X: x, Y: y + size}, {X: x, Y: y}}}
}
