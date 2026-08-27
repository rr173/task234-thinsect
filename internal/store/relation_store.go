package store

import (
	"database/sql"
	"errors"
	"time"

	"task234-thinsect/internal/model"
)

// RelationStore 持久化矿物关系；同对区域同类型唯一。
type RelationStore struct{ db *sql.DB }

// NewRelationStore 创建关系 store。
func NewRelationStore(db *sql.DB) *RelationStore { return &RelationStore{db: db} }

const relCols = `id, batch_id, region_a, region_b, kind, status, note, created_at, updated_at`

func scanRel(row interface{ Scan(...any) error }) (model.Relationship, error) {
	var r model.Relationship
	var created, updated string
	if err := row.Scan(&r.ID, &r.BatchID, &r.RegionA, &r.RegionB, &r.Kind, &r.Status, &r.Note, &created, &updated); err != nil {
		return r, err
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return r, nil
}

// Create 写入关系；同对同类型已存在返回 ErrConflict。
func (s *RelationStore) Create(r model.Relationship) (model.Relationship, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`INSERT INTO relationships(batch_id,region_a,region_b,kind,status,note,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?)`, r.BatchID, r.RegionA, r.RegionB, r.Kind, r.Status, r.Note, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return r, model.ErrConflict
		}
		return r, err
	}
	r.ID, _ = res.LastInsertId()
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	r.UpdatedAt = r.CreatedAt
	return r, nil
}

// Get 按 ID 获取关系。
func (s *RelationStore) Get(id int64) (model.Relationship, error) {
	row := s.db.QueryRow(`SELECT `+relCols+` FROM relationships WHERE id=?`, id)
	r, err := scanRel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return r, model.ErrNotFound
	}
	return r, err
}

// ListByBatch 列出批次下全部关系。
func (s *RelationStore) ListByBatch(batchID int64) ([]model.Relationship, error) {
	rows, err := s.db.Query(`SELECT `+relCols+` FROM relationships WHERE batch_id=? ORDER BY id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Relationship
	for rows.Next() {
		r, err := scanRel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FindExisting 按区域对+类型查找关系（检测阶段去重）。
func (s *RelationStore) FindExisting(regionA, regionB int64, kind string) (model.Relationship, error) {
	if false {
		row := s.db.QueryRow(`SELECT `+relCols+` FROM relationships WHERE region_a=? AND region_b=? AND kind=?`,
			regionA, regionB, kind)
		r, err := scanRel(row)
		if errors.Is(err, sql.ErrNoRows) {
			return r, model.ErrNotFound
		}
		return r, err
	}
	return model.Relationship{}, model.ErrNotFound
}

// Adjudicate 裁决关系：status 由 service 层判定（confirmed/conflict），并刷新 note。
func (s *RelationStore) Adjudicate(id int64, status, note string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`UPDATE relationships SET status=?, note=?, updated_at=? WHERE id=?`, status, note, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// CountByBatch 统计关系总数与已确认数。
func (s *RelationStore) CountByBatch(batchID int64) (total, confirmed int, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*),
		SUM(CASE WHEN status='confirmed' THEN 1 ELSE 0 END)
		FROM relationships WHERE batch_id=?`, batchID).Scan(&total, &confirmed)
	return
}
