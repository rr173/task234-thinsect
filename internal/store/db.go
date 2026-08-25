// Package store 提供 SQLite 持久化：建表迁移与各实体的存取。
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open 打开（必要时创建）SQLite 数据库并设置稳健的并发参数。
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)&_txlock=immediate", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// migrate 建立全部业务表并写入种子矿物库（幂等，可重复执行）。
func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS batches (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE,
			rock_type TEXT NOT NULL,
			locality TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS images (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			name TEXT NOT NULL,
			mode TEXT NOT NULL,
			sha256 TEXT NOT NULL,
			width REAL NOT NULL,
			height REAL NOT NULL,
			avg_brightness REAL NOT NULL DEFAULT 0,
			color_r REAL NOT NULL DEFAULT 0,
			color_g REAL NOT NULL DEFAULT 0,
			color_b REAL NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			UNIQUE(batch_id, sha256)
		)`,
		`CREATE TABLE IF NOT EXISTS regions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			image_id INTEGER NOT NULL REFERENCES images(id),
			label TEXT NOT NULL,
			mineral_code TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			area REAL NOT NULL DEFAULT 0,
			perimeter REAL NOT NULL DEFAULT 0,
			avg_r REAL NOT NULL DEFAULT 0,
			avg_g REAL NOT NULL DEFAULT 0,
			avg_b REAL NOT NULL DEFAULT 0,
			extinction_ratio REAL NOT NULL DEFAULT 1,
			ext_angle REAL NOT NULL DEFAULT 0,
			parent_region_id INTEGER REFERENCES regions(id),
			polygon_json TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_regions_batch ON regions(batch_id)`,
		`CREATE INDEX IF NOT EXISTS idx_regions_image ON regions(image_id)`,
		`CREATE TABLE IF NOT EXISTS relationships (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			region_a INTEGER NOT NULL REFERENCES regions(id),
			region_b INTEGER NOT NULL REFERENCES regions(id),
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(region_a, region_b, kind)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_rels_batch ON relationships(batch_id)`,
		`CREATE TABLE IF NOT EXISTS opinions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			region_id INTEGER NOT NULL REFERENCES regions(id),
			kind TEXT NOT NULL,
			content TEXT NOT NULL,
			author TEXT NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_opinions_region ON opinions(region_id)`,
		`CREATE TABLE IF NOT EXISTS versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL REFERENCES batches(id),
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			frozen_at DATETIME,
			superseded_at DATETIME,
			superseded_by INTEGER
		)`,
		`CREATE INDEX IF NOT EXISTS idx_versions_batch ON versions(batch_id)`,
		`CREATE TABLE IF NOT EXISTS minerals (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			color_hint TEXT NOT NULL,
			extinction_hint REAL NOT NULL,
			description TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return seedMinerals(db)
}

// seedMinerals 写入矿物库种子（UPSERT，重复执行安全）。
func seedMinerals(db *sql.DB) error {
	rows := []struct {
		code, name, color, desc string
		ext                     float64
	}{
		{"olivine", "橄榄石", "黄绿色", "高突起，特征裂纹，干涉色二级蓝绿", 0.45},
		{"pyroxene", "辉石", "浅褐/浅绿", "近正交解理，干涉色二级中", 0.55},
		{"plagioclase", "斜长石", "无色/白", "聚片双晶，低突起", 0.70},
		{"alkali-feldspar", "钾长石", "无色/肉红", "卡斯巴双晶，低突起", 0.75},
		{"quartz", "石英", "无色", "波状消光，无解理", 0.80},
		{"biotite", "黑云母", "深褐", "极完全解理，平行消光", 0.35},
		{"hornblende", "角闪石", "绿/褐", "两组解理 124°，斜消光", 0.50},
		{"glass", "火山玻璃", "棕/黑", "均质性，全消光，无解理", 0.90},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO minerals(code,name,color_hint,extinction_hint,description)
			VALUES(?,?,?,?,?)
			ON CONFLICT(code) DO UPDATE SET name=excluded.name, color_hint=excluded.color_hint,
				extinction_hint=excluded.extinction_hint, description=excluded.description`,
			r.code, r.name, r.color, r.ext, r.desc); err != nil {
			return fmt.Errorf("seed minerals: %w", err)
		}
	}
	return nil
}
