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
