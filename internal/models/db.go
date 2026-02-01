package models

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
	mu sync.RWMutex
}

type Request struct {
	ID            int        `json:"id"`
	RequesterName string     `json:"requester_name"`
	RequesterEmail *string   `json:"requester_email"`
	MediaType     string     `json:"media_type"`
	TmdbID        *int       `json:"tmdb_id"`
	TvdbID        *int       `json:"tvdb_id"`
	ImdbID        *string    `json:"imdb_id"`
	Title         string     `json:"title"`
	Year          *int       `json:"year"`
	Poster        *string    `json:"poster"`
	Status        string     `json:"status"`
	AdminNotes    *string    `json:"admin_notes"`
	ArrID         *int       `json:"arr_id"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	NotifiedAt    *time.Time `json:"notified_at"`
}

type Activity struct {
	ID        int       `json:"id"`
	Action    string    `json:"action"`
	Details   *string   `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

func InitDB(dbPath string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	// Set connection pool settings
	sqlDB.SetMaxOpenConns(1) // SQLite only supports one writer
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	db := &DB{DB: sqlDB}

	if err := db.createTables(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			requester_name TEXT NOT NULL,
			requester_email TEXT,
			media_type TEXT DEFAULT 'series',
			tmdb_id INTEGER,
			tvdb_id INTEGER,
			imdb_id TEXT,
			title TEXT NOT NULL,
			year INTEGER,
			poster TEXT,
			status TEXT DEFAULT 'pending',
			admin_notes TEXT,
			arr_id INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			notified_at TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS activity_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action TEXT NOT NULL,
			details TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_requests_status ON requests(status)`,
		`CREATE INDEX IF NOT EXISTS idx_requests_media_type ON requests(media_type)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}

	return nil
}

// Settings functions
func (db *DB) GetSetting(key string) string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		return ""
	}
	return value
}

func (db *DB) SetSetting(key, value string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, value)
	return err
}

func (db *DB) SetSettingIfNotExists(key, value string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.Exec("INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)", key, value)
	return err
}

func (db *DB) GetAllSettings() (map[string]string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	return settings, nil
}

// Request functions
func (db *DB) CreateRequest(req *Request) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.Exec(`
		INSERT INTO requests (requester_name, requester_email, media_type, tmdb_id, tvdb_id, imdb_id, title, year, poster, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')
	`, req.RequesterName, req.RequesterEmail, req.MediaType, req.TmdbID, req.TvdbID, req.ImdbID, req.Title, req.Year, req.Poster)
	
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (db *DB) GetRequests(status, mediaType string) ([]Request, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	query := "SELECT id, requester_name, requester_email, media_type, tmdb_id, tvdb_id, imdb_id, title, year, poster, status, admin_notes, arr_id, created_at, updated_at, notified_at FROM requests WHERE 1=1"
	args := []interface{}{}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	if mediaType != "" {
		query += " AND media_type = ?"
		args = append(args, mediaType)
	}
	query += " ORDER BY created_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []Request
	for rows.Next() {
		var r Request
		err := rows.Scan(&r.ID, &r.RequesterName, &r.RequesterEmail, &r.MediaType, &r.TmdbID, &r.TvdbID, &r.ImdbID, &r.Title, &r.Year, &r.Poster, &r.Status, &r.AdminNotes, &r.ArrID, &r.CreatedAt, &r.UpdatedAt, &r.NotifiedAt)
		if err != nil {
			return nil, err
		}
		requests = append(requests, r)
	}
	return requests, nil
}

func (db *DB) GetRequest(id int) (*Request, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var r Request
	err := db.QueryRow(`
		SELECT id, requester_name, requester_email, media_type, tmdb_id, tvdb_id, imdb_id, title, year, poster, status, admin_notes, arr_id, created_at, updated_at, notified_at 
		FROM requests WHERE id = ?
	`, id).Scan(&r.ID, &r.RequesterName, &r.RequesterEmail, &r.MediaType, &r.TmdbID, &r.TvdbID, &r.ImdbID, &r.Title, &r.Year, &r.Poster, &r.Status, &r.AdminNotes, &r.ArrID, &r.CreatedAt, &r.UpdatedAt, &r.NotifiedAt)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (db *DB) GetApprovedRequests() ([]Request, error) {
	return db.GetRequests("approved", "")
}

func (db *DB) UpdateRequestStatus(id int, status, adminNotes string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.Exec("UPDATE requests SET status = ?, admin_notes = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", status, adminNotes, id)
	return err
}

func (db *DB) UpdateRequestArrID(id, arrID int) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.Exec("UPDATE requests SET arr_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", arrID, id)
	return err
}

func (db *DB) CheckDuplicateRequest(mediaType string, tmdbID, tvdbID *int) (bool, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var count int
	var err error

	if mediaType == "series" && tvdbID != nil {
		err = db.QueryRow("SELECT COUNT(*) FROM requests WHERE tvdb_id = ? AND media_type = 'series' AND status = 'pending'", *tvdbID).Scan(&count)
	} else if mediaType == "movie" && tmdbID != nil {
		err = db.QueryRow("SELECT COUNT(*) FROM requests WHERE tmdb_id = ? AND media_type = 'movie' AND status = 'pending'", *tmdbID).Scan(&count)
	}

	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (db *DB) GetRequestedIDs(mediaType string) (map[int]bool, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var query string
	if mediaType == "series" {
		query = "SELECT tvdb_id FROM requests WHERE media_type = 'series' AND status IN ('pending', 'approved') AND tvdb_id IS NOT NULL"
	} else {
		query = "SELECT tmdb_id FROM requests WHERE media_type = 'movie' AND status IN ('pending', 'approved') AND tmdb_id IS NOT NULL"
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, nil
}

// Stats
func (db *DB) GetStats() (map[string]int, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	stats := map[string]int{
		"total":     0,
		"pending":   0,
		"approved":  0,
		"rejected":  0,
		"completed": 0,
	}

	rows, err := db.Query(`
		SELECT 
			COUNT(*) as total,
			SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending,
			SUM(CASE WHEN status = 'approved' THEN 1 ELSE 0 END) as approved,
			SUM(CASE WHEN status = 'rejected' THEN 1 ELSE 0 END) as rejected,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed
		FROM requests
	`)
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	if rows.Next() {
		var total, pending, approved, rejected, completed int
		rows.Scan(&total, &pending, &approved, &rejected, &completed)
		stats["total"] = total
		stats["pending"] = pending
		stats["approved"] = approved
		stats["rejected"] = rejected
		stats["completed"] = completed
	}
	return stats, nil
}

// Activity log
func (db *DB) LogActivity(action string, details map[string]interface{}) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var detailsJSON *string
	if details != nil {
		b, _ := json.Marshal(details)
		s := string(b)
		detailsJSON = &s
	}

	_, err := db.Exec("INSERT INTO activity_log (action, details) VALUES (?, ?)", action, detailsJSON)
	return err
}

func (db *DB) GetActivity(limit int) ([]Activity, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.Query("SELECT id, action, details, created_at FROM activity_log ORDER BY created_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.Action, &a.Details, &a.CreatedAt); err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}
	return activities, nil
}
