// Package relation 检测矿物区域间的相邻与交生关系，并支持研究者裁决确认或标记冲突。
//
// 判定规则（全部基于区域多边形几何，与图像无关）：
//   - 相邻 adjacent：两多边形最小顶点距离 ≤ 图像短边 2%（共享边界近似）；
//   - 交生 intergrowth：区域 A 的质心落在区域 B 内部，且 A 面积 < B 面积 30%
//     （小晶体嵌生于大矿物内部）；
//   - 冲突 conflict：两区域共享邻接且矿物标签相同，但消光比差异超过 0.25
//     （暗示可能是同一矿物被误分割，或标签与特征矛盾），交由研究者裁决。
package relation

import (
	"math"

	"task234-thinsect/internal/model"
	"task234-thinsect/internal/store"
)

// Service 提供关系检测与裁决。
type Service struct {
	batches  *store.BatchStore
	images   *store.ImageStore
	regions  *store.RegionStore
	rels     *store.RelationStore
	versions *store.VersionStore
}

// NewService 创建关系服务。
func NewService(batches *store.BatchStore, images *store.ImageStore, regions *store.RegionStore, rels *store.RelationStore, versions *store.VersionStore) *Service {
	return &Service{batches: batches, images: images, regions: regions, rels: rels, versions: versions}
}

// guardFrozen 批次存在冻结版本时拒绝关系写操作。
func (s *Service) guardFrozen(batchID int64) error {
	if frozen, err := s.versions.FrozenVersionOfBatch(batchID); err == nil && frozen.ID != 0 {
		return model.ErrFrozenVersion
	} else if err != nil && err != model.ErrNotFound {
		return err
	}
	return nil
}

// DetectResult 是一次检测的产出统计。
type DetectResult struct {
	BatchID       int64   `json:"batch_id"`
	AdjacentAdded int     `json:"adjacent_added"`
	IntergrowthAdded int `json:"intergrowth_added"`
	ConflictAdded int     `json:"conflict_added"`
	AlreadyKnown  int     `json:"already_known"`
	Total         int     `json:"total"`
}

// Detect 对批次内全部区域对执行邻接/交生/冲突检测，已存在的关系跳过。
// 仅批次处于 review 阶段才允许生成关系（分割完成后）。
func (s *Service) Detect(batchID int64) (DetectResult, error) {
	batch, err := s.batches.Get(batchID)
	if err != nil {
		return DetectResult{}, err
	}
	if batch.Status != model.BatchReview {
		return DetectResult{}, model.ErrClosedLoopRequired
	}
	if err := s.guardFrozen(batchID); err != nil {
		return DetectResult{}, err
	}
	regions, err := s.regions.ListByBatch(batchID)
	if err != nil {
		return DetectResult{}, err
	}
	images, err := s.images.ListByBatch(batchID)
	if err != nil {
		return DetectResult{}, err
	}
	// 图像短边作为邻接阈值基准。
	var minDim float64 = 1000
	for _, im := range images {
		d := math.Min(im.Width, im.Height)
		if d > 0 && d < minDim {
			minDim = d
		}
	}
	threshold := minDim * 0.02

	var out DetectResult
	out.BatchID = batchID
	for i := 0; i < len(regions); i++ {
		for j := i + 1; j < len(regions); j++ {
			a, b := regions[i], regions[j]
			kind := s.classifyPair(a, b, threshold)
			if kind == "" {
				continue
			}
			if _, err := s.rels.FindExisting(a.ID, b.ID, kind); err == nil {
				out.AlreadyKnown++
				continue
			} else if err != model.ErrNotFound {
				return out, err
			}
			rel := model.Relationship{
				BatchID: batchID,
				RegionA: a.ID,
				RegionB: b.ID,
				Kind:    kind,
				Status:  kind,
				Note:    s.describe(kind, a, b),
			}
			if _, err := s.rels.Create(rel); err != nil {
				if err == model.ErrConflict {
					out.AlreadyKnown++
					continue
				}
				return out, err
			}
			switch kind {
			case model.RelationAdjacent:
				out.AdjacentAdded++
			case model.RelationIntergrowth:
				out.IntergrowthAdded++
			case model.RelationConflict:
				out.ConflictAdded++
			}
			out.Total++
		}
	}
	return out, nil
}

// classifyPair 判定区域对的关系类型；无关系返回空串。
func (s *Service) classifyPair(a, b model.Region, threshold float64) string {
	// 交生：b 的质心在 a 内且 b 面积小（或反向）。
	if isIntergrowth(a, b) {
		return model.RelationIntergrowth
	}
	if isIntergrowth(b, a) {
		return model.RelationIntergrowth
	}
	// 相邻：几何距离低于阈值。
	if a.Polygon.MinVertexDistance(b.Polygon) <= threshold {
		// 同矿物标签但消光差异大 → 冲突。
		if a.MineralCode != "" && a.MineralCode == b.MineralCode &&
			math.Abs(a.ExtinctionRatio-b.ExtinctionRatio) > 0.25 {
			return model.RelationConflict
		}
		return model.RelationAdjacent
	}
	return ""
}

// isIntergrowth 判断 inner 是否嵌生于 outer 内部。
func isIntergrowth(inner, outer model.Region) bool {
	if inner.Area <= 0 || outer.Area <= 0 {
		return false
	}
	if inner.Area >= outer.Area*0.3 {
		return false
	}
	return outer.Polygon.Contains(inner.Polygon.Centroid())
}

// describe 生成关系的人类可读说明。
func (s *Service) describe(kind string, a, b model.Region) string {
	switch kind {
	case model.RelationAdjacent:
		return "两区域几何距离低于邻接阈值，可能共享矿物边界"
	case model.RelationIntergrowth:
		return "小面积区域质心落入大区域内部，疑似晶体交生"
	case model.RelationConflict:
		return "同标签区域消光特征差异显著，需人工裁决是否误分割"
	default:
		return ""
	}
}

// AdjudicateInput 是裁决请求：confirmed=true 确认关系，false 标记冲突。
type AdjudicateInput struct {
	RelationID int64
	Confirmed  bool
	Note       string
}

// Adjudicate 裁决关系：确认 → confirmed；否认 → conflict（附注说明原因）。
func (s *Service) Adjudicate(in AdjudicateInput) (model.Relationship, error) {
	rel, err := s.rels.Get(in.RelationID)
	if err != nil {
		return rel, err
	}
	if err := s.guardFrozen(rel.BatchID); err != nil {
		return rel, err
	}
	if rel.Status == model.RelationConfirmed {
		return rel, model.ErrConflict // 已确认关系不可重复裁决
	}
	status := model.RelationConfirmed
	if !in.Confirmed {
		status = model.RelationConflict
	}
	if err := s.rels.Adjudicate(rel.ID, status, in.Note); err != nil {
		return rel, err
	}
	return s.rels.Get(rel.ID)
}

// ListByBatch 列出批次关系。
func (s *Service) ListByBatch(batchID int64) ([]model.Relationship, error) {
	return s.rels.ListByBatch(batchID)
}
