package model

import "time"

// Batch 是一组火山岩薄片图像的复核批次。
type Batch struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`      // 薄片编号，如 TS-2026-001
	RockType  string    `json:"rock_type"` // 岩石类型，如 basalt / andesite / rhyolite
	Locality  string    `json:"locality"`  // 采样产地
	Status    string    `json:"status"`    // 批次状态机
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Image 是薄片下某偏光模式的图像摘要；sha256 用于幂等导入。
// 摘要字段（亮度/颜色统计）模拟从偏光图像自动提取的低倍统计量，
// 是特征模块计算区域颜色与消光特征的输入。
type Image struct {
	ID            int64   `json:"id"`
	BatchID       int64   `json:"batch_id"`
	Name          string  `json:"name"`
	Mode          string  `json:"mode"` // PPL / XPL
	SHA256        string  `json:"sha256"`
	Width         float64 `json:"width"`
	Height        float64 `json:"height"`
	AvgBrightness float64 `json:"avg_brightness"` // 图像整体亮度 0-255
	ColorR        float64 `json:"color_r"`        // 图像整体色调统计（0-255）
	ColorG        float64 `json:"color_g"`
	ColorB        float64 `json:"color_b"`
	CreatedAt     time.Time `json:"created_at"`
}

// Region 是图像中的矿物区域（自动分割或人工拆分产物）。
type Region struct {
	ID              int64     `json:"id"`
	BatchID         int64     `json:"batch_id"`
	ImageID         int64     `json:"image_id"`
	Label           string    `json:"label"` // 区域别名，如 R1
	MineralCode     string    `json:"mineral_code"`
	MineralName     string    `json:"mineral_name,omitempty"`
	Status          string    `json:"status"` // 区域状态机
	Area            float64   `json:"area"`
	Perimeter       float64   `json:"perimeter"`
	AvgR            float64   `json:"avg_r"`
	AvgG            float64   `json:"avg_g"`
	AvgB            float64   `json:"avg_b"`
	ExtinctionRatio float64   `json:"extinction_ratio"` // XPL 亮度 / PPL 亮度，越低越消光
	ExtAngle        float64   `json:"ext_angle"`        // 最大消光角（度）
	ParentRegionID  *int64    `json:"parent_region_id,omitempty"` // 误合并拆分的来源区域
	PolygonJSON     string    `json:"-"`
	Polygon         Polygon   `json:"polygon"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Relationship 描述两个区域间的矿物关系（相邻/交生/冲突），人工裁决后确认。
type Relationship struct {
	ID        int64     `json:"id"`
	BatchID   int64     `json:"batch_id"`
	RegionA   int64     `json:"region_a"`
	RegionB   int64     `json:"region_b"`
	Kind      string    `json:"kind"`   // adjacent / intergrowth / conflict
	Status    string    `json:"status"` // 与 Kind 同值；确认后为 confirmed
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Opinion 是研究者补充的显微证据（如消光行为、解理、干涉色描述）。
type Opinion struct {
	ID        int64     `json:"id"`
	RegionID  int64     `json:"region_id"`
	Kind      string    `json:"kind"` // extinction / cleavage / interference / texture
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// InterpretationVersion 是薄片批次的解释版本，冻结后不可变。
type InterpretationVersion struct {
	ID            int64     `json:"id"`
	BatchID       int64     `json:"batch_id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"` // draft / shared / frozen / superseded
	Summary       string    `json:"summary"`
	CreatedAt     time.Time `json:"created_at"`
	FrozenAt      *time.Time `json:"frozen_at,omitempty"`
	SupersededAt  *time.Time `json:"superseded_at,omitempty"`
	SupersededBy  *int64    `json:"superseded_by,omitempty"`
}

// Mineral 是矿物库条目，用于特征→候选标签匹配。
type Mineral struct {
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	ColorHint      string  `json:"color_hint"`      // 典型颜色（低倍 PPL）
	ExtinctionHint float64 `json:"extinction_hint"` // 典型消光比阈值
	Description    string  `json:"description"`
}

// BatchStats 汇总批次复核进度，供自检与页面使用。
type BatchStats struct {
	BatchID        int64  `json:"batch_id"`
	TotalImages    int    `json:"total_images"`
	TotalRegions   int    `json:"total_regions"`
	LabeledRegions int    `json:"labeled_regions"`
	MismergedRegions int `json:"mismerged_regions"`
	OpenBoundaryRegions int `json:"open_boundary_regions"`
	Relationships  int    `json:"relationships"`
	ConfirmedRels  int    `json:"confirmed_rels"`
	FrozenVersion  *int64 `json:"frozen_version,omitempty"`
}
