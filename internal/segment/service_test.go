package segment_test

import (
	"path/filepath"
	"testing"

	"task234-thinsect/internal/image"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/segment"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

func TestSplitPrevalidatesAllPartsBeforeWriting(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "segment.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("SEG-1", "basalt", "field")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	img, _, err := app.Image.Import(image.ImportInput{BatchID: batch.ID, Name: "ppl", Mode: model.ImageModePPL, SHA256: "seg-ppl", Width: 100, Height: 100, AvgBrightness: 100})
	if err != nil {
		t.Fatalf("import image: %v", err)
	}
	parent, err := app.Segment.Import(segment.RegionInput{BatchID: batch.ID, ImageID: img.ID, Label: "merged", Polygon: squarePolygon(10, 10, 60)})
	if err != nil {
		t.Fatalf("import parent: %v", err)
	}
	if _, err := app.Segment.MarkMismerged(parent.ID); err != nil {
		t.Fatalf("mark mismerged: %v", err)
	}

	badParts := []model.Polygon{squarePolygon(12, 12, 20), {Vertices: []model.Point{{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 20, Y: 20}}}}
	if _, err := app.Segment.Split(segment.SplitInput{RegionID: parent.ID, Parts: badParts}); err == nil {
		t.Fatal("invalid child geometry should reject the whole split")
	}
	children, err := app.Regions.ListByBatch(batch.ID)
	if err != nil {
		t.Fatalf("list regions: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("failed split must not create partial children: %+v", children)
	}

	goodParts := []model.Polygon{squarePolygon(12, 12, 20), squarePolygon(40, 12, 20)}
	created, err := app.Segment.Split(segment.SplitInput{RegionID: parent.ID, Parts: goodParts})
	if err != nil || len(created) != 2 {
		t.Fatalf("valid split should create two children: count=%d err=%v", len(created), err)
	}
	for _, child := range created {
		if child.ParentRegionID == nil || *child.ParentRegionID != parent.ID || child.Status != model.RegionCandidate {
			t.Fatalf("child provenance/status missing: %+v", child)
		}
	}
}

func squarePolygon(x, y, size float64) model.Polygon {
	return model.Polygon{Vertices: []model.Point{{X: x, Y: y}, {X: x + size, Y: y}, {X: x + size, Y: y + size}, {X: x, Y: y + size}, {X: x, Y: y}}}
}
