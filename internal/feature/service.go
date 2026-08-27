// Package feature 计算矿物区域的颜色与消光特征，并据矿物库生成候选标签。
//
// 特征输入来自图像摘要（低倍颜色/亮度统计）与区域几何：
//   - avgR/avgG/avgB：图像整体色调叠加按区域质心位置的确定性微扰（模拟像素采样差异）；
//   - extinctionRatio：XPL 图像亮度 / PPL 图像亮度，越低表示该矿物越趋于消光；
//   - extAngle：区域主轴方向角（度），代表最大消光位对应的旋转角。
//
// 同一区域在相同输入下特征可复现，保证冻结版本可审计。
package feature

import (
	"fmt"
	"math"
	"strings"

	"task234-thinsect/internal/model"
	"task234-thinsect/internal/store"
)

// Service 提供特征计算与矿物候选判定。
type Service struct {
	batches  *store.BatchStore
	images   *store.ImageStore
	regions  *store.RegionStore
	minerals *store.MineralStore
}

// NewService 创建特征服务。
func NewService(batches *store.BatchStore, images *store.ImageStore, regions *store.RegionStore, minerals *store.MineralStore) *Service {
	return &Service{batches: batches, images: images, regions: regions, minerals: minerals}
}

// Feature 是一次特征计算的结果。
type Feature struct {
	RegionID        int64   `json:"region_id"`
	AvgR            float64 `json:"avg_r"`
	AvgG            float64 `json:"avg_g"`
	AvgB            float64 `json:"avg_b"`
	ExtinctionRatio float64 `json:"extinction_ratio"`
	ExtAngle        float64 `json:"ext_angle"`
	CandidateCode   string  `json:"candidate_code"`
	CandidateName   string  `json:"candidate_name"`
	Confidence      float64 `json:"confidence"` // 0-1，越低越需要人工复核
}

// Compute 计算区域特征并持久化；需要同批次成对的 PPL 与 XPL 图像摘要。
func (s *Service) Compute(regionID int64) (Feature, error) {
	region, err := s.regions.Get(regionID)
	if err != nil {
		return Feature{}, err
	}
	images, err := s.images.ListByBatch(region.BatchID)
	if err != nil {
		return Feature{}, err
	}
	var ppl, xpl *model.Image
	for i := range images {
		switch images[i].Mode {
		case model.ImageModePPL:
			ppl = &images[i]
		case model.ImageModeXPL:
			xpl = &images[i]
		}
	}
	if ppl == nil || xpl == nil {
		return Feature{}, model.ErrImageModeMismatch
	}
	centroid := region.Polygon.Centroid()
	// 确定性微扰：按区域质心与标签哈希，避免同一图像内区域特征完全相同。
	seed := seedOf(region.ID, centroid)
	avgR := clamp(ppl.ColorR+float64((seed%9)-4)*1.2, 0, 255)
	avgG := clamp(ppl.ColorG+float64((seed/7)%9-4)*1.2, 0, 255)
	avgB := clamp(ppl.ColorB+float64((seed/13)%9-4)*1.2, 0, 255)
	extRatio := clamp((xpl.AvgBrightness+0.5)/(ppl.AvgBrightness+0.5), 0.05, 1.0)
	extAngle := math.Mod(math.Abs(orientation(region.Polygon))*180/math.Pi, 180)

	if err := s.regions.UpdateFeatures(regionID, avgR, avgG, avgB, extRatio, extAngle); err != nil {
		return Feature{}, err
	}
	cand, conf := s.classify(avgR, avgG, avgB, extRatio)
	return Feature{
		RegionID:        regionID,
		AvgR:            avgR,
		AvgG:            avgG,
		AvgB:            avgB,
		ExtinctionRatio: extRatio,
		ExtAngle:        extAngle,
		CandidateCode:   cand.Code,
		CandidateName:   cand.Name,
		Confidence:      conf,
	}, nil
}

// classify 依据消光比与亮度匹配矿物库，返回候选与置信度。
// 无候选低于匹配阈值时返回 empty 候选（需人工复核）。
func (s *Service) classify(avgR, avgG, avgB, extRatio float64) (model.Mineral, float64) {
	brt := (avgR*0.299 + avgG*0.587 + avgB*0.114) / 255 // 归一化亮度 0-1
	all, _ := s.minerals.List()
	if len(all) == 0 {
		return model.Mineral{}, 0
	}
	var best model.Mineral
	bestDist := math.Inf(1)
	for _, m := range all {
		dExt := math.Abs(extRatio - m.ExtinctionHint)
		dBrt := math.Abs(brt - brightnessOf(m.ColorHint))
		dist := dExt*1.0 + dBrt*0.5
		if dist < bestDist {
			bestDist = dist
			best = m
		}
	}
	if math.IsInf(bestDist, 1) || bestDist > 0.45 {
		return model.Mineral{}, 0
	}
	conf := 1 - math.Min(1, bestDist/0.45)
	return best, conf
}

// brightnessOf 根据矿物颜色关键词估计期望亮度，用于候选匹配。
func brightnessOf(colorHint string) float64 {
	switch {
	case containsAny(colorHint, "黑", "深", "棕"):
		return 0.25
	case containsAny(colorHint, "绿", "褐", "黄"):
		return 0.45
	case containsAny(colorHint, "无色", "白", "灰"):
		return 0.80
	default:
		return 0.55
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// orientation 计算多边形主轴方向角（最远两顶点连线方向）。
func orientation(p model.Polygon) float64 {
	verts := p.Vertices
	if len(verts) < 2 {
		return 0
	}
	best, bx, by := 0.0, 0.0, 0.0
	for i := 0; i < len(verts); i++ {
		for j := i + 1; j < len(verts); j++ {
			dx, dy := verts[i].X-verts[j].X, verts[i].Y-verts[j].Y
			d := dx*dx + dy*dy
			if d > best {
				best = d
				bx, by = dx, dy
			}
		}
	}
	return math.Atan2(by, bx)
}

// seedOf 从区域 ID 与质心生成确定性种子。
func seedOf(id int64, c model.Point) int64 {
	return id*1000003 + int64(c.X*97) + int64(c.Y*61)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Describe 生成人类可读的特征描述（供页面与自检输出）。
func (f Feature) Describe() string {
	return fmt.Sprintf("R=%.0f G=%.0f B=%.0f 消光比=%.2f 消光角=%.1f° 候选=%s 置信=%.0f%%",
		f.AvgR, f.AvgG, f.AvgB, f.ExtinctionRatio, f.ExtAngle, f.CandidateName, f.Confidence*100)
}
