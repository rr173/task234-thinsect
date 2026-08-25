// Package image 管理薄片图像的摘要导入：同批次同哈希幂等，保证重启重导不产生重复。
package image

import (
	"task234-thinsect/internal/model"
	"task234-thinsect/internal/store"
)

// Service 提供图像摘要导入与查询。
type Service struct {
	batches *store.BatchStore
	images  *store.ImageStore
}

// NewService 创建图像服务。
func NewService(batches *store.BatchStore, images *store.ImageStore) *Service {
	return &Service{batches: batches, images: images}
}

// ImportInput 是图像摘要导入请求。
type ImportInput struct {
	BatchID       int64
	Name          string
	Mode          string
	SHA256        string
	Width         float64
	Height        float64
	AvgBrightness float64
	ColorR        float64
	ColorG        float64
	ColorB        float64
}

// Import 导入图像摘要；哈希重复时返回既有图像（幂等），不报错。
// 批次必须处于 importing 或 segmenting 阶段，模式必须是 PPL/XPL。
func (s *Service) Import(in ImportInput) (model.Image, bool, error) {
	if in.Mode != model.ImageModePPL && in.Mode != model.ImageModeXPL {
		return model.Image{}, false, model.ErrValidation
	}
	if in.Width <= 0 || in.Height <= 0 || in.Name == "" || in.SHA256 == "" {
		return model.Image{}, false, model.ErrValidation
	}
	if in.AvgBrightness < 0 || in.AvgBrightness > 255 {
		return model.Image{}, false, model.ErrValidation
	}
	batch, err := s.batches.Get(in.BatchID)
	if err != nil {
		return model.Image{}, false, err
	}
	if batch.Status != model.BatchImporting && batch.Status != model.BatchSegmenting {
		return model.Image{}, false, model.ErrBadState
	}
	_, err = s.images.FindByHash(in.BatchID, in.SHA256)
	if err == nil {
		return model.Image{}, false, model.ErrConflict
	}
	if err != model.ErrNotFound {
		return model.Image{}, false, err
	}
	img := model.Image{
		BatchID:       in.BatchID,
		Name:          in.Name,
		Mode:          in.Mode,
		SHA256:        in.SHA256,
		Width:         in.Width,
		Height:        in.Height,
		AvgBrightness: in.AvgBrightness,
		ColorR:        in.ColorR,
		ColorG:        in.ColorG,
		ColorB:        in.ColorB,
	}
	created, err := s.images.Create(img)
	return created, false, err
}

// ListByBatch 列出批次下全部图像摘要。
func (s *Service) ListByBatch(batchID int64) ([]model.Image, error) {
	return s.images.ListByBatch(batchID)
}

// Get 获取单张图像摘要。
func (s *Service) Get(id int64) (model.Image, error) {
	return s.images.Get(id)
}
