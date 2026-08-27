package relation_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"task234-thinsect/internal/image"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/relation"
	"task234-thinsect/internal/segment"
	"task234-thinsect/internal/service"
	"task234-thinsect/internal/store"
)

// TestDetectIsIdempotentOnRetry 验证重复执行同一批次的关系检测不会产生重复关系：
// 同一对区域、同一种关系在 relationships 表中只能保留一条记录（检测可安全重试）。
func TestDetectIsIdempotentOnRetry(t *testing.T) {
	app, batchID := setupRelationFixture(t)

	first, err := app.Relation.Detect(batchID)
	if err != nil {
		t.Fatalf("first detect: %v", err)
	}
	if first.Total < 2 {
		t.Fatalf("首次检测应至少产出 2 条关系（相邻+交生），实际 %d", first.Total)
	}

	// 重复检测：不应新增，应全部命中既有关系。
	second, err := app.Relation.Detect(batchID)
	if err != nil {
		t.Fatalf("retry detect: %v", err)
	}
	if second.Total != 0 {
		t.Fatalf("重试不应新增关系，实际新增 %d", second.Total)
	}
	if second.AlreadyKnown != first.Total {
		t.Fatalf("重试应将全部既有关系计为 already_known，实际 %d（期望 %d）",
			second.AlreadyKnown, first.Total)
	}

	rels, err := app.Rels.ListByBatch(batchID)
	if err != nil {
		t.Fatalf("list rels: %v", err)
	}
	if len(rels) != first.Total {
		t.Fatalf("关系总数应与首次检测一致，实际 %d（期望 %d）", len(rels), first.Total)
	}
	assertNoDuplicateRels(t, rels)

	// 多轮重试后断言无重复：同对区域同类型只能出现一次。
	for i := 0; i < 3; i++ {
		if _, err := app.Relation.Detect(batchID); err != nil {
			t.Fatalf("extra detect %d: %v", i, err)
		}
	}
	rels, _ = app.Rels.ListByBatch(batchID)
	if len(rels) != first.Total {
		t.Fatalf("多轮重试后关系数应保持稳定，实际 %d（期望 %d）", len(rels), first.Total)
	}
	assertNoDuplicateRels(t, rels)
}

// TestDetectRetryPreservesAdjudicatedStatus 验证重试检测不会覆盖已裁决的状态：
// 既有关系被跳过，confirmed 状态与原关系 ID 得以保留，不被新建的初始态重复覆盖。
func TestDetectRetryPreservesAdjudicatedStatus(t *testing.T) {
	app, batchID := setupRelationFixture(t)

	if _, err := app.Relation.Detect(batchID); err != nil {
		t.Fatalf("detect: %v", err)
	}
	rels, _ := app.Rels.ListByBatch(batchID)
	// 两相邻大区域 + 嵌生小晶体 → 相邻 + 交生两条关系。
	var rel model.Relationship
	for _, r := range rels {
		if r.Kind == model.RelationAdjacent {
			rel = r
		}
	}
	if rel.ID == 0 {
		t.Fatalf("未找到相邻关系，实际 %+v", rels)
	}
	if rel.Status != model.RelationAdjacent {
		t.Fatalf("相邻关系初始状态应为 adjacent，实际 %s", rel.Status)
	}

	// 裁决确认后，重试检测应跳过该关系、保留 confirmed。
	if _, err := app.Relation.Adjudicate(relation.AdjudicateInput{
		RelationID: rel.ID, Confirmed: true, Note: "已确认边界共享",
	}); err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	relCountBefore := len(rels)
	if _, err := app.Relation.Detect(batchID); err != nil {
		t.Fatalf("retry detect after adjudicate: %v", err)
	}
	rels, _ = app.Rels.ListByBatch(batchID)
	if len(rels) != relCountBefore {
		t.Fatalf("裁决后重试不应新增关系，实际 %d（期望 %d）", len(rels), relCountBefore)
	}
	var confirmedRel model.Relationship
	for _, r := range rels {
		if r.ID == rel.ID {
			confirmedRel = r
		}
	}
	if confirmedRel.Status != model.RelationConfirmed {
		t.Fatalf("重试应保留 confirmed 状态，实际 %s", confirmedRel.Status)
	}
}

// setupRelationFixture 建立一个进入待复核的批次：两相邻大区域 + 一嵌生小晶体。
func setupRelationFixture(t *testing.T) (*service.App, int64) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "relation.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := service.New(db)

	batch, err := app.CreateBatch("REL-FIX", "basalt", "field")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	ppl, _, err := app.Image.Import(image.ImportInput{
		BatchID: batch.ID, Name: "ppl", Mode: model.ImageModePPL, SHA256: "rel-ppl",
		Width: 512, Height: 512, AvgBrightness: 100,
	})
	if err != nil {
		t.Fatalf("import ppl: %v", err)
	}
	if _, _, err := app.Image.Import(image.ImportInput{
		BatchID: batch.ID, Name: "xpl", Mode: model.ImageModeXPL, SHA256: "rel-xpl",
		Width: 512, Height: 512, AvgBrightness: 50,
	}); err != nil {
		t.Fatalf("import xpl: %v", err)
	}
	for i, p := range []model.Polygon{
		squarePolygon(10, 10, 80),
		squarePolygon(100, 10, 80),
		squarePolygon(40, 40, 20),
	} {
		if _, err := app.Segment.Import(segment.RegionInput{
			BatchID: batch.ID, ImageID: ppl.ID, Label: "R", Polygon: p,
		}); err != nil {
			t.Fatalf("import region %d: %v", i, err)
		}
	}
	if _, err := app.AdvanceBatch(batch.ID, model.BatchSegmenting); err != nil {
		t.Fatalf("advance segmenting: %v", err)
	}
	if _, err := app.AdvanceBatch(batch.ID, model.BatchReview); err != nil {
		t.Fatalf("advance review: %v", err)
	}
	return app, batch.ID
}

func squarePolygon(x, y, size float64) model.Polygon {
	return model.Polygon{Vertices: []model.Point{
		{X: x, Y: y}, {X: x + size, Y: y}, {X: x + size, Y: y + size}, {X: x, Y: y + size}, {X: x, Y: y},
	}}
}

// assertNoDuplicateRels 断言关系列表中无重复（同对区域 + 同类型）。
func assertNoDuplicateRels(t *testing.T, rels []model.Relationship) {
	t.Helper()
	seen := make(map[string]bool)
	for _, r := range rels {
		key := relKey(r.RegionA, r.RegionB, r.Kind)
		if seen[key] {
			t.Fatalf("检测到重复关系: %s", key)
		}
		seen[key] = true
	}
}

func relKey(a, b int64, kind string) string {
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("%d-%d|%s", a, b, kind)
}
