package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/google/uuid"
	_ "modernc.org/sqlite" // Pure-Go zero-CGO SQLite driver
)

// EditSummary represents one row in FAWE's SQLite rollback summary database (`_edits`).
type EditSummary struct {
	Player  uuid.UUID
	ID      int
	Time    time.Time
	Min     plugin.BlockPos
	Max     plugin.BlockPos
	Size    int64
	Command string
	World   string
}

// RollbackDB implements FAWE's upstream RollbackDatabase schema in pure-Go SQLite.
// File path: <historyDir>/<world>/summary.db
// Table: `_edits (player BLOB(16), id INT, time INT, x1 INT, x2 INT, z1 INT, z2 INT, y1 INT, y2 INT, size BIGINT, command VARCHAR, PRIMARY KEY (player, id))`
// Note: y1 and y2 are stored with FAWE's upstream -128 offset for backwards compatibility.
type RollbackDB struct {
	mu         sync.Mutex
	db         *sql.DB
	worldName  string
	historyDir string
	dbPath     string
}

// OpenRollbackDB opens (or creates) the SQLite rollback summary database for worldName.
func OpenRollbackDB(historyDir, worldName string, deleteAfterDays int) (*RollbackDB, error) {
	dir := filepath.Join(historyDir, SanitizeWorldDir(worldName))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dir, "summary.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open rollback sqlite %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1)

	rdb := &RollbackDB{
		db:         db,
		worldName:  worldName,
		historyDir: historyDir,
		dbPath:     dbPath,
	}
	if err := rdb.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if deleteAfterDays > 0 {
		_, _ = rdb.Purge(time.Duration(deleteAfterDays) * 24 * time.Hour)
	}
	return rdb, nil
}

func (r *RollbackDB) init() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	const createSQL = `CREATE TABLE IF NOT EXISTS "_edits" (
		"player" BLOB(16) NOT NULL,
		"id" INT NOT NULL,
		"time" INT NOT NULL,
		"x1" INT NOT NULL,
		"x2" INT NOT NULL,
		"z1" INT NOT NULL,
		"z2" INT NOT NULL,
		"y1" INT NOT NULL,
		"y2" INT NOT NULL,
		"size" BIGINT NOT NULL,
		"command" VARCHAR,
		PRIMARY KEY ("player", "id")
	)`
	if _, err := r.db.Exec(createSQL); err != nil {
		return fmt.Errorf("init _edits table: %w", err)
	}
	// Migrate legacy FAWE `edits` table if present (matching RollbackDatabase.java).
	_, _ = r.db.Exec(`ALTER TABLE "edits" ADD COLUMN "command" VARCHAR`)
	_, _ = r.db.Exec(`ALTER TABLE "edits" ADD COLUMN "size" BIGINT DEFAULT 0 NOT NULL`)
	if res, err := r.db.Exec(`INSERT OR IGNORE INTO "_edits" (player, id, time, x1, x2, z1, z2, y1, y2, size, command)
		SELECT player, id, time, x1, x2, z1, z2, y1, y2, size, command FROM "edits"`); err == nil {
		_ = res
		_, _ = r.db.Exec(`DROP TABLE "edits"`)
	}
	return nil
}

// LogEdit inserts or replaces an edit summary in `_edits`.
func (r *RollbackDB) LogEdit(s EditSummary) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.Exec(
		`INSERT OR REPLACE INTO "_edits" (player, id, time, x1, x2, z1, z2, y1, y2, command, size) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Player[:],
		s.ID,
		int(s.Time.Unix()),
		s.Min.X,
		s.Max.X,
		s.Min.Z,
		s.Max.Z,
		s.Min.Y-128,
		s.Max.Y-128,
		s.Command,
		s.Size,
	)
	return err
}

// GetEdit retrieves a single edit summary by player UUID and edit ID.
func (r *RollbackDB) GetEdit(player uuid.UUID, id int) (EditSummary, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.db.QueryRow(
		`SELECT player, id, time, x1, x2, z1, z2, y1, y2, size, command FROM "_edits" WHERE player = ? AND id = ?`,
		player[:], id,
	)
	s, err := r.scanRow(row)
	if err == sql.ErrNoRows {
		return EditSummary{}, false, nil
	}
	if err != nil {
		return EditSummary{}, false, err
	}
	return s, true, nil
}

// QueryEdits searches edits intersecting [pos1..pos2], optionally filtered by player and minimum time.
func (r *RollbackDB) QueryEdits(player *uuid.UUID, minTime time.Time, pos1, pos2 plugin.BlockPos, ascending bool, limit int) ([]EditSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lo := plugin.BlockPos{X: min(pos1.X, pos2.X), Y: min(pos1.Y, pos2.Y), Z: min(pos1.Z, pos2.Z)}
	hi := plugin.BlockPos{X: max(pos1.X, pos2.X), Y: max(pos1.Y, pos2.Y), Z: max(pos1.Z, pos2.Z)}

	query := `SELECT player, id, time, x1, x2, z1, z2, y1, y2, size, command FROM "_edits"
		WHERE time >= ? AND x2 >= ? AND x1 <= ? AND z2 >= ? AND z1 <= ? AND y2 >= ? AND y1 <= ?`
	args := []any{int(minTime.Unix()), lo.X, hi.X, lo.Z, hi.Z, lo.Y - 128, hi.Y - 128}
	if player != nil {
		query += ` AND player = ?`
		args = append(args, (*player)[:])
	}
	if ascending {
		query += ` ORDER BY time ASC, id ASC`
	} else {
		query += ` ORDER BY time DESC, id DESC`
	}
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EditSummary
	for rows.Next() {
		s, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// QueryByPlayer returns recent edits by a player across the whole world.
func (r *RollbackDB) QueryByPlayer(player uuid.UUID, limit int) ([]EditSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	query := `SELECT player, id, time, x1, x2, z1, z2, y1, y2, size, command FROM "_edits" WHERE player = ? ORDER BY time DESC, id DESC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}
	rows, err := r.db.Query(query, player[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EditSummary
	for rows.Next() {
		s, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Delete removes a single edit record.
func (r *RollbackDB) Delete(player uuid.UUID, id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.Exec(`DELETE FROM "_edits" WHERE player = ? AND id = ?`, player[:], id)
	return err
}

// Purge deletes edit summaries older than maxAge.
func (r *RollbackDB) Purge(maxAge time.Duration) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := int(time.Now().Add(-maxAge).Unix())
	res, err := r.db.Exec(`DELETE FROM "_edits" WHERE time < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ImportFromDisk scans .bd files under <historyDir>/<world>/ and indexes them into `_edits`.
func (r *RollbackDB) ImportFromDisk() (int, error) {
	worldDir := filepath.Join(r.historyDir, SanitizeWorldDir(r.worldName))
	imported := 0
	err := filepath.WalkDir(worldDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".bd") {
			return nil
		}
		cs, err := LoadChangeSetFromDisk(path)
		if err != nil {
			return nil
		}
		lo, hi, ok := cs.Bounds()
		if !ok {
			return nil
		}
		if err := r.LogEdit(EditSummary{
			Player:  cs.PlayerUUID,
			ID:      cs.ID,
			Time:    cs.Timestamp,
			Min:     lo,
			Max:     hi,
			Size:    cs.Size(),
			Command: cs.Command,
			World:   r.worldName,
		}); err == nil {
			imported++
		}
		return nil
	})
	return imported, err
}

// Close closes the underlying SQLite connection.
func (r *RollbackDB) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db == nil {
		return nil
	}
	err := r.db.Close()
	r.db = nil
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func (r *RollbackDB) scanRow(s scanner) (EditSummary, error) {
	var rawUUID []byte
	var id, ts, x1, x2, z1, z2, y1, y2 int
	var size int64
	var cmd sql.NullString
	if err := s.Scan(&rawUUID, &id, &ts, &x1, &x2, &z1, &z2, &y1, &y2, &size, &cmd); err != nil {
		return EditSummary{}, err
	}
	var uid uuid.UUID
	if len(rawUUID) == 16 {
		copy(uid[:], rawUUID)
	}
	return EditSummary{
		Player:  uid,
		ID:      id,
		Time:    time.Unix(int64(ts), 0),
		Min:     plugin.BlockPos{X: x1, Y: y1 + 128, Z: z1},
		Max:     plugin.BlockPos{X: x2, Y: y2 + 128, Z: z2},
		Size:    size,
		Command: cmd.String,
		World:   r.worldName,
	}, nil
}
