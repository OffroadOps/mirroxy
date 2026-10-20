package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var errStateMissing = errors.New("state missing")

type stateStore struct {
	path string
	key  []byte
}

func (a *App) ensureStore() (*stateStore, error) {
	a.storeMu.Lock()
	defer a.storeMu.Unlock()
	if a.store != nil {
		return a.store, nil
	}
	if err := os.MkdirAll(a.dataPath(""), 0700); err != nil {
		return nil, err
	}
	keyPath := a.dataPath("Mirroxy.key")
	key, err := loadOrCreateDataKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("无法打开本机加密密钥: %w", err)
	}
	dbPath := a.dataPath("Mirroxy.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	for _, statement := range []string{
		`PRAGMA busy_timeout=5000`, `PRAGMA journal_mode=DELETE`, `PRAGMA synchronous=FULL`, `PRAGMA secure_delete=ON`,
		`CREATE TABLE IF NOT EXISTS state (name TEXT PRIMARY KEY, nonce BLOB NOT NULL, payload BLOB NOT NULL, updated_at INTEGER NOT NULL)`,
	} {
		if _, err = db.Exec(statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	db.Close()
	a.store = &stateStore{path: dbPath, key: key}
	return a.store, nil
}

func (s *stateStore) database() (*sql.DB, error) {
	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *stateStore) seal(plain []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return nonce, gcm.Seal(nil, nonce, plain, nil), nil
}
func (s *stateStore) open(nonce, payload []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, payload, nil)
}
func (a *App) writeState(name string, value any) error {
	s, err := a.ensureStore()
	if err != nil {
		return err
	}
	plain, err := json.Marshal(value)
	if err != nil {
		return err
	}
	nonce, payload, err := s.seal(plain)
	if err != nil {
		return err
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO state(name,nonce,payload,updated_at) VALUES(?,?,?,unixepoch()) ON CONFLICT(name) DO UPDATE SET nonce=excluded.nonce,payload=excluded.payload,updated_at=excluded.updated_at`, name, nonce, payload)
	return err
}
func (a *App) readState(name string, value any) error {
	s, err := a.ensureStore()
	if err != nil {
		return err
	}
	var nonce, payload []byte
	db, err := s.database()
	if err != nil {
		return err
	}
	defer db.Close()
	err = db.QueryRow(`SELECT nonce,payload FROM state WHERE name=?`, name).Scan(&nonce, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return errStateMissing
	}
	if err != nil {
		return err
	}
	plain, err := s.open(nonce, payload)
	if err != nil {
		return fmt.Errorf("加密状态校验失败: %w", err)
	}
	if err = json.Unmarshal(plain, value); err != nil {
		return fmt.Errorf("状态数据损坏: %w", err)
	}
	return nil
}
func (a *App) deleteState(name string) error {
	s, e := a.ensureStore()
	if e != nil {
		return e
	}
	db, e := s.database()
	if e != nil {
		return e
	}
	defer db.Close()
	_, e = db.Exec(`DELETE FROM state WHERE name=?`, name)
	return e
}
func (a *App) closeStore() { a.storeMu.Lock(); a.store = nil; a.storeMu.Unlock() }

func removeLegacyPlaintext(dataDir string) {
	patterns := []string{"settings.json", "settings.before-*.json*", "traffic.json", "traffic.before-*.json*", "xray.json", "xray.log", "sing-box-*.json", "sing-box-*.log", "runtime/Mirroxy_app_router.json"}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(dataDir, pattern))
		for _, path := range matches {
			_ = os.Remove(path)
		}
	}
}
