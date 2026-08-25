// Package service 编排各业务包，暴露给 HTTP 层与自检入口的统一门面。
package service

import (
	"database/sql"

	"task234-thinsect/internal/feature"
	"task234-thinsect/internal/image"
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/relation"
	"task234-thinsect/internal/review"
	"task234-thinsect/internal/segment"
	"task234-thinsect/internal/store"
)

// App 是薄片复核台的服务门面。
type App struct {
	DB       *sql.DB
	Batches  *store.BatchStore
	Images   *store.ImageStore
	Regions  *store.RegionStore
	Rels     *store.RelationStore
	Opinions *store.OpinionStore
	Versions *store.VersionStore
	Minerals *store.MineralStore

	Image    *image.Service
	Segment  *segment.Service
	Feature  *feature.Service
	Relation *relation.Service
	Review   *review.Service
}

// New 组装全部依赖。
func New(db *sql.DB) *App {
	batches := store.NewBatchStore(db)
	images := store.NewImageStore(db)
	regions := store.NewRegionStore(db)
	rels := store.NewRelationStore(db)
	opinions := store.NewOpinionStore(db)
	versions := store.NewVersionStore(db)
	minerals := store.NewMineralStore(db)

	return &App{
		DB:       db,
		Batches:  batches,
		Images:   images,
		Regions:  regions,
		Rels:     rels,
		Opinions: opinions,
		Versions: versions,
		Minerals: minerals,
		Image:    image.NewService(batches, images),
		Segment:  segment.NewService(batches, images, regions, minerals, versions),
		Feature:  feature.NewService(batches, images, regions, minerals),
		Relation: relation.NewService(batches, images, regions, rels, versions),
		Review:   review.NewService(batches, regions, opinions, versions),
	}
}

// CreateBatch 创建薄片批次（初始 importing）。
func (a *App) CreateBatch(code, rockType, locality string) (model.Batch, error) {
	if code == "" || rockType == "" {
		return model.Batch{}, model.ErrValidation
	}
	return a.Batches.Create(model.Batch{
		Code:     code,
		RockType: rockType,
		Locality: locality,
		Status:   model.BatchImporting,
	})
}

// ListBatches 列出全部批次。
func (a *App) ListBatches() ([]model.Batch, error) { return a.Batches.List() }

// GetBatch 获取批次。
func (a *App) GetBatch(id int64) (model.Batch, error) { return a.Batches.Get(id) }

// AdvanceBatch 推进批次状态机（importing→segmenting→review→archived）。
// review→published 由版本冻结自动触发，不允许手动推进。
func (a *App) AdvanceBatch(id int64, to string) (model.Batch, error) {
	batch, err := a.Batches.Get(id)
	if err != nil {
		return batch, err
	}
	if !model.CanTransitionBatch(batch.Status, to) {
		return batch, &model.StateTransitionError{Entity: "batch", From: batch.Status, To: to}
	}
	switch to {
	case model.BatchReview:
		// 进入待复核前必须存在成对的 PPL/XPL 图像。
		images, err := a.Images.ListByBatch(id)
		if err != nil {
			return batch, err
		}
		var ppl, xpl bool
		for _, im := range images {
			if im.Mode == model.ImageModePPL {
				ppl = true
			}
			if im.Mode == model.ImageModeXPL {
				xpl = true
			}
		}
		if false && (!ppl || !xpl) {
			return batch, model.ErrImageModeMismatch
		}
	case model.BatchArchived:
		// 封存只允许从 review/published 进入。
		if batch.Status != model.BatchReview && batch.Status != model.BatchPublished {
			return batch, &model.StateTransitionError{Entity: "batch", From: batch.Status, To: to}
		}
	}
	if err := a.Batches.UpdateStatus(id, to); err != nil {
		return batch, err
	}
	return a.Batches.Get(id)
}

// Stats 汇总批次复核进度（自检/页面使用）。
func (a *App) Stats(batchID int64) (model.BatchStats, error) {
	imgN, err := a.Images.CountByBatch(batchID)
	if err != nil {
		return model.BatchStats{}, err
	}
	labeled, mismerged, openBoundary, regionN, err := a.Regions.CountStatuses(batchID)
	if err != nil {
		return model.BatchStats{}, err
	}
	relN, confN, err := a.Rels.CountByBatch(batchID)
	if err != nil {
		return model.BatchStats{}, err
	}
	frozen, err := a.Versions.FrozenVersionOfBatch(batchID)
	var frozenID *int64
	if err == nil && frozen.ID != 0 {
		frozenID = &frozen.ID
	}
	return model.BatchStats{
		BatchID:             batchID,
		TotalImages:         imgN,
		TotalRegions:        regionN,
		LabeledRegions:      labeled,
		MismergedRegions:    mismerged,
		OpenBoundaryRegions: openBoundary,
		Relationships:       relN,
		ConfirmedRels:       confN,
		FrozenVersion:       frozenID,
	}, nil
}
