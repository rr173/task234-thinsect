package review_test

import (
	"path/filepath"
	"testing"

	"task234-thinsect/internal/image"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

func TestBug09FreezingReplacementSupersedesOldVersion(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "bug09.db")); if err != nil { t.Fatalf("open db: %v", err) }; defer db.Close()
	app := service.New(db); batch, err := app.CreateBatch("BUG09", "basalt", "field"); if err != nil { t.Fatalf("batch: %v", err) }
	for _, in := range []image.ImportInput{{BatchID: batch.ID, Name: "ppl", Mode: "PPL", SHA256: "bug09-ppl", Width: 100, Height: 100, AvgBrightness: 100}, {BatchID: batch.ID, Name: "xpl", Mode: "XPL", SHA256: "bug09-xpl", Width: 100, Height: 100, AvgBrightness: 80}} { if _, _, err := app.Image.Import(in); err != nil { t.Fatalf("image: %v", err) } }
	if _, err := app.AdvanceBatch(batch.ID, model.BatchSegmenting); err != nil { t.Fatalf("segmenting: %v", err) }; if _, err := app.AdvanceBatch(batch.ID, model.BatchReview); err != nil { t.Fatalf("review: %v", err) }
	first, err := app.Review.CreateVersion(batch.ID, "v1", "first"); if err != nil { t.Fatalf("v1: %v", err) }; if _, err := app.Review.ShareVersion(first.ID); err != nil { t.Fatalf("share1: %v", err) }; if _, err := app.Review.FreezeVersion(first.ID); err != nil { t.Fatalf("freeze1: %v", err) }
	second, err := app.Review.CreateVersion(batch.ID, "v2", "replacement"); if err != nil { t.Fatalf("v2: %v", err) }; if _, err := app.Review.ShareVersion(second.ID); err != nil { t.Fatalf("share2: %v", err) }; if _, err := app.Review.FreezeVersion(second.ID); err != nil { t.Fatalf("freeze2: %v", err) }
	old, err := app.Review.GetVersion(first.ID); if err != nil { t.Fatalf("old: %v", err) }
	if old.Status != model.VersionSuperseded || old.SupersededBy == nil || *old.SupersededBy != second.ID { t.Fatalf("old frozen version was not superseded: %+v", old) }
}
