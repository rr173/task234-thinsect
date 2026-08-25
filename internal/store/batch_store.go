package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"task234-thinsect/internal/model"
)

// BatchStore 持久化薄片批次。
type BatchStore struct{ db *sql.DB }

// NewBatchStore 创建批次 store。
func NewBatchStore(db *sql.DB) *BatchStore { return &BatchStore{db: db} }

const batchCols = `id, code, rock_type, locality, status, created_at, updated_at`

func scanBatch(row interface{ Scan(...any) error }) (model.Batch, error) {
	var b model.Batch
	var created, updated string
	if err := row.Scan(&b.ID, &b.Code, &b.RockType, &b.Locality, &b.Status, &created, &updated); err != nil {
		return b, err
	}
	b.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return b, nil
}

// Create 创建批次，code 冲突时返回 ErrConflict。
func (s *BatchStore) Create(b model.Batch) (model.Batch, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`INSERT INTO batches(code,rock_type,locality,status,created_at,updated_at)
		VALUES(?,?,?,?,?,?)`, b.Code, b.RockType, b.Locality, b.Status, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return b, model.ErrConflict
		}
		return b, err
	}
	b.ID, _ = res.LastInsertId()
	b.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	b.UpdatedAt = b.CreatedAt
	return b, nil
}

// Get 按 ID 获取批次。
func (s *BatchStore) Get(id int64) (model.Batch, error) {
	row := s.db.QueryRow(`SELECT `+batchCols+` FROM batches WHERE id=?`, id)
	b, err := scanBatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return b, model.ErrNotFound
	}
	return b, err
}

// GetByCode 按薄片编号获取批次。
func (s *BatchStore) GetByCode(code string) (model.Batch, error) {
	row := s.db.QueryRow(`SELECT `+batchCols+` FROM batches WHERE code=?`, code)
	b, err := scanBatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return b, model.ErrNotFound
	}
	return b, err
}

// List 列出全部批次（按创建时间倒序）。
func (s *BatchStore) List() ([]model.Batch, error) {
	rows, err := s.db.Query(`SELECT ` + batchCols + ` FROM batches ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Batch
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// UpdateStatus 更新批次状态并刷新 updated_at。
func (s *BatchStore) UpdateStatus(id int64, status string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`UPDATE batches SET status=?, updated_at=? WHERE id=?`, status, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// Delete 删除批次（仅允许未产生数据或 importing 阶段由 service 层把关）。
func (s *BatchStore) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM batches WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// isUniqueViolation 判断 SQLite 唯一约束冲突。
func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "constraint failed"))
}
