// Package segment 维护自动分割产出的矿物区域：几何校验（闭合/自交/越界）、
// 误合并拆分（保留来源）与状态流转。
package segment

import (
	"fmt"

	"task234-thinsect/internal/model"
	"task234-thinsect/internal/store"
)

// Service 提供区域管理与校验。
type Service struct {
	batches  *store.BatchStore
	images   *store.ImageStore
	regions  *store.RegionStore
	minerals *store.MineralStore
	versions *store.VersionStore
}

// NewService 创建分割服务。
func NewService(batches *store.BatchStore, images *store.ImageStore, regions *store.RegionStore, minerals *store.MineralStore, versions *store.VersionStore) *Service {
	return &Service{batches: batches, images: images, regions: regions, minerals: minerals, versions: versions}
}

// guardFrozen 批次存在冻结版本时拒绝区域写操作（不可变快照语义，任何调用路径生效）。
func (s *Service) guardFrozen(batchID int64) error {
	if frozen, err := s.versions.FrozenVersionOfBatch(batchID); err == nil && frozen.ID != 0 {
		return model.ErrFrozenVersion
	} else if err != nil && err != model.ErrNotFound {
		return err
	}
	return nil
}

// validateMineral 校验矿物编码存在于矿物库。
func (s *Service) validateMineral(code string) error {
	if _, err := s.minerals.Get(code); err != nil {
		return model.ErrUnknownMineral
	}
	return nil
}

// RegionInput 是导入区域请求。
type RegionInput struct {
	BatchID     int64
	ImageID     int64
	Label       string
	Polygon     model.Polygon
	MineralCode string // 可空：空表示尚未判定
}

// Import 导入分割区域：几何闭合校验、自交拒绝、越界拒绝、矿物存在性校验。
// 批次须处于 importing/segmenting/review 阶段。
func (s *Service) Import(in RegionInput) (model.Region, error) {
	batch, err := s.batches.Get(in.BatchID)
	if err != nil {
		return model.Region{}, err
	}
	switch batch.Status {
	case model.BatchImporting, model.BatchSegmenting, model.BatchReview:
	default:
		return model.Region{}, model.ErrBadState
	}
	img, err := s.images.Get(in.ImageID)
	if err != nil {
		return model.Region{}, err
	}
	if len(in.Polygon.Vertices) < 3 || in.Label == "" {
		return model.Region{}, model.ErrValidation
	}
	if err := in.Polygon.Validate(); err != nil {
		return model.Region{}, err
	}
	if !in.Polygon.WithinBounds(img.Width, img.Height) {
		return model.Region{}, model.ErrRegionOutOfBounds
	}
	status := model.RegionCandidate
	if in.MineralCode != "" {
		if err := s.validateMineral(in.MineralCode); err != nil {
			return model.Region{}, err
		}
		status = model.RegionLabeled
	}
	r := model.Region{
		BatchID:     in.BatchID,
		ImageID:     in.ImageID,
		Label:       in.Label,
		MineralCode: in.MineralCode,
		Status:      status,
		Polygon:     in.Polygon,
	}
	return s.regions.Create(r)
}

// MarkMismerged 标记自动分割把两种矿物合并为一个区域（误合并）。
func (s *Service) MarkMismerged(id int64) (model.Region, error) {
	r, err := s.regions.Get(id)
	if err != nil {
		return r, err
	}
	if err := s.guardFrozen(r.BatchID); err != nil {
		return r, err
	}
	if !model.CanTransitionRegion(r.Status, model.RegionMismerged) {
		return r, &model.StateTransitionError{Entity: "region", From: r.Status, To: model.RegionMismerged}
	}
	if err := s.regions.UpdateStatus(id, model.RegionMismerged); err != nil {
		return r, err
	}
	return s.regions.Get(id)
}

// MarkOpenBoundary 标记闭合环存在缺口（缺边界）。
func (s *Service) MarkOpenBoundary(id int64) (model.Region, error) {
	r, err := s.regions.Get(id)
	if err != nil {
		return r, err
	}
	if err := s.guardFrozen(r.BatchID); err != nil {
		return r, err
	}
	if !model.CanTransitionRegion(r.Status, model.RegionOpenBoundary) {
		return r, &model.StateTransitionError{Entity: "region", From: r.Status, To: model.RegionOpenBoundary}
	}
	if err := s.regions.UpdateStatus(id, model.RegionOpenBoundary); err != nil {
		return r, err
	}
	return s.regions.Get(id)
}

// Exclude 排除非矿物区域（气泡/裂缝/玻屑），审计保留记录。
func (s *Service) Exclude(id int64) (model.Region, error) {
	r, err := s.regions.Get(id)
	if err != nil {
		return r, err
	}
	if err := s.guardFrozen(r.BatchID); err != nil {
		return r, err
	}
	if !model.CanTransitionRegion(r.Status, model.RegionExcluded) {
		return r, &model.StateTransitionError{Entity: "region", From: r.Status, To: model.RegionExcluded}
	}
	if err := s.regions.UpdateStatus(id, model.RegionExcluded); err != nil {
		return r, err
	}
	return s.regions.Get(id)
}

// Label 人工标注区域矿物：未知矿物拒绝；候选/缺边界/误合并可转为已标注。
func (s *Service) Label(id int64, mineralCode string) (model.Region, error) {
	if err := s.validateMineral(mineralCode); err != nil {
		return model.Region{}, err
	}
	r, err := s.regions.Get(id)
	if err != nil {
		return r, err
	}
	if err := s.guardFrozen(r.BatchID); err != nil {
		return r, err
	}
	if !model.CanTransitionRegion(r.Status, model.RegionLabeled) {
		return r, &model.StateTransitionError{Entity: "region", From: r.Status, To: model.RegionLabeled}
	}
	if err := s.regions.UpdateLabel(id, mineralCode, model.RegionLabeled); err != nil {
		return r, err
	}
	return s.regions.Get(id)
}

// SplitInput 是误合并拆分请求：原区域保留为误合并证据，拆出子区域均保留来源。
type SplitInput struct {
	RegionID int64
	Parts    []model.Polygon
}

// Split 拆分误合并区域：原区域标 mismerged（保留），每个子区域 parent 指向原区域。
// 任一子区域几何非法或越界则整体拒绝，不产生部分写入。
func (s *Service) Split(in SplitInput) ([]model.Region, error) {
	parent, err := s.regions.Get(in.RegionID)
	if err != nil {
		return nil, err
	}
	if err := s.guardFrozen(parent.BatchID); err != nil {
		return nil, err
	}
	if parent.Status != model.RegionMismerged {
		return nil, &model.StateTransitionError{Entity: "region", From: parent.Status, To: model.RegionMismerged}
	}
	if len(in.Parts) < 2 {
		return nil, model.ErrValidation
	}
	img, err := s.images.Get(parent.ImageID)
	if err != nil {
		return nil, err
	}
	// 预校验：全部子区域必须几何合法且在图像内，任一失败即整体拒绝。
	for i, p := range in.Parts {
		if len(p.Vertices) < 3 {
			return nil, model.ErrInvalidGeometry
		}
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("part %d: %w", i, err)
		}
		if !p.WithinBounds(img.Width, img.Height) {
			return nil, fmt.Errorf("part %d: %w", i, model.ErrRegionOutOfBounds)
		}
	}
	// 原区域标记 mismerged 与全部子区域写入同一事务，失败时整体回滚。
	var out []model.Region
	if err := s.regions.WithTx(func(tx *store.RegionTx) error {
		if err := tx.UpdateStatus(in.RegionID, model.RegionMismerged); err != nil {
			return err
		}
		for i, p := range in.Parts {
			r := model.Region{
				BatchID:        parent.BatchID,
				ImageID:        parent.ImageID,
				Label:          fmt.Sprintf("%s-%d", parent.Label, i+1),
				MineralCode:    "",
				Status:         model.RegionCandidate,
				ParentRegionID: &parent.ID,
				Polygon:        p,
			}
			created, err := tx.Create(r)
			if err != nil {
				return err
			}
			out = append(out, created)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// Get 获取区域详情。
func (s *Service) Get(id int64) (model.Region, error) {
	return s.regions.Get(id)
}

// ListByImage 列出图像下区域。
func (s *Service) ListByImage(imageID int64) ([]model.Region, error) {
	return s.regions.ListByImage(imageID)
}

// ListByBatch 列出批次下区域。
func (s *Service) ListByBatch(batchID int64) ([]model.Region, error) {
	return s.regions.ListByBatch(batchID)
}
