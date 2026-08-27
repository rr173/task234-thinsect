// Package smoke 实现 --smoke-test 契约：真实创建数据、跑通核心闭环、
// 验证冻结守卫与重启恢复，最后以 0 退出码结束。
package smoke

import (
	"errors"
	"fmt"

	"task234-thinsect/internal/image"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/segment"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

// Run 执行端到端自检。dbPath 为数据库文件路径（同时用于重启恢复验证）。
func Run(dbPath string) error {
	summary := &report{}
	if err := phase1(summary, dbPath); err != nil {
		return err
	}
	if err := phase2(summary, dbPath); err != nil {
		return err
	}
	printReport(summary)
	return nil
}

type report struct {
	batchCode string
	batchID   int64
	images    int
	regions   int
	rels      int
	features  int
	opinions  int
	versionID int64
	frozenOK  bool
	restoreOK bool
}

// phase1 创建数据并跑通核心闭环（导入→分割→特征→关系→意见→版本冻结）。
func phase1(r *report, dbPath string) error {
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	app := service.New(db)

	// 1. 创建批次。
	batch, err := app.CreateBatch("TS-SMOKE-234", "玄武岩", "长白山天池")
	if err != nil {
		return fmt.Errorf("create batch: %w", err)
	}
	r.batchCode = batch.Code
	r.batchID = batch.ID

	// 2. 导入成对偏光图像（PPL/XPL），各重复导入一次验证哈希幂等。
	if _, _, err := app.Image.Import(serviceImageInput(batch.ID, "ppl.jpg", model.ImageModePPL,
		"sha-ppl-234", 512, 512, 180, 200, 180, 150)); err != nil {
		return fmt.Errorf("import ppl: %w", err)
	}
	if _, dedup, err := app.Image.Import(serviceImageInput(batch.ID, "ppl-copy.jpg", model.ImageModePPL,
		"sha-ppl-234", 512, 512, 180, 200, 180, 150)); err != nil {
		return fmt.Errorf("reimport ppl: %w", err)
	} else if !dedup {
		return errors.New("ppl 图像重复导入未幂等去重")
	}
	if _, _, err := app.Image.Import(serviceImageInput(batch.ID, "xpl.jpg", model.ImageModeXPL,
		"sha-xpl-234", 512, 512, 90, 90, 85, 80)); err != nil {
		return fmt.Errorf("import xpl: %w", err)
	}
	imgs, _ := app.Images.ListByBatch(batch.ID)
	if len(imgs) != 2 {
		return fmt.Errorf("期望 2 条图像摘要（幂等去重），实际 %d", len(imgs))
	}
	r.images = len(imgs)

	// 3. 导入三个区域：R1 大矿物、R2 相邻矿物、R3 嵌生小晶体。
	regions := []struct {
		label string
		verts [][]float64
	}{
		{"R1", [][]float64{{10, 10}, {90, 10}, {90, 90}, {10, 90}, {10, 10}}},
		{"R2", [][]float64{{100, 10}, {190, 10}, {190, 90}, {100, 90}, {100, 10}}},
		{"R3", [][]float64{{40, 40}, {60, 40}, {60, 60}, {40, 60}, {40, 40}}},
	}
	pplID := imgs[0].ID
	for _, rg := range regions {
		verts := make([]model.Point, 0, len(rg.verts))
		for _, v := range rg.verts {
			verts = append(verts, model.Point{X: v[0], Y: v[1]})
		}
		if _, err := app.Segment.Import(segmentInput(batch.ID, pplID, rg.label, verts)); err != nil {
			return fmt.Errorf("import region %s: %w", rg.label, err)
		}
	}
	all, _ := app.Regions.ListByBatch(batch.ID)
	if len(all) != 3 {
		return fmt.Errorf("期望 3 个区域，实际 %d", len(all))
	}
	r.regions = len(all)

	// 4. 推进批次进入待复核，检测关系。
	if _, err := app.AdvanceBatch(batch.ID, model.BatchSegmenting); err != nil {
		return fmt.Errorf("advance segmenting: %w", err)
	}
	if _, err := app.AdvanceBatch(batch.ID, model.BatchReview); err != nil {
		return fmt.Errorf("advance review: %w", err)
	}
	detect, err := app.Relation.Detect(batch.ID)
	if err != nil {
		return fmt.Errorf("detect relations: %w", err)
	}
	if detect.AdjacentAdded+detect.IntergrowthAdded < 2 {
		return fmt.Errorf("期望至少 2 条关系（相邻+交生），实际 %d", detect.Total)
	}
	rels, _ := app.Rels.ListByBatch(batch.ID)
	r.rels = len(rels)

	// 5. 计算特征并生成候选标签。
	for _, rg := range all {
		f, err := app.Feature.Compute(rg.ID)
		if err != nil {
			return fmt.Errorf("compute features %s: %w", rg.Label, err)
		}
		if f.ExtinctionRatio <= 0 || f.ExtinctionRatio > 1 {
			return fmt.Errorf("区域 %s 消光比越界: %v", rg.Label, f.ExtinctionRatio)
		}
		r.features++
	}
	// 人工复核：R1 标注橄榄石（与特征候选可能不同，人工优先）。
	region1 := all[0]
	if _, err := app.Segment.Label(region1.ID, "olivine"); err != nil {
		return fmt.Errorf("label region: %w", err)
	}

	// 6. 补充显微证据（消光观察）。
	if _, err := app.Review.AddOpinion(region1.ID, "extinction", "旋转载物台 90° 出现两次消光，斜消光角约 10°", "petrologist-a"); err != nil {
		return fmt.Errorf("add opinion: %w", err)
	}
	ops, _ := app.Opinions.ListByRegion(region1.ID)
	r.opinions = len(ops)

	// 7. 创建解释版本并冻结（同一批次串行发布）。
	v, err := app.Review.CreateVersion(batch.ID, "v1", "复核完成，主矿物橄榄石与辉石相邻，含嵌生晶体")
	if err != nil {
		return fmt.Errorf("create version: %w", err)
	}
	if _, err := app.Review.ShareVersion(v.ID); err != nil {
		return fmt.Errorf("share version: %w", err)
	}
	frozen, err := app.Review.FreezeVersion(v.ID)
	if err != nil {
		return fmt.Errorf("freeze version: %w", err)
	}
	if frozen.Status != model.VersionFrozen {
		return fmt.Errorf("版本未冻结: %s", frozen.Status)
	}
	r.versionID = frozen.ID

	// 8. 冻结守卫：冻结后修改区域必须被拒绝。
	before := frozen.Status
	if _, err := app.Segment.Exclude(region1.ID); !errors.Is(err, model.ErrFrozenVersion) {
		return fmt.Errorf("冻结后修改区域未拒绝: %v", err)
	}
	_ = before
	r.frozenOK = true

	// 批次应为已发布。
	b, _ := app.GetBatch(batch.ID)
	if b.Status != model.BatchPublished {
		return fmt.Errorf("冻结后批次应进入 published，实际 %s", b.Status)
	}
	return db.Close()
}

// phase2 重开同一数据库验证持久化与重启恢复。
func phase2(r *report, dbPath string) error {
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("reopen db: %w", err)
	}
	defer db.Close()
	app := service.New(db)
	batch, err := app.GetBatch(r.batchID)
	if err != nil {
		return fmt.Errorf("重启后批次丢失: %w", err)
	}
	imgs, _ := app.Images.ListByBatch(batch.ID)
	regions, _ := app.Regions.ListByBatch(batch.ID)
	rels, _ := app.Rels.ListByBatch(batch.ID)
	versions, _ := app.Versions.ListByBatch(batch.ID)
	if len(imgs) != r.images || len(regions) != r.regions || len(rels) != r.rels {
		return fmt.Errorf("重启后数据不一致: images=%d regions=%d rels=%d",
			len(imgs), len(regions), len(rels))
	}
	if len(versions) != 1 || versions[0].Status != model.VersionFrozen {
		return fmt.Errorf("重启后冻结版本状态异常: %+v", versions)
	}
	// 冻结区域几何在重启后仍可解析（不可变快照语义）。
	if len(regions) > 0 {
		if regions[0].PolygonJSON == "" || len(regions[0].Polygon.Vertices) == 0 {
			return errors.New("重启后区域几何丢失")
		}
	}
	r.restoreOK = true
	return nil
}

func printReport(r *report) {
	fmt.Printf("SMOKE PASS — 火山岩薄片矿物边界复核台\n")
	fmt.Printf("  批次: %s (id=%d)\n", r.batchCode, r.batchID)
	fmt.Printf("  图像摘要: %d (哈希幂等去重 ✓)\n", r.images)
	fmt.Printf("  区域: %d\n", r.regions)
	fmt.Printf("  特征计算: %d 个区域\n", r.features)
	fmt.Printf("  矿物关系: %d 条（相邻/交生检测 ✓）\n", r.rels)
	fmt.Printf("  显微证据意见: %d 条\n", r.opinions)
	fmt.Printf("  解释版本: id=%d 已冻结\n", r.versionID)
	fmt.Printf("  冻结守卫: 冻结后修改被拒绝 ✓\n")
	fmt.Printf("  重启恢复: 数据完整 ✓\n")
}

// serviceImageInput 构造图像导入参数。
func serviceImageInput(batchID int64, name, mode, sha string, w, h, bright, cr, cg, cb float64) image.ImportInput {
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

func segmentInput(batchID, imageID int64, label string, verts []model.Point) segment.RegionInput {
	return segment.RegionInput{
		BatchID: batchID,
		ImageID: imageID,
		Label:   label,
		Polygon: model.Polygon{Vertices: verts},
	}
}
