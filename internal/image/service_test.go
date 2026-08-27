package image_test

import (
	"errors"
	"path/filepath"
	"testing"

	"task234-thinsect/internal/image"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

func TestImportValidatesSummaryAndIsIdempotent(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "image.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app := service.New(db)
	batch, err := app.CreateBatch("IMG-1", "basalt", "field")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}

	bad := image.ImportInput{BatchID: batch.ID, Name: "ppl", Mode: model.ImageModePPL, SHA256: "bad", Width: 10, Height: 10, AvgBrightness: 256}
	if _, _, err := app.Image.Import(bad); !errors.Is(err, model.ErrValidation) {
		t.Fatalf("out-of-range brightness should be rejected: %v", err)
	}

	in := image.ImportInput{BatchID: batch.ID, Name: "ppl", Mode: model.ImageModePPL, SHA256: "same", Width: 100, Height: 80, AvgBrightness: 120}
	first, duplicate, err := app.Image.Import(in)
	if err != nil || duplicate {
		t.Fatalf("first import = (%v, %v), want created: %v", first, duplicate, err)
	}
	second, duplicate, err := app.Image.Import(image.ImportInput{BatchID: batch.ID, Name: "renamed", Mode: model.ImageModePPL, SHA256: "same", Width: 100, Height: 80, AvgBrightness: 120})
	if err != nil || !duplicate || second.ID != first.ID || second.Name != first.Name {
		t.Fatalf("duplicate import should return original row: first=%v second=%v duplicate=%v err=%v", first, second, duplicate, err)
	}
}
