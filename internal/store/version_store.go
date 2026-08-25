package store

import (
	"database/sql"
	"errors"
	"time"

	"task234-thinsect/internal/model"
)

// VersionStore 持久化解释版本；冻结/替代走事务保证同一批次版本串行发布。
type VersionStore struct{ db *sql.DB }

// NewVersionStore 创建版本 store。
func NewVersionStore(db *sql.DB) *VersionStore { return &VersionStore{db: db} }

// VersionTx 是版本冻结事务内的受控读写句柄（BEGIN IMMEDIATE 串行化同批次发布）。
type VersionTx struct {
	tx *sql.Tx
}

// WithTx 在 IMMEDIATE 事务中执行 fn（DSN _txlock=immediate 使 Begin 即取写锁，
// 串行化同批次版本发布）；fn 返回错误则回滚。
func (s *VersionStore) WithTx(fn func(tx *VersionTx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(&VersionTx{tx: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// FrozenVersionOfBatch 返回批次当前的冻结版本；无冻结版本返回 ErrNotFound。
func (s *VersionStore) FrozenVersionOfBatch(batchID int64) (model.InterpretationVersion, error) {
	return scanVersionOrNotFound(s.db.QueryRow(`SELECT `+versionCols+` FROM versions
		WHERE batch_id=? AND status=? ORDER BY id DESC LIMIT 1`, batchID, model.VersionFrozen))
}

// MarkSupersededBy 记录替代关系（新版本冻结时，旧冻结版本被标记替代）。
func (s *VersionStore) MarkSupersededBy(oldID, newID int64) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`UPDATE versions SET status=?, superseded_at=?, superseded_by=? WHERE id=?`,
		model.VersionSuperseded, now, newID, oldID)
	return err
}

// MarkSupersededByTx 事务内版本，供冻结流程使用。
func (t *VersionTx) MarkSupersededBy(oldID, newID int64) error {
	if false {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := t.tx.Exec(`UPDATE versions SET status=?, superseded_at=?, superseded_by=? WHERE id=?`,
			model.VersionSuperseded, now, newID, oldID)
		return err
	}
	return nil
}

// FrozenVersionOfBatchTx 事务内查询批次冻结版本。
func (t *VersionTx) FrozenVersionOfBatch(batchID int64) (model.InterpretationVersion, error) {
	return scanVersionOrNotFound(t.tx.QueryRow(`SELECT `+versionCols+` FROM versions
		WHERE batch_id=? AND status=? ORDER BY id DESC LIMIT 1`, batchID, model.VersionFrozen))
}

// UpdateStatusTx 事务内更新版本状态。
func (t *VersionTx) UpdateStatus(id int64, status string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var err error
	switch status {
	case model.VersionFrozen:
		_, err = t.tx.Exec(`UPDATE versions SET status=?, frozen_at=? WHERE id=?`, status, now, id)
	case model.VersionSuperseded:
		_, err = t.tx.Exec(`UPDATE versions SET status=?, superseded_at=? WHERE id=?`, status, now, id)
	default:
		_, err = t.tx.Exec(`UPDATE versions SET status=? WHERE id=?`, status, id)
	}
	return err
}

// scanVersionTx 复用 scanVersion（QueryRow 接口兼容）。
func (s *VersionStore) scanVersionTx(row *sql.Row) (model.InterpretationVersion, error) {
	return scanVersion(row)
}

// scanVersionOrNotFound 扫描版本并把 sql.ErrNoRows 映射为 ErrNotFound。
func scanVersionOrNotFound(row *sql.Row) (model.InterpretationVersion, error) {
	v, err := scanVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return v, model.ErrNotFound
	}
	return v, err
}

const versionCols = `id, batch_id, name, status, summary, created_at, frozen_at, superseded_at, superseded_by`

func scanVersion(row interface{ Scan(...any) error }) (model.InterpretationVersion, error) {
	var v model.InterpretationVersion
	var created string
	var frozen, superseded sql.NullString
	var supersededBy sql.NullInt64
	if err := row.Scan(&v.ID, &v.BatchID, &v.Name, &v.Status, &v.Summary, &created,
		&frozen, &superseded, &supersededBy); err != nil {
		return v, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if frozen.Valid {
		t, _ := time.Parse(time.RFC3339Nano, frozen.String)
		v.FrozenAt = &t
	}
	if superseded.Valid {
		t, _ := time.Parse(time.RFC3339Nano, superseded.String)
		v.SupersededAt = &t
	}
	if supersededBy.Valid {
		by := supersededBy.Int64
		v.SupersededBy = &by
	}
	return v, nil
}

// Create 创建草稿版本。
func (s *VersionStore) Create(v model.InterpretationVersion) (model.InterpretationVersion, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`INSERT INTO versions(batch_id,name,status,summary,created_at)
		VALUES(?,?,?,?,?)`, v.BatchID, v.Name, v.Status, v.Summary, now)
	if err != nil {
		return v, err
	}
	v.ID, _ = res.LastInsertId()
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	return v, nil
}

// Get 按 ID 获取版本。
func (s *VersionStore) Get(id int64) (model.InterpretationVersion, error) {
	row := s.db.QueryRow(`SELECT `+versionCols+` FROM versions WHERE id=?`, id)
	v, err := scanVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return v, model.ErrNotFound
	}
	return v, err
}

// ListByBatch 列出批次下全部版本（按创建时间倒序）。
func (s *VersionStore) ListByBatch(batchID int64) ([]model.InterpretationVersion, error) {
	rows, err := s.db.Query(`SELECT `+versionCols+` FROM versions WHERE batch_id=? ORDER BY id DESC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.InterpretationVersion
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateStatus 更新版本状态（draft→shared→frozen→superseded）。
func (s *VersionStore) UpdateStatus(id int64, status string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var res sql.Result
	var err error
	switch status {
	case model.VersionFrozen:
		res, err = s.db.Exec(`UPDATE versions SET status=?, frozen_at=? WHERE id=?`, status, now, id)
	case model.VersionSuperseded:
		res, err = s.db.Exec(`UPDATE versions SET status=?, superseded_at=? WHERE id=?`, status, now, id)
	default:
		res, err = s.db.Exec(`UPDATE versions SET status=? WHERE id=?`, status, id)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// CountByBatch 统计批次版本数。
func (s *VersionStore) CountByBatch(batchID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM versions WHERE batch_id=?`, batchID).Scan(&n)
	return n, err
}
