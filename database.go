package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Database provides SQLite database operations
type Database struct {
	db *sql.DB
}

// TrackerRecord represents a tracker in the database
type TrackerRecord struct {
	ID                int
	Name              string
	LastSyncTimestamp time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// PositionRecord represents a position in the database
type PositionRecord struct {
	ID           string
	TrackerID    int
	Timestamp    time.Time
	Latitude     float64
	Longitude    float64
	Battery      *int
	Speed        *float64
	Direction    *int
	ValidSignal  *bool
	Satellites   *int
	GSM          *int
	Type         *string
	LastMessage  *time.Time
	DateServer   *time.Time
	DateTracker  *time.Time
	CreatedAt    time.Time
}

// SyncLogRecord represents a sync log entry
type SyncLogRecord struct {
	ID               int
	TrackerID        *int
	SyncTime         time.Time
	PositionsFetched int
	StartDate        *time.Time
	EndDate          *time.Time
	Success          bool
	ErrorMessage     *string
	DurationMs       int
}

// StatusInfo holds daemon status information
type StatusInfo struct {
	TrackerCount      int
	PositionCount     int
	LastSyncTime      time.Time
	LastSyncSuccess   bool
	LastSyncPositions int
	LastSyncError     string
}

// TrackerStats holds statistics for a tracker
type TrackerStats struct {
	TrackerID     int
	TrackerName   string
	PositionCount int
	FirstPosition time.Time
	LastPosition  time.Time
	LastSync      time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS trackers (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  last_sync_timestamp DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS positions (
  id TEXT PRIMARY KEY,
  tracker_id INTEGER NOT NULL,
  timestamp DATETIME NOT NULL,
  latitude REAL NOT NULL,
  longitude REAL NOT NULL,
  battery INTEGER,
  speed REAL,
  direction INTEGER,
  valid_signal BOOLEAN,
  satellites INTEGER,
  gsm INTEGER,
  type TEXT,
  last_message DATETIME,
  date_server DATETIME,
  date_tracker DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (tracker_id) REFERENCES trackers(id)
);

CREATE INDEX IF NOT EXISTS idx_positions_tracker_timestamp
  ON positions(tracker_id, timestamp);

CREATE TABLE IF NOT EXISTS sync_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tracker_id INTEGER,
  sync_time DATETIME NOT NULL,
  positions_fetched INTEGER DEFAULT 0,
  start_date DATETIME,
  end_date DATETIME,
  success BOOLEAN NOT NULL,
  error_message TEXT,
  duration_ms INTEGER,
  FOREIGN KEY (tracker_id) REFERENCES trackers(id)
);
`

// initDatabase initializes the database with schema
func initDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Create schema
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &Database{db: db}, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// UpsertTracker inserts or updates a tracker
func (d *Database) UpsertTracker(id int, name string) error {
	query := `
		INSERT INTO trackers (id, name, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := d.db.Exec(query, id, name)
	return err
}

// GetTracker retrieves a tracker by ID
func (d *Database) GetTracker(id int) (*TrackerRecord, error) {
	query := `
		SELECT id, name, last_sync_timestamp, created_at, updated_at
		FROM trackers WHERE id = ?
	`
	var t TrackerRecord
	var lastSync sql.NullTime

	err := d.db.QueryRow(query, id).Scan(
		&t.ID, &t.Name, &lastSync, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if lastSync.Valid {
		t.LastSyncTimestamp = lastSync.Time
	}

	return &t, nil
}

// UpdateTrackerSyncTime updates the last sync timestamp for a tracker
func (d *Database) UpdateTrackerSyncTime(id int, syncTime time.Time) error {
	query := `
		UPDATE trackers
		SET last_sync_timestamp = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := d.db.Exec(query, syncTime, id)
	return err
}

// InsertPosition inserts a position (idempotent by position ID)
func (d *Database) InsertPosition(p *PositionRecord) error {
	query := `
		INSERT INTO positions (
			id, tracker_id, timestamp, latitude, longitude,
			battery, speed, direction, valid_signal, satellites,
			gsm, type, last_message, date_server, date_tracker,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO NOTHING
	`
	_, err := d.db.Exec(query,
		p.ID, p.TrackerID, p.Timestamp, p.Latitude, p.Longitude,
		p.Battery, p.Speed, p.Direction, p.ValidSignal, p.Satellites,
		p.GSM, p.Type, p.LastMessage, p.DateServer, p.DateTracker,
	)
	return err
}

// InsertSyncLog logs a sync operation
func (d *Database) InsertSyncLog(log *SyncLogRecord) error {
	query := `
		INSERT INTO sync_log (
			tracker_id, sync_time, positions_fetched,
			start_date, end_date, success, error_message, duration_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := d.db.Exec(query,
		log.TrackerID, log.SyncTime, log.PositionsFetched,
		log.StartDate, log.EndDate, log.Success, log.ErrorMessage, log.DurationMs,
	)
	return err
}

// GetStatus returns overall daemon status
func (d *Database) GetStatus() (*StatusInfo, error) {
	var status StatusInfo

	// Get tracker count
	err := d.db.QueryRow("SELECT COUNT(*) FROM trackers").Scan(&status.TrackerCount)
	if err != nil {
		return nil, err
	}

	// Get position count
	err = d.db.QueryRow("SELECT COUNT(*) FROM positions").Scan(&status.PositionCount)
	if err != nil {
		return nil, err
	}

	// Get last sync info
	query := `
		SELECT sync_time, success, positions_fetched, error_message
		FROM sync_log
		ORDER BY sync_time DESC
		LIMIT 1
	`
	var syncTime sql.NullTime
	var errorMsg sql.NullString
	err = d.db.QueryRow(query).Scan(
		&syncTime, &status.LastSyncSuccess, &status.LastSyncPositions, &errorMsg,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if syncTime.Valid {
		status.LastSyncTime = syncTime.Time
	}
	if errorMsg.Valid {
		status.LastSyncError = errorMsg.String
	}

	return &status, nil
}

// GetStats returns statistics for trackers
func (d *Database) GetStats(trackerID int) ([]TrackerStats, error) {
	query := `
		SELECT
			t.id,
			t.name,
			COUNT(p.id) as position_count,
			MIN(p.timestamp) as first_position,
			MAX(p.timestamp) as last_position,
			t.last_sync_timestamp
		FROM trackers t
		LEFT JOIN positions p ON t.id = p.tracker_id
	`

	args := []interface{}{}
	if trackerID > 0 {
		query += " WHERE t.id = ?"
		args = append(args, trackerID)
	}

	query += " GROUP BY t.id ORDER BY t.name"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []TrackerStats
	for rows.Next() {
		var s TrackerStats
		var firstPos, lastPos, lastSync sql.NullTime

		err := rows.Scan(
			&s.TrackerID, &s.TrackerName, &s.PositionCount,
			&firstPos, &lastPos, &lastSync,
		)
		if err != nil {
			return nil, err
		}

		if firstPos.Valid {
			s.FirstPosition = firstPos.Time
		}
		if lastPos.Valid {
			s.LastPosition = lastPos.Time
		}
		if lastSync.Valid {
			s.LastSync = lastSync.Time
		}

		stats = append(stats, s)
	}

	return stats, rows.Err()
}