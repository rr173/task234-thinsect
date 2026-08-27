package segment_test

import (
	"errors"
	"path/filepath"
	"testing"

	"task234-thinsect/internal/image"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/segment"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

// TestImportRejectsCrossBatchRegion 锁定：请求携带的批次编号与图像实际所属批次
// 不一致时，必须按非法引用拒绝写入，区域始终归属于图像所在的批次。
func TestImportRejectsCrossBatchRegion(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "crossbatch.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app := service.New(db)
	batchA, err := app.CreateBatch("CB-A", "basalt", "field")
	if err != nil {
		t.Fatalf("create batch A: %v", err)
	}
	batchB, err := app.CreateBatch("CB-B", "andesite", "field")
	if err != nil {
		t.Fatalf("create batch B: %v", err)
	}
	// 图像属于 batchA。
	img, _, err := app.Image.Import(image.ImportInput{
		BatchID: batchA.ID, Name: "ppl", Mode: model.ImageModePPL, SHA256: "cb-ppl",
		Width: 100, Height: 100, AvgBrightness: 100,
	})
	if err != nil {
		t.Fatalf("import image: %v", err)
	}
	// 请求谎报区域属于 batchB（跨批次引用）——必须被拒绝。
	if _, err := app.Segment.Import(segment.RegionInput{
		BatchID: batchB.ID, ImageID: img.ID, Label: "cross", Polygon: squarePolygon(10, 10, 40),
	}); !errors.Is(err, model.ErrValidation) {
		t.Fatalf("跨批次区域应被拒绝，实际 err=%v", err)
	}
	// 正确归属（batchA）应被接受，且写入区域的 batch_id 必须等于图像所在批次。
	rg, err := app.Segment.Import(segment.RegionInput{
		BatchID: batchA.ID, ImageID: img.ID, Label: "ok", Polygon: squarePolygon(10, 10, 40),
	})
	if err != nil {
		t.Fatalf("import region: %v", err)
	}
	if rg.BatchID != batchA.ID {
		t.Fatalf("区域应归属于图像所在批次 batchA，实际 batch_id=%d", rg.BatchID)
	}
	// 确认没有跨批次区域被持久化。
	regionsB, err := app.Regions.ListByBatch(batchB.ID)
	if err != nil {
		t.Fatalf("list batchB regions: %v", err)
	}
	if len(regionsB) != 0 {
		t.Fatalf("batchB 不应残留跨批次区域，实际 %d", len(regionsB))
	}
}

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
