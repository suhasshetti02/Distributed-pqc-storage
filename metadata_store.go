package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

type ShardMeta struct {
	Index  int    `json:"index"`
	CID    string `json:"cid"`
	NodeID string `json:"nodeId"`
}

type FileRecord struct {
	Key          string      `json:"key"`
	CID          string      `json:"cid"`
	OriginalSize int64       `json:"originalSize"`
	CreatedAt    time.Time   `json:"createdAt"`
	Shards       []ShardMeta `json:"shards"`
}

type MetadataStore struct {
	db *sql.DB
}

func (m *MetadataStore) Close() error {
	return m.db.Close()
}

func NewMetadataStore(path string) (*MetadataStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &MetadataStore{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (m *MetadataStore) migrate() error {
	_, err := m.db.Exec(`
CREATE TABLE IF NOT EXISTS files (
  key TEXT PRIMARY KEY,
  cid TEXT NOT NULL,
  original_size INTEGER NOT NULL,
  shards_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_files_cid ON files(cid);
`)
	return err
}

func (m *MetadataStore) UpsertFile(key, cid string, originalSize int64, shards []ShardMeta) error {
	shardsJSON, err := json.Marshal(shards)
	if err != nil {
		return err
	}

	_, err = m.db.Exec(`
INSERT INTO files(key, cid, original_size, shards_json, created_at)
VALUES(?, ?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
  cid = excluded.cid,
  original_size = excluded.original_size,
  shards_json = excluded.shards_json,
  created_at = excluded.created_at;
`, key, cid, originalSize, string(shardsJSON), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (m *MetadataStore) GetByCID(cid string) (*FileRecord, error) {
	var rec FileRecord
	var createdAt string
	var shardsJSON string
	err := m.db.QueryRow(`SELECT key, cid, original_size, shards_json, created_at FROM files WHERE cid = ? LIMIT 1`, cid).
		Scan(&rec.Key, &rec.CID, &rec.OriginalSize, &shardsJSON, &createdAt)
	if err != nil {
		return nil, err
	}
	rec.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	_ = json.Unmarshal([]byte(shardsJSON), &rec.Shards)
	return &rec, nil
}

func (m *MetadataStore) GetByKey(key string) (FileRecord, error) {
	var rec FileRecord
	var createdAt string
	var shardsJSON string
	err := m.db.QueryRow(`SELECT key, cid, original_size, shards_json, created_at FROM files WHERE key = ?`, key).
		Scan(&rec.Key, &rec.CID, &rec.OriginalSize, &shardsJSON, &createdAt)
	if err != nil {
		return FileRecord{}, err
	}
	rec.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return FileRecord{}, err
	}
	if err := json.Unmarshal([]byte(shardsJSON), &rec.Shards); err != nil {
		return FileRecord{}, err
	}
	return rec, nil
}

func (m *MetadataStore) List(limit int) ([]FileRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := m.db.Query(`SELECT key, cid, original_size, shards_json, created_at FROM files ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []FileRecord
	for rows.Next() {
		var rec FileRecord
		var createdAt string
		var shardsJSON string
		if err := rows.Scan(&rec.Key, &rec.CID, &rec.OriginalSize, &shardsJSON, &createdAt); err != nil {
			return nil, err
		}
		
		// Handle potential time format issues
		rec.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		
		if err := json.Unmarshal([]byte(shardsJSON), &rec.Shards); err != nil {
			// fallback for legacy data
			rec.Shards = []ShardMeta{}
		}
		result = append(result, rec)
	}
	return result, nil
}

func (m *MetadataStore) KeyByCID(cid string) (string, error) {
	var key string
	err := m.db.QueryRow(`SELECT key FROM files WHERE cid = ? LIMIT 1`, cid).Scan(&key)
	if err != nil {
		return "", err
	}
	return key, nil
}

func (m *MetadataStore) HasKey(key string) (bool, error) {
	var k string
	err := m.db.QueryRow(`SELECT key FROM files WHERE key = ?`, key).Scan(&k)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func (m *MetadataStore) HasCID(cid string) (bool, error) {
	_, err := m.KeyByCID(cid)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func (m *MetadataStore) GetAll() ([]FileRecord, error) {
	rows, err := m.db.Query("SELECT key, cid, original_size, shards_json, created_at FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []FileRecord
	for rows.Next() {
		var rec FileRecord
		var createdAt string
		var shardsJSON string
		if err := rows.Scan(&rec.Key, &rec.CID, &rec.OriginalSize, &shardsJSON, &createdAt); err != nil {
			return nil, err
		}
		rec.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		json.Unmarshal([]byte(shardsJSON), &rec.Shards)
		result = append(result, rec)
	}
	return result, nil
}
func (m *MetadataStore) GetShardManifest(cid string) ([]ShardMeta, error) {
	var shardsJSON string
	err := m.db.QueryRow("SELECT shards_json FROM files WHERE cid = ?", cid).Scan(&shardsJSON)
	if err != nil {
		return nil, err
	}
	var shards []ShardMeta
	err = json.Unmarshal([]byte(shardsJSON), &shards)
	return shards, err
}
