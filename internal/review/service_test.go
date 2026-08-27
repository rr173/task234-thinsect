package review_test

import (
	"errors"
	"path/filepath"
	"testing"

	"task234-thinsect/internal/image"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

func TestPublishedBatchCanCreateReplacementVersion(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "review.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("REV-1", "basalt", "field")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	for _, in := range []image.ImportInput{
		{BatchID: batch.ID, Name: "ppl", Mode: model.ImageModePPL, SHA256: "rev-ppl", Width: 100, Height: 100, AvgBrightness: 100},
		{BatchID: batch.ID, Name: "xpl", Mode: model.ImageModeXPL, SHA256: "rev-xpl", Width: 100, Height: 100, AvgBrightness: 80},
	} {
		if _, _, err := app.Image.Import(in); err != nil {
			t.Fatalf("import image: %v", err)
		}
	}
	if _, err := app.AdvanceBatch(batch.ID, model.BatchSegmenting); err != nil {
		t.Fatalf("advance segmenting: %v", err)
	}
	if _, err := app.AdvanceBatch(batch.ID, model.BatchReview); err != nil {
		t.Fatalf("advance review: %v", err)
	}

	first, err := app.Review.CreateVersion(batch.ID, "v1", "first")
	if err != nil {
		t.Fatalf("create first version: %v", err)
	}
	if _, err := app.Review.ShareVersion(first.ID); err != nil {
		t.Fatalf("share first: %v", err)
	}
	if _, err := app.Review.FreezeVersion(first.ID); err != nil {
		t.Fatalf("freeze first: %v", err)
	}

	second, err := app.Review.CreateVersion(batch.ID, "v2", "replacement")
	if err != nil {
		t.Fatalf("published batch should accept replacement draft: %v", err)
	}
	if _, err := app.Review.ShareVersion(second.ID); err != nil {
		t.Fatalf("share replacement: %v", err)
	}
	if _, err := app.Review.FreezeVersion(second.ID); err != nil {
		t.Fatalf("freeze replacement: %v", err)
	}
	old, err := app.Review.GetVersion(first.ID)
	if err != nil {
		t.Fatalf("get old version: %v", err)
	}
	if old.Status != model.VersionSuperseded || old.SupersededBy == nil || *old.SupersededBy != second.ID {
		t.Fatalf("old version should record replacement: %+v", old)
	}
	if _, err := app.Review.AddOpinion(99999, "texture", "x", "tester"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("missing region should remain a not-found error: %v", err)
	}
}
