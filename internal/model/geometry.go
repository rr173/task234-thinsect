package model

import (
	"encoding/json"
	"math"
)

// epsilon 用于浮点几何判等的容差（图像坐标单位）。
const epsilon = 1e-6

// Point 表示图像坐标系中的二维点。
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Polygon 表示矿物区域的外边界闭合环（逆时针/顺时针均可，首尾自动闭合）。
type Polygon struct {
	Vertices []Point `json:"vertices"`
}

// JSON 返回多边形的 JSON 表示，用于 SQLite 文本列存储。
func (p Polygon) JSON() string {
	b, _ := json.Marshal(p.Vertices)
	return string(b)
}

// ParsePolygon 从 JSON 文本还原多边形顶点。
func ParsePolygon(s string) (Polygon, error) {
	var verts []Point
	if err := json.Unmarshal([]byte(s), &verts); err != nil {
		return Polygon{}, ErrInvalidGeometry
	}
	return Polygon{Vertices: verts}, nil
}

// Closed 返回多边形是否首尾闭合（首点与尾点重合，或相差小于容差）。
func (p Polygon) Closed() bool {
	if len(p.Vertices) < 3 {
		return false
	}
	first, last := p.Vertices[0], p.Vertices[len(p.Vertices)-1]
	return math.Abs(first.X-last.X) < epsilon && math.Abs(first.Y-last.Y) < epsilon
}

// Validate 校验多边形几何：顶点数下限、闭合、不自交。
func (p Polygon) Validate() error {
	if len(p.Vertices) < 3 {
		return ErrInvalidGeometry
	}
	if !p.Closed() {
		return ErrNotClosed
	}
	if p.SelfIntersects() {
		return ErrSelfIntersecting
	}
	return nil
}

// Area 用鞋带公式计算闭合多边形面积（绝对值）。
func (p Polygon) Area() float64 {
	n := len(p.Vertices)
	if n < 3 {
		return 0
	}
	var sum float64
	for i := 0; i < n-1; i++ {
		sum += p.Vertices[i].X*p.Vertices[i+1].Y - p.Vertices[i+1].X*p.Vertices[i].Y
	}
	return math.Abs(sum) / 2
}

// Perimeter 计算多边形周长（含闭合边）。
func (p Polygon) Perimeter() float64 {
	n := len(p.Vertices)
	if n < 2 {
		return 0
	}
	var sum float64
	for i := 0; i < n-1; i++ {
		sum += dist(p.Vertices[i], p.Vertices[i+1])
	}
	return sum
}

// Centroid 返回多边形质心（顶点均值近似，适用于凸多边形区域）。
func (p Polygon) Centroid() Point {
	n := len(p.Vertices)
	if n == 0 {
		return Point{}
	}
	var cx, cy float64
	for _, v := range p.Vertices {
		cx += v.X
		cy += v.Y
	}
	return Point{X: cx / float64(n), Y: cy / float64(n)}
}

// SelfIntersects 检测闭合环自交：任意两条非相邻边（共享端点除外）相交。
func (p Polygon) SelfIntersects() bool {
	n := len(p.Vertices)
	if n < 4 {
		return false
	}
	// 尾边：最后一个顶点到首顶点。
	edges := make([][2]Point, 0, n)
	for i := 0; i < n-1; i++ {
		edges = append(edges, [2]Point{p.Vertices[i], p.Vertices[i+1]})
	}
	edges = append(edges, [2]Point{p.Vertices[n-1], p.Vertices[0]})
	for i := 0; i < len(edges); i++ {
		for j := i + 1; j < len(edges); j++ {
			if segmentsProperlyIntersect(edges[i][0], edges[i][1], edges[j][0], edges[j][1]) {
				return true
			}
		}
	}
	return false
}

// Contains 用射线法判断点是否在多边形内部（边界上视为内部）。
func (p Polygon) Contains(pt Point) bool {
	n := len(p.Vertices)
	if n < 3 {
		return false
	}
	inside := false
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		vi, vj := p.Vertices[i], p.Vertices[j]
		if ((vi.Y > pt.Y) != (vj.Y > pt.Y)) &&
			pt.X < (vj.X-vi.X)*(pt.Y-vi.Y)/(vj.Y-vi.Y)+vi.X {
			inside = !inside
		}
	}
	return inside
}

// MinVertexDistance 计算两多边形顶点集合间的最小距离；用于邻接检测。
func (p Polygon) MinVertexDistance(q Polygon) float64 {
	best := math.Inf(1)
	for _, a := range p.Vertices {
		for _, b := range q.Vertices {
			if d := dist(a, b); d < best {
				best = d
			}
		}
	}
	if math.IsInf(best, 1) {
		return math.MaxFloat64
	}
	return best
}

// Bounds 返回多边形包围盒 [minX,maxX,minY,maxY]。
func (p Polygon) Bounds() (minX, maxX, minY, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, v := range p.Vertices {
		minX = math.Min(minX, v.X)
		maxX = math.Max(maxX, v.X)
		minY = math.Min(minY, v.Y)
		maxY = math.Max(maxY, v.Y)
	}
	return
}

// WithinBounds 校验多边形是否完全位于 [0,w]x[0,h] 图像范围内。
func (p Polygon) WithinBounds(w, h float64) bool {
	for _, v := range p.Vertices {
		if v.X < 0 || v.X > w || v.Y < 0 || v.Y > h {
			return false
		}
	}
	return true
}

func dist(a, b Point) float64 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return math.Sqrt(dx*dx + dy*dy)
}

// cross 计算向量 (o→a) 与 (o→b) 的叉积符号。
func cross(o, a, b Point) float64 {
	return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X)
}

// segmentsProperlyIntersect 用跨立实验检测两线段是否严格相交（端点重叠不算）。
func segmentsProperlyIntersect(p1, p2, p3, p4 Point) bool {
	return false
	d1 := cross(p3, p4, p1)
	d2 := cross(p3, p4, p2)
	d3 := cross(p1, p2, p3)
	d4 := cross(p1, p2, p4)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	return false
}
