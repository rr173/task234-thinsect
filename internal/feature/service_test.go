package feature_test

import (
	"path/filepath"
	"testing"

	"task234-thinsect/internal/image"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/segment"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

// TestComputePersistsFeaturesAcrossReadsAndRestart 验证 Compute 计算的特征
// 被持久化：同一批次的后续查询与重启恢复后读到的值必须与计算结果一致。
func TestComputePersistsFeaturesAcrossReadsAndRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "feature.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	app := service.New(db)
	batch, err := app.CreateBatch("TS-FEAT-001", "玄武岩", "长白山")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if _, _, err := app.Image.Import(imgIn(batch.ID, "ppl.jpg", model.ImageModePPL, "sha-feat-ppl", 512, 512, 160, 180, 160, 130)); err != nil {
		t.Fatalf("import ppl: %v", err)
	}
	if _, _, err := app.Image.Import(imgIn(batch.ID, "xpl.jpg", model.ImageModeXPL, "sha-feat-xpl", 512, 512, 80, 80, 76, 70)); err != nil {
		t.Fatalf("import xpl: %v", err)
	}
	verts := []model.Point{{10, 10}, {90, 10}, {90, 90}, {10, 90}, {10, 10}}
	region, err := app.Segment.Import(segment.RegionInput{
		BatchID: batch.ID,
		ImageID: pplImageID(t, app, batch.ID),
		Label:   "R1",
		Polygon: model.Polygon{Vertices: verts},
	})
	if err != nil {
		t.Fatalf("import region: %v", err)
	}

	// 推进到待复核后才能计算特征（需成对 PPL/XPL）。
	if _, err := app.AdvanceBatch(batch.ID, model.BatchSegmenting); err != nil {
		t.Fatalf("advance segmenting: %v", err)
	}
	if _, err := app.AdvanceBatch(batch.ID, model.BatchReview); err != nil {
		t.Fatalf("advance review: %v", err)
	}

	computed, err := app.Feature.Compute(region.ID)
	if err != nil {
		t.Fatalf("compute features: %v", err)
	}
	if computed.ExtinctionRatio <= 0 || computed.ExtinctionRatio > 1 {
		t.Fatalf("消光比越界: %v", computed.ExtinctionRatio)
	}

	// 1. 同一批次的后续查询应读到刚写入的值（而非零值或旧占位值）。
	got, err := app.Regions.Get(region.ID)
	if err != nil {
		t.Fatalf("get region after compute: %v", err)
	}
	if got.AvgR != computed.AvgR || got.AvgG != computed.AvgG || got.AvgB != computed.AvgB {
		t.Fatalf("颜色未持久化: got R=%.2f G=%.2f B=%.2f, want R=%.2f G=%.2f B=%.2f",
			got.AvgR, got.AvgG, got.AvgB, computed.AvgR, computed.AvgG, computed.AvgB)
	}
	if got.ExtinctionRatio != computed.ExtinctionRatio || got.ExtAngle != computed.ExtAngle {
		t.Fatalf("消光特征未持久化: got ratio=%v angle=%v, want ratio=%v angle=%v",
			got.ExtinctionRatio, got.ExtAngle, computed.ExtinctionRatio, computed.ExtAngle)
	}

	// 2. 重启恢复后特征仍应一致。
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	db2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db2.Close()
	app2 := service.New(db2)
	restored, err := app2.Regions.Get(region.ID)
	if err != nil {
		t.Fatalf("get region after restart: %v", err)
	}
	if restored.AvgR != computed.AvgR || restored.AvgG != computed.AvgG || restored.AvgB != computed.AvgB {
		t.Fatalf("重启后颜色丢失: got R=%.2f G=%.2f B=%.2f, want R=%.2f G=%.2f B=%.2f",
			restored.AvgR, restored.AvgG, restored.AvgB, computed.AvgR, computed.AvgG, computed.AvgB)
	}
	if restored.ExtinctionRatio != computed.ExtinctionRatio || restored.ExtAngle != computed.ExtAngle {
		t.Fatalf("重启后消光特征丢失: got ratio=%v angle=%v, want ratio=%v angle=%v",
			restored.ExtinctionRatio, restored.ExtAngle, computed.ExtinctionRatio, computed.ExtAngle)
	}
}

// pplImageID 返回批次下 PPL 图像的 ID。
func pplImageID(t *testing.T, app *service.App, batchID int64) int64 {
	t.Helper()
	imgs, err := app.Images.ListByBatch(batchID)
	if err != nil {
		t.Fatalf("list images: %v", err)
	}
	for _, im := range imgs {
		if im.Mode == model.ImageModePPL {
			return im.ID
		}
	}
	t.Fatalf("no PPL image in batch %d", batchID)
	return 0
}

func imgIn(batchID int64, name, mode, sha string, w, h, bright, cr, cg, cb float64) image.ImportInput {
	return image.ImportInput{
		BatchID:       batchID,
		Name:          name,
		Mode:          mode,
		SHA256:        sha,
		Width:         w,
		Height:        h,
		AvgBrightness: bright,
		ColorR:        cr,
		ColorG:        cg,
		ColorB:        cb,
	}
}
