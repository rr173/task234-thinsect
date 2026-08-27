package model

import (
	"math"
	"testing"
)

func square(x, y, size float64) Polygon {
	return Polygon{Vertices: []Point{
		{X: x, Y: y}, {X: x + size, Y: y}, {X: x + size, Y: y + size}, {X: x, Y: y + size}, {X: x, Y: y},
	}}
}

func TestPolygonClosed(t *testing.T) {
	p := square(0, 0, 10)
	if !p.Closed() {
		t.Fatal("方形应闭合")
	}
	open := Polygon{Vertices: []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}}
	if open.Closed() {
		t.Fatal("未闭合多边形不应通过")
	}
}

func TestPolygonArea(t *testing.T) {
	p := square(0, 0, 10)
	if got := p.Area(); math.Abs(got-100) > 1e-9 {
		t.Fatalf("面积应为 100，实际 %v", got)
	}
}

func TestPolygonSelfIntersects(t *testing.T) {
	bowtie := Polygon{Vertices: []Point{
		{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 10, Y: 0}, {X: 0, Y: 10}, {X: 0, Y: 0},
	}}
	if !bowtie.SelfIntersects() {
		t.Fatal("蝴蝶结形应判定自交")
	}
	if square(0, 0, 10).SelfIntersects() {
		t.Fatal("方形不应判定自交")
	}
}

func TestPolygonContains(t *testing.T) {
	p := square(0, 0, 10)
	if !p.Contains(Point{X: 5, Y: 5}) {
		t.Fatal("内部点应被包含")
	}
	if p.Contains(Point{X: 15, Y: 15}) {
		t.Fatal("外部点不应被包含")
	}
}

func TestPolygonWithinBounds(t *testing.T) {
	if !square(1, 1, 8).WithinBounds(10, 10) {
		t.Fatal("界内方形应通过")
	}
	if square(5, 5, 10).WithinBounds(10, 10) {
		t.Fatal("越界方形应拒绝")
	}
}

func TestMinVertexDistance(t *testing.T) {
	a := square(0, 0, 10)
	b := square(12, 0, 10)
	if got := a.MinVertexDistance(b); math.Abs(got-2) > 1e-9 {
		t.Fatalf("最小顶点距离应为 2，实际 %v", got)
	}
}

func TestStateTransitions(t *testing.T) {
	if !CanTransitionBatch(BatchImporting, BatchSegmenting) {
		t.Fatal("importing→segmenting 应合法")
	}
	if CanTransitionBatch(BatchPublished, BatchReview) {
		t.Fatal("published→review 应非法")
	}
	if !CanTransitionRegion(RegionCandidate, RegionMismerged) {
		t.Fatal("candidate→mismerged 应合法")
	}
	if CanTransitionRegion(RegionExcluded, RegionLabeled) {
		t.Fatal("excluded→labeled 应非法")
	}
	if !CanTransitionVersion(VersionShared, VersionFrozen) {
		t.Fatal("shared→frozen 应合法")
	}
	if CanTransitionVersion(VersionFrozen, VersionDraft) {
		t.Fatal("frozen→draft 应非法")
	}
}
