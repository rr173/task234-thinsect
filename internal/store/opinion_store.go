package store

import (
	"database/sql"
	"errors"
	"time"

	"task234-thinsect/internal/model"
)

// OpinionStore 持久化研究者补充的显微证据。
type OpinionStore struct{ db *sql.DB }

// NewOpinionStore 创建意见 store。
func NewOpinionStore(db *sql.DB) *OpinionStore { return &OpinionStore{db: db} }

const opinionCols = `id, region_id, kind, content, author, created_at`

func scanOpinion(row interface{ Scan(...any) error }) (model.Opinion, error) {
	var o model.Opinion
	var created string
	if err := row.Scan(&o.ID, &o.RegionID, &o.Kind, &o.Content, &o.Author, &created); err != nil {
		return o, err
	}
	o.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return o, nil
}

// Create 写入意见。
func (s *OpinionStore) Create(o model.Opinion) (model.Opinion, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`INSERT INTO opinions(region_id,kind,content,author,created_at)
		VALUES(?,?,?,?,?)`, o.RegionID, o.Kind, o.Content, o.Author, now)
	if err != nil {
		return o, err
	}
	o.ID, _ = res.LastInsertId()
	o.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	return o, nil
}

// ListByRegion 列出区域下全部意见（按时间正序）。
func (s *OpinionStore) ListByRegion(regionID int64) ([]model.Opinion, error) {
	rows, err := s.db.Query(`SELECT `+opinionCols+` FROM opinions WHERE region_id=? ORDER BY id`, regionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Opinion
	for rows.Next() {
		o, err := scanOpinion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// GetRegionExists 校验区域存在性（意见写入前置）。
func (s *OpinionStore) GetRegionExists(regionID int64) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM regions WHERE id=?`, regionID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
