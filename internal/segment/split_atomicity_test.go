package segment_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"task234-thinsect/internal/image"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/segment"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

func TestSplitRollsBackWhenLaterChildInsertFails(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "split-atomic.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("SPLIT-ATOMIC", "basalt", "field")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	img, _, err := app.Image.Import(image.ImportInput{BatchID: batch.ID, Name: "ppl", Mode: model.ImageModePPL, SHA256: "split-ppl", Width: 100, Height: 100, AvgBrightness: 100})
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
	if _, err := db.Exec(`CREATE TRIGGER fail_second_split BEFORE INSERT ON regions WHEN NEW.label='merged-2' BEGIN SELECT RAISE(ABORT, 'forced split failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	parts := []model.Polygon{squarePolygon(12, 12, 20), squarePolygon(40, 12, 20)}
	if _, err := app.Segment.Split(segment.SplitInput{RegionID: parent.ID, Parts: parts}); err == nil {
		t.Fatal("second child failure should be returned")
	}
	regions, err := app.Regions.ListByBatch(batch.ID)
	if err != nil {
		t.Fatalf("list regions: %v", err)
	}
	if len(regions) != 1 || regions[0].ID != parent.ID {
		t.Fatalf("split failure left partial children: %+v", regions)
	}
	if regions[0].Status != model.RegionMismerged {
		t.Fatalf("parent evidence status should remain mismerged: %s", regions[0].Status)
	}
	var triggerCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name='fail_second_split'`).Scan(&triggerCount); err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("check trigger: %v", err)
	}
}

// TestSplitPreservesParentProvenanceAcrossReopen 验证误合并拆分产生的子区域在
// 返回结果与持久化中均保留来源区域编号；关闭数据库再打开后仍可据此追溯原区域。
func TestSplitPreservesParentProvenanceAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "split-prov.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	app := service.New(db)
	batch, err := app.CreateBatch("SPLIT-PROV", "basalt", "field")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	img, _, err := app.Image.Import(image.ImportInput{BatchID: batch.ID, Name: "ppl", Mode: model.ImageModePPL, SHA256: "prov-ppl", Width: 100, Height: 100, AvgBrightness: 100})
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
	parts := []model.Polygon{squarePolygon(12, 12, 20), squarePolygon(40, 12, 20)}
	created, err := app.Segment.Split(segment.SplitInput{RegionID: parent.ID, Parts: parts})
	if err != nil || len(created) != 2 {
		t.Fatalf("valid split should create two children: count=%d err=%v", len(created), err)
	}
	// 接口返回的子区域必须携带来源编号，可直接追溯原区域。
	for _, child := range created {
		if child.ParentRegionID == nil || *child.ParentRegionID != parent.ID {
			t.Fatalf("returned child missing parent provenance: %+v", child)
		}
	}

	// 关闭数据库再打开，验证来源编号已落盘且可追溯。
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	db2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db2.Close()
	app2 := service.New(db2)
	restored, err := app2.Regions.ListByBatch(batch.ID)
	if err != nil {
		t.Fatalf("list after reopen: %v", err)
	}
	var children []model.Region
	for _, r := range restored {
		if r.ParentRegionID != nil && *r.ParentRegionID == parent.ID {
			children = append(children, r)
		}
	}
	if len(children) != 2 {
		t.Fatalf("after reopen expected 2 children pointing at parent, got %d", len(children))
	}
	// 通过来源编号能追溯到原区域本身。
	origin, err := app2.Segment.Get(parent.ID)
	if err != nil {
		t.Fatalf("trace origin after reopen: %v", err)
	}
	if origin.Status != model.RegionMismerged {
		t.Fatalf("origin region should remain mismerged evidence: %s", origin.Status)
	}
}
