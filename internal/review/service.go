// Package review 承载人工复核：补充显微证据（意见）与解释版本发布。
//
// 版本发布约束：
//   - 同一薄片批次的版本发布串行（FreezeVersion 使用 BEGIN IMMEDIATE 事务加批次级锁）；
//   - 冻结（frozen）是终态快照，冻结后任何区域/关系修改一律拒绝；
//   - 新冻结版本产生时，旧冻结版本自动标记 superseded（替代关系留痕）。
package review

import (
	"errors"
	"fmt"

	"task234-thinsect/internal/model"
	"task234-thinsect/internal/store"
)

// Service 提供意见与版本管理。
type Service struct {
	batches  *store.BatchStore
	regions  *store.RegionStore
	opinions *store.OpinionStore
	versions *store.VersionStore
}

// NewService 创建复核服务。
func NewService(batches *store.BatchStore, regions *store.RegionStore, opinions *store.OpinionStore, versions *store.VersionStore) *Service {
	return &Service{batches: batches, regions: regions, opinions: opinions, versions: versions}
}

// OpinionKind 是意见类型白名单。
var OpinionKind = map[string]bool{
	"extinction":   true, // 消光行为观察
	"cleavage":     true, // 解理观察
	"interference": true, // 干涉色观察
	"texture":      true, // 结构构造观察
}

// AddOpinion 为区域补充显微证据；要求批次处于 review 阶段且未冻结。
func (s *Service) AddOpinion(regionID int64, kind, content, author string) (model.Opinion, error) {
	if !OpinionKind[kind] || content == "" {
		return model.Opinion{}, model.ErrValidation
	}
	region, err := s.regions.Get(regionID)
	if err != nil {
		return model.Opinion{}, err
	}
	// 冻结守卫优先：冻结后批次为 published，先返回明确的冻结语义。
	if err := s.GuardFrozen(region.BatchID); err != nil {
		return model.Opinion{}, err
	}
	batch, err := s.batches.Get(region.BatchID)
	if err != nil {
		return model.Opinion{}, err
	}
	if batch.Status != model.BatchReview {
		return model.Opinion{}, model.ErrBadState
	}
	return s.opinions.Create(model.Opinion{
		RegionID: regionID,
		Kind:     kind,
		Content:  content,
		Author:   author,
	})
}

// ListOpinions 列出区域意见。
func (s *Service) ListOpinions(regionID int64) ([]model.Opinion, error) {
	return s.opinions.ListByRegion(regionID)
}

// CreateVersion 创建解释版本草稿；批次须处于 review 或 published 阶段。
// published 阶段允许创建下一版解释，以便冻结时替代旧冻结版本。
func (s *Service) CreateVersion(batchID int64, name, summary string) (model.InterpretationVersion, error) {
	if name == "" {
		return model.InterpretationVersion{}, model.ErrValidation
	}
	batch, err := s.batches.Get(batchID)
	if err != nil {
		return model.InterpretationVersion{}, err
	}
	if batch.Status != model.BatchReview && batch.Status != model.BatchPublished {
		return model.InterpretationVersion{}, model.ErrBadState
	}
	return s.versions.Create(model.InterpretationVersion{
		BatchID: batchID,
		Name:    name,
		Status:  model.VersionDraft,
		Summary: summary,
	})
}

// ShareVersion 草稿 → 共享。
func (s *Service) ShareVersion(id int64) (model.InterpretationVersion, error) {
	return s.transitionVersion(id, model.VersionShared)
}

// FreezeVersion 共享 → 冻结。BEGIN IMMEDIATE 事务串行化同批次发布；
// 若已存在冻结版本，先将其标记 superseded 再冻结新版本。
func (s *Service) FreezeVersion(id int64) (model.InterpretationVersion, error) {
	v, err := s.versions.Get(id)
	if err != nil {
		return v, err
	}
	if !model.CanTransitionVersion(v.Status, model.VersionFrozen) {
		return v, &model.StateTransitionError{Entity: "version", From: v.Status, To: model.VersionFrozen}
	}
	batch, err := s.batches.Get(v.BatchID)
	if err != nil {
		return v, err
	}
	if batch.Status != model.BatchReview && batch.Status != model.BatchPublished {
		return v, model.ErrBadState
	}
	if err := s.versions.WithTx(func(tx *store.VersionTx) error {
		// 同一批次旧的冻结版本 → superseded（替代关系）。
		frozen, err := tx.FrozenVersionOfBatch(v.BatchID)
		if err != nil && !errors.Is(err, model.ErrNotFound) {
			return err
		}
		if frozen.ID != 0 && frozen.ID != v.ID {
			if err := tx.MarkSupersededBy(frozen.ID, v.ID); err != nil {
				return err
			}
		}
		return tx.UpdateStatus(v.ID, model.VersionFrozen)
	}); err != nil {
		return v, err
	}
	// 批次进入已发布态。
	if batch.Status == model.BatchReview {
		if err := s.batches.UpdateStatus(batch.ID, model.BatchPublished); err != nil {
			return v, err
		}
	}
	return s.versions.Get(v.ID)
}

// SupersedeVersion 冻结 → 替代（手动废弃旧版本）。
func (s *Service) SupersedeVersion(id int64) (model.InterpretationVersion, error) {
	return s.transitionVersion(id, model.VersionSuperseded)
}

// transitionVersion 通用版本状态迁移（不涉及冻结的替代链）。
func (s *Service) transitionVersion(id int64, to string) (model.InterpretationVersion, error) {
	v, err := s.versions.Get(id)
	if err != nil {
		return v, err
	}
	if !model.CanTransitionVersion(v.Status, to) {
		return v, &model.StateTransitionError{Entity: "version", From: v.Status, To: to}
	}
	if err := s.versions.UpdateStatus(id, to); err != nil {
		return v, err
	}
	return s.versions.Get(id)
}

// ListVersions 列出批次版本。
func (s *Service) ListVersions(batchID int64) ([]model.InterpretationVersion, error) {
	return s.versions.ListByBatch(batchID)
}

// GetVersion 获取单个版本。
func (s *Service) GetVersion(id int64) (model.InterpretationVersion, error) {
	return s.versions.Get(id)
}

// GuardFrozen 检查批次是否已有冻结版本；有则拒绝区域/关系修改。
// 供 segment/relation 服务在冻结后拦截写操作。
func (s *Service) GuardFrozen(batchID int64) error {
	frozen, err := s.versions.FrozenVersionOfBatch(batchID)
	if err == nil && frozen.ID != 0 {
		return model.ErrFrozenVersion
	}
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		return err
	}
	return nil
}

// Describe 输出版本状态机的人类可读摘要。
func (v *Service) Describe(ver model.InterpretationVersion) string {
	return fmt.Sprintf("版本 %s[%s] 状态=%s 批次=%d", ver.Name, ver.Status, ver.Status, ver.BatchID)
}
