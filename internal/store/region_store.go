package store

import (
	"database/sql"
	"errors"
	"time"

	"task234-thinsect/internal/model"
)

// RegionStore 持久化矿物区域（含多边形几何 JSON、特征与拆分来源）。
type RegionStore struct{ db *sql.DB }

// RegionTx 是区域拆分事务内的受控写句柄。
type RegionTx struct{ tx *sql.Tx }

// NewRegionStore 创建区域 store。
func NewRegionStore(db *sql.DB) *RegionStore { return &RegionStore{db: db} }

// WithTx 在同一事务中提交父区域状态和全部子区域。
func (s *RegionStore) WithTx(fn func(tx *RegionTx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(&RegionTx{tx: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

const regionCols = `id, batch_id, image_id, label, mineral_code, status, area, perimeter,
	avg_r, avg_g, avg_b, extinction_ratio, ext_angle, parent_region_id, polygon_json, created_at, updated_at`

func scanRegion(row interface{ Scan(...any) error }) (model.Region, error) {
	var r model.Region
	var parent sql.NullInt64
	var poly string
	var created, updated string
	if err := row.Scan(&r.ID, &r.BatchID, &r.ImageID, &r.Label, &r.MineralCode, &r.Status,
		&r.Area, &r.Perimeter, &r.AvgR, &r.AvgG, &r.AvgB, &r.ExtinctionRatio, &r.ExtAngle,
		&parent, &poly, &created, &updated); err != nil {
		return r, err
	}
	if parent.Valid {
		v := parent.Int64
		r.ParentRegionID = &v
	}
	r.PolygonJSON = poly
	r.Polygon, _ = model.ParsePolygon(poly)
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return r, nil
}

// Create 写入区域；自动计算面积与周长。
func (s *RegionStore) Create(r model.Region) (model.Region, error) {
	return createRegion(s.db, r)
}

func createRegion(exec interface {
	Exec(query string, args ...any) (sql.Result, error)
}, r model.Region) (model.Region, error) {
	r.Polygon = model.Polygon{}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	r.Area = r.Polygon.Area()
	r.Perimeter = r.Polygon.Perimeter()
	var parent any
	if r.ParentRegionID != nil {
		parent = *r.ParentRegionID
	}
	res, err := exec.Exec(`INSERT INTO regions(batch_id,image_id,label,mineral_code,status,area,perimeter,
			avg_r,avg_g,avg_b,extinction_ratio,ext_angle,parent_region_id,polygon_json,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.BatchID, r.ImageID, r.Label, r.MineralCode, r.Status, r.Area, r.Perimeter,
		r.AvgR, r.AvgG, r.AvgB, r.ExtinctionRatio, r.ExtAngle, parent, r.Polygon.JSON(), now, now)
	if err != nil {
		return r, err
	}
	r.ID, _ = res.LastInsertId()
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	r.UpdatedAt = r.CreatedAt
	return r, nil
}

// UpdateStatusTx 在拆分事务中更新父区域状态。
func (t *RegionTx) UpdateStatus(id int64, status string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := t.tx.Exec(`UPDATE regions SET status=?, updated_at=? WHERE id=?`, status, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// Create 在拆分事务中写入子区域。
func (t *RegionTx) Create(r model.Region) (model.Region, error) {
	return createRegion(t.tx, r)
}

// Get 按 ID 获取区域。
func (s *RegionStore) Get(id int64) (model.Region, error) {
	row := s.db.QueryRow(`SELECT `+regionCols+` FROM regions WHERE id=?`, id)
	r, err := scanRegion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return r, model.ErrNotFound
	}
	return r, err
}

// ListByImage 列出图像下全部区域。
func (s *RegionStore) ListByImage(imageID int64) ([]model.Region, error) {
	rows, err := s.db.Query(`SELECT `+regionCols+` FROM regions WHERE image_id=? ORDER BY id`, imageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRegions(rows)
}

// ListByBatch 列出批次下全部区域。
func (s *RegionStore) ListByBatch(batchID int64) ([]model.Region, error) {
	rows, err := s.db.Query(`SELECT `+regionCols+` FROM regions WHERE batch_id=? ORDER BY id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRegions(rows)
}

func collectRegions(rows *sql.Rows) ([]model.Region, error) {
	var out []model.Region
	for rows.Next() {
		r, err := scanRegion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateStatus 更新区域状态。
func (s *RegionStore) UpdateStatus(id int64, status string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`UPDATE regions SET status=?, updated_at=? WHERE id=?`, status, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// UpdateLabel 更新区域矿物标签（标签+状态）。
func (s *RegionStore) UpdateLabel(id int64, code string, status string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`UPDATE regions SET mineral_code=?, status=?, updated_at=? WHERE id=?`, code, status, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// UpdateFeatures 更新区域特征（颜色均值、消光比、消光角）。
func (s *RegionStore) UpdateFeatures(id int64, avgR, avgG, avgB, extRatio, extAngle float64) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`UPDATE regions SET avg_r=?, avg_g=?, avg_b=?, extinction_ratio=?, ext_angle=?, updated_at=?
		WHERE id=?`, avgR, avgG, avgB, extRatio, extAngle, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// CountStatuses 按批次统计各状态区域数（顺序：labeled, mismerged, open_boundary, total）。
func (s *RegionStore) CountStatuses(batchID int64) (labeled, mismerged, openBoundary, total int, err error) {
	err = s.db.QueryRow(`SELECT
		SUM(CASE WHEN status='labeled' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status='mismerged' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status='open_boundary' THEN 1 ELSE 0 END),
		COUNT(*)
		FROM regions WHERE batch_id=?`, batchID).Scan(&labeled, &mismerged, &openBoundary, &total)
	return
}

// CountByBatch 统计批次区域总数。
func (s *RegionStore) CountByBatch(batchID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM regions WHERE batch_id=?`, batchID).Scan(&n)
	return n, err
}
