package smoke_test

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

// TestEndToEnd 验证完整业务闭环：导入→分割→关系→特征→版本冻结→重启恢复。
func TestEndToEnd(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "e2e.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	app := service.New(db)

	batch, err := app.CreateBatch("TS-TEST-001", "安山岩", "五大连池")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if batch.Status != model.BatchImporting {
		t.Fatalf("初始状态应为 importing，实际 %s", batch.Status)
	}

	// 导入 PPL/XPL 图像，重复导入验证幂等。
	if _, _, err := app.Image.Import(imgInput(batch.ID, "a-ppl.jpg", model.ImageModePPL, "hash-ppl", 512, 512, 160, 180, 160, 130)); err != nil {
		t.Fatalf("import ppl: %v", err)
	}
	if _, dup, err := app.Image.Import(imgInput(batch.ID, "a-ppl-2.jpg", model.ImageModePPL, "hash-ppl", 512, 512, 160, 180, 160, 130)); err != nil {
		t.Fatalf("reimport ppl: %v", err)
	} else if !dup {
		t.Fatal("重复导入未幂等")
	}
	if _, _, err := app.Image.Import(imgInput(batch.ID, "a-xpl.jpg", model.ImageModeXPL, "hash-xpl", 512, 512, 80, 80, 76, 70)); err != nil {
		t.Fatalf("import xpl: %v", err)
	}
	imgs, _ := app.Images.ListByBatch(batch.ID)
	if len(imgs) != 2 {
		t.Fatalf("图像应为 2，实际 %d", len(imgs))
	}

	// 导入非法几何：开放环、越界、自交都应被拒绝。
	bad := []PolygonForTest{
		{Verts: [][]float64{{0, 0}, {50, 0}, {50, 50}}}, // 未闭合
		{Verts: [][]float64{{450, 450}, {550, 450}, {550, 550}, {450, 550}, {450, 450}}}, // 越界
		{Verts: [][]float64{{0, 0}, {50, 50}, {50, 0}, {0, 50}, {0, 0}}},                 // 自交
	}
	for i, b := range bad {
		if _, err := app.Segment.Import(segInput(batch.ID, imgs[0].ID, "bad", b.Verts)); err == nil {
			t.Fatalf("非法几何 #%d 应被拒绝", i)
		}
	}

	// 合法区域：大矿物 + 相邻 + 嵌生。
	good := [][][]float64{
		{{10, 10}, {90, 10}, {90, 90}, {10, 90}, {10, 10}},
		{{100, 10}, {190, 10}, {190, 90}, {100, 90}, {100, 10}},
		{{40, 40}, {60, 40}, {60, 60}, {40, 60}, {40, 40}},
	}
	for i, g := range good {
		if _, err := app.Segment.Import(segInput(batch.ID, imgs[0].ID, "R", g)); err != nil {
			t.Fatalf("import region %d: %v", i, err)
		}
	}
	regions, _ := app.Regions.ListByBatch(batch.ID)
	if len(regions) != 3 {
		t.Fatalf("区域应为 3，实际 %d", len(regions))
	}

	// 推进状态机：缺 XPL 时推进 review 应失败——已存在 XPL，故直接推进。
	if _, err := app.AdvanceBatch(batch.ID, model.BatchSegmenting); err != nil {
		t.Fatalf("advance segmenting: %v", err)
	}
	if _, err := app.AdvanceBatch(batch.ID, model.BatchReview); err != nil {
		t.Fatalf("advance review: %v", err)
	}

	// 关系检测：应识别相邻 + 交生。
	res, err := app.Relation.Detect(batch.ID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if res.Total < 2 {
		t.Fatalf("关系应 ≥2，实际 %d", res.Total)
	}

	// 特征计算：消光比应在 (0,1]。
	f, err := app.Feature.Compute(regions[0].ID)
	if err != nil {
		t.Fatalf("compute feature: %v", err)
	}
	if f.ExtinctionRatio <= 0 || f.ExtinctionRatio > 1 {
		t.Fatalf("消光比越界: %v", f.ExtinctionRatio)
	}

	// 人工标注 + 意见。
	if _, err := app.Segment.Label(regions[0].ID, "olivine"); err != nil {
		t.Fatalf("label: %v", err)
	}
	if _, err := app.Review.AddOpinion(regions[0].ID, "cleavage", "两组解理清晰", "tester"); err != nil {
		t.Fatalf("opinion: %v", err)
	}
	if _, err := app.Segment.Label(regions[0].ID, "not-a-mineral"); !errors.Is(err, model.ErrUnknownMineral) {
		t.Fatalf("未知矿物应被拒绝: %v", err)
	}

	// 版本发布：draft→shared→frozen，批次转 published。
	v, err := app.Review.CreateVersion(batch.ID, "v1", "复核完成")
	if err != nil {
		t.Fatalf("create version: %v", err)
	}
	if _, err := app.Review.ShareVersion(v.ID); err != nil {
		t.Fatalf("share: %v", err)
	}
	if _, err := app.Review.FreezeVersion(v.ID); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	b, _ := app.GetBatch(batch.ID)
	if b.Status != model.BatchPublished {
		t.Fatalf("冻结后批次应 published，实际 %s", b.Status)
	}

	// 冻结守卫：修改区域/关系/意见均被拒绝。
	if _, err := app.Segment.Exclude(regions[0].ID); !errors.Is(err, model.ErrFrozenVersion) {
		t.Fatalf("冻结后 exclude 应拒绝: %v", err)
	}
	if _, err := app.Segment.Label(regions[0].ID, "quartz"); !errors.Is(err, model.ErrFrozenVersion) {
		t.Fatalf("冻结后 label 应拒绝: %v", err)
	}
	if _, err := app.Review.AddOpinion(regions[0].ID, "texture", "x", "tester"); !errors.Is(err, model.ErrFrozenVersion) {
		t.Fatalf("冻结后 opinion 应拒绝: %v", err)
	}

	// 重启恢复。
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	db2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	app2 := service.New(db2)
	restored, err := app2.GetBatch(batch.ID)
	if err != nil {
		t.Fatalf("重启后批次丢失: %v", err)
	}
	restoredRegions, _ := app2.Regions.ListByBatch(restored.ID)
	if len(restoredRegions) != 3 {
		t.Fatalf("重启后区域数应为 3，实际 %d", len(restoredRegions))
	}
	if restoredRegions[0].PolygonJSON == "" {
		t.Fatal("重启后区域几何丢失")
	}
}

// PolygonForTest 简化多边形输入。
type PolygonForTest struct {
	Verts [][]float64
}

func imgInput(batchID int64, name, mode, sha string, w, h, bright, cr, cg, cb float64) image.ImportInput {
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

func segInput(batchID, imageID int64, label string, verts [][]float64) segment.RegionInput {
	pts := make([]model.Point, 0, len(verts))
	for _, v := range verts {
		pts = append(pts, model.Point{X: v[0], Y: v[1]})
	}
	return segment.RegionInput{
		BatchID: batchID,
		ImageID: imageID,
		Label:   label,
		Polygon: model.Polygon{Vertices: pts},
	}
}
