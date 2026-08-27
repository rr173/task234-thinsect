package store

import (
	"database/sql"
	"errors"
	"time"

	"task234-thinsect/internal/model"
)

// ImageStore 持久化图像摘要；同一批次内 sha256 唯一实现幂等导入。
type ImageStore struct{ db *sql.DB }

// NewImageStore 创建图像 store。
func NewImageStore(db *sql.DB) *ImageStore { return &ImageStore{db: db} }

const imageCols = `id, batch_id, name, mode, sha256, width, height, avg_brightness, color_r, color_g, color_b, created_at`

func scanImage(row interface{ Scan(...any) error }) (model.Image, error) {
	var im model.Image
	var created string
	if err := row.Scan(&im.ID, &im.BatchID, &im.Name, &im.Mode, &im.SHA256, &im.Width, &im.Height,
		&im.AvgBrightness, &im.ColorR, &im.ColorG, &im.ColorB, &created); err != nil {
		return im, err
	}
	im.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return im, nil
}

// Create 写入图像摘要；同批次同哈希冲突返回 ErrConflict。
func (s *ImageStore) Create(im model.Image) (model.Image, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`INSERT INTO images(batch_id,name,mode,sha256,width,height,avg_brightness,color_r,color_g,color_b,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		im.BatchID, im.Name, im.Mode, im.SHA256, im.Width, im.Height,
		im.AvgBrightness, im.ColorR, im.ColorG, im.ColorB, now)
	if err != nil {
		if isUniqueViolation(err) {
			return im, model.ErrConflict
		}
		return im, err
	}
	im.ID, _ = res.LastInsertId()
	im.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	return im, nil
}

// FindByHash 按批次+哈希查找既有图像（幂等导入去重）。
func (s *ImageStore) FindByHash(batchID int64, sha string) (model.Image, error) {
	row := s.db.QueryRow(`SELECT `+imageCols+` FROM images WHERE batch_id=? AND sha256=?`, batchID, sha)
	im, err := scanImage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return im, model.ErrNotFound
	}
	return im, err
}

// Get 按 ID 获取图像。
func (s *ImageStore) Get(id int64) (model.Image, error) {
	row := s.db.QueryRow(`SELECT `+imageCols+` FROM images WHERE id=?`, id)
	im, err := scanImage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return im, model.ErrNotFound
	}
	return im, err
}

// ListByBatch 列出批次下全部图像。
func (s *ImageStore) ListByBatch(batchID int64) ([]model.Image, error) {
	rows, err := s.db.Query(`SELECT `+imageCols+` FROM images WHERE batch_id=? ORDER BY id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Image
	for rows.Next() {
		im, err := scanImage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, im)
	}
	return out, rows.Err()
}

// CountByBatch 统计批次图像数。
func (s *ImageStore) CountByBatch(batchID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM images WHERE batch_id=?`, batchID).Scan(&n)
	return n, err
}
