package store

import (
	"database/sql"
	"errors"

	"task234-thinsect/internal/model"
)

// MineralStore 提供矿物库查询，用于特征→候选标签匹配。
type MineralStore struct{ db *sql.DB }

// NewMineralStore 创建矿物库 store。
func NewMineralStore(db *sql.DB) *MineralStore { return &MineralStore{db: db} }

// List 列出全部矿物。
func (s *MineralStore) List() ([]model.Mineral, error) {
	rows, err := s.db.Query(`SELECT code, name, color_hint, extinction_hint, description FROM minerals ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Mineral
	for rows.Next() {
		var m model.Mineral
		if err := rows.Scan(&m.Code, &m.Name, &m.ColorHint, &m.ExtinctionHint, &m.Description); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Get 按编码获取矿物；不存在返回 ErrNotFound。
func (s *MineralStore) Get(code string) (model.Mineral, error) {
	var m model.Mineral
	err := s.db.QueryRow(`SELECT code, name, color_hint, extinction_hint, description FROM minerals WHERE code=?`, code).
		Scan(&m.Code, &m.Name, &m.ColorHint, &m.ExtinctionHint, &m.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return m, model.ErrNotFound
	}
	return m, err
}
