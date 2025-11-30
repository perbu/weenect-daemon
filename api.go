package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	gosure "github.com/perbu/go-sure"
)

// APIServer provides HTTP API for tracker data
type APIServer struct {
	db         *Database
	cfg        *Config
	server     *http.Server
	logger     *slog.Logger
	sureClient *gosure.Client

	// Cache for SureHub pet status
	petStatusCache     map[string]petFlapStatus
	petStatusCacheTime time.Time
}

// NewAPIServer creates a new API server
func NewAPIServer(db *Database, cfg *Config, listenAddr string, logger *slog.Logger) *APIServer {
	api := &APIServer{
		db:     db,
		cfg:    cfg,
		logger: logger,
	}

	// Initialize SureHub client if credentials are configured
	if cfg.SureHubEmail != "" && cfg.SureHubPassword != "" {
		api.sureClient = gosure.NewClient(cfg.SureHubEmail, cfg.SureHubPassword)
		logger.Info("SureHub client initialized")
	}

	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/trackers", api.handleGetTrackers)
	mux.HandleFunc("/api/positions/", api.handleGetPositions)
	mux.HandleFunc("/api/status", api.handleGetStatus)
	mux.HandleFunc("/health", api.handleHealth)

	// Static file serving for web UI
	fs := http.FileServer(http.Dir("./web"))
	mux.Handle("/", fs)

	// Wrap with middleware
	handler := api.loggingMiddleware(api.corsMiddleware(mux))

	api.server = &http.Server{
		Addr:         listenAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return api
}

// Start starts the HTTP server
func (a *APIServer) Start() error {
	a.logger.Info("Starting HTTP API server", "addr", a.server.Addr)

	if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the HTTP server
func (a *APIServer) Shutdown(ctx context.Context) error {
	a.logger.Info("Shutting down HTTP API server")
	return a.server.Shutdown(ctx)
}

// corsMiddleware adds CORS headers for local development
func (a *APIServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests
func (a *APIServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Call the next handler
		next.ServeHTTP(w, r)

		duration := time.Since(start)
		a.logger.Debug("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"duration_ms", duration.Milliseconds(),
		)
	})
}

// writeJSON writes a JSON response
func (a *APIServer) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		a.logger.Error("Failed to encode JSON response", "error", err)
	}
}

// writeError writes an error response
func (a *APIServer) writeError(w http.ResponseWriter, status int, message string) {
	a.writeJSON(w, status, map[string]string{"error": message})
}

// handleHealth handles GET /health
func (a *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	a.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleGetTrackers handles GET /api/trackers
func (a *APIServer) handleGetTrackers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	trackers, err := a.db.GetAllTrackers()
	if err != nil {
		a.logger.Error("Failed to get trackers", "error", err)
		a.writeError(w, http.StatusInternalServerError, "Failed to retrieve trackers")
		return
	}

	// Return empty array instead of null if no trackers
	if trackers == nil {
		trackers = []TrackerWithCount{}
	}

	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"trackers": trackers,
	})
}

// handleGetPositions handles GET /api/positions/{trackerID}
func (a *APIServer) handleGetPositions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract tracker ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/positions/")
	trackerID, err := strconv.Atoi(path)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "Invalid tracker ID")
		return
	}

	// Check if tracker exists
	exists, err := a.db.TrackerExists(trackerID)
	if err != nil {
		a.logger.Error("Failed to check tracker existence", "tracker_id", trackerID, "error", err)
		a.writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !exists {
		a.writeError(w, http.StatusNotFound, "Tracker not found")
		return
	}

	// Parse query parameters for date range
	query := r.URL.Query()
	var start, end time.Time

	if startStr := query.Get("start"); startStr != "" {
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			a.writeError(w, http.StatusBadRequest, "Invalid start date format (use RFC3339)")
			return
		}
	} else {
		// Default: last 7 days
		start = time.Now().AddDate(0, 0, -7)
	}

	if endStr := query.Get("end"); endStr != "" {
		end, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			a.writeError(w, http.StatusBadRequest, "Invalid end date format (use RFC3339)")
			return
		}
	} else {
		// Default: now
		end = time.Now()
	}

	// Get positions
	positions, err := a.db.GetPositions(trackerID, start, end)
	if err != nil {
		a.logger.Error("Failed to get positions", "tracker_id", trackerID, "error", err)
		a.writeError(w, http.StatusInternalServerError, "Failed to retrieve positions")
		return
	}

	// Return empty array instead of null if no positions
	if positions == nil {
		positions = []SimplePosition{}
	}

	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"tracker_id": trackerID,
		"start":      start.Format(time.RFC3339),
		"end":        end.Format(time.RFC3339),
		"count":      len(positions),
		"positions":  positions,
	})
}

// StatusResponse represents the /api/status response for the radar display
type StatusResponse struct {
	Home struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"home"`
	Trackers []TrackerStatus `json:"trackers"`
	POIs     []POI           `json:"pois"`
}

// HistoryPoint represents a position in the tracker's recent history
type HistoryPoint struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Timestamp time.Time `json:"timestamp"`
}

// TrackerStatus represents a tracker's current status for radar display
type TrackerStatus struct {
	ID        int            `json:"id"`
	Name      string         `json:"name"`
	Color     string         `json:"color"`
	Lat       float64        `json:"lat"`
	Lon       float64        `json:"lon"`
	Battery   *int           `json:"battery,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	IsInside  *bool          `json:"is_inside,omitempty"`  // From SureHub pet flap
	LastFlap  *string        `json:"last_flap,omitempty"`  // Time of last flap activity
	History   []HistoryPoint `json:"history,omitempty"`    // Recent position history for trail
}

// Color palette for auto-assigning tracker colors
var trackerColors = []string{
	"#ff6b6b", // red
	"#4ecdc4", // teal
	"#ffe66d", // yellow
	"#95e1d3", // mint
	"#f38181", // coral
	"#aa96da", // lavender
	"#fcbad3", // pink
	"#a8d8ea", // sky blue
}

// handleGetStatus handles GET /api/status - returns latest positions for radar
func (a *APIServer) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	positions, err := a.db.GetLatestPositions()
	if err != nil {
		a.logger.Error("Failed to get latest positions", "error", err)
		a.writeError(w, http.StatusInternalServerError, "Failed to retrieve positions")
		return
	}

	// Fetch pet status from SureHub if configured
	petStatus := make(map[string]petFlapStatus)
	if a.sureClient != nil {
		petStatus = a.fetchPetStatus()
	}

	resp := StatusResponse{}
	resp.Home.Lat = a.cfg.HomeLat
	resp.Home.Lon = a.cfg.HomeLon

	// Time window for history trail (3 hours)
	historyStart := time.Now().Add(-3 * time.Hour)

	for i, p := range positions {
		color := trackerColors[i%len(trackerColors)]
		tracker := TrackerStatus{
			ID:        p.TrackerID,
			Name:      p.TrackerName,
			Color:     color,
			Lat:       p.Latitude,
			Lon:       p.Longitude,
			Battery:   p.Battery,
			Timestamp: p.Timestamp,
		}

		// Fetch recent position history for trail
		recentPositions, err := a.db.GetRecentPositions(p.TrackerID, historyStart)
		if err != nil {
			a.logger.Error("Failed to get recent positions for trail", "tracker_id", p.TrackerID, "error", err)
		} else if len(recentPositions) > 0 {
			tracker.History = make([]HistoryPoint, len(recentPositions))
			for j, pos := range recentPositions {
				tracker.History[j] = HistoryPoint{
					Lat:       pos.Latitude,
					Lon:       pos.Longitude,
					Timestamp: pos.Timestamp,
				}
			}
		}

		// Match pet status by name (case-insensitive)
		if status, ok := petStatus[strings.ToLower(p.TrackerName)]; ok {
			tracker.IsInside = &status.isInside
			if status.lastFlap != nil {
				formatted := status.lastFlap.Format(time.RFC3339)
				tracker.LastFlap = &formatted
			}
		}

		resp.Trackers = append(resp.Trackers, tracker)
	}

	// Return empty array instead of null if no trackers
	if resp.Trackers == nil {
		resp.Trackers = []TrackerStatus{}
	}

	// Add POIs from config
	if a.cfg.POIs != nil {
		resp.POIs = a.cfg.POIs
	} else {
		resp.POIs = []POI{}
	}

	a.writeJSON(w, http.StatusOK, resp)
}

// petFlapStatus holds the status from SureHub pet flap
type petFlapStatus struct {
	isInside bool
	lastFlap *time.Time
}

// fetchPetStatus retrieves pet inside/outside status from SureHub (with caching)
func (a *APIServer) fetchPetStatus() map[string]petFlapStatus {
	// Cache for 5 minutes (matches default sync schedule)
	cacheDuration := 5 * time.Minute

	// Return cached data if still valid
	if a.petStatusCache != nil && time.Since(a.petStatusCacheTime) < cacheDuration {
		a.logger.Debug("Using cached pet status", "age", time.Since(a.petStatusCacheTime).Round(time.Second))
		return a.petStatusCache
	}

	result := make(map[string]petFlapStatus)

	dashboard, err := a.sureClient.GetDashboard()
	if err != nil {
		a.logger.Error("Failed to fetch SureHub dashboard", "error", err)
		// Return stale cache if available
		if a.petStatusCache != nil {
			a.logger.Debug("Returning stale cache due to error")
			return a.petStatusCache
		}
		return result
	}

	for _, pet := range dashboard.Pets {
		name := strings.ToLower(pet.Name)
		status := petFlapStatus{}

		if pet.Position != nil && pet.Position.Where != nil {
			status.isInside = *pet.Position.Where == gosure.PetPositionInside
			if pet.Position.Since != nil {
				status.lastFlap = pet.Position.Since
			}
		}

		result[name] = status
	}

	// Update cache
	a.petStatusCache = result
	a.petStatusCacheTime = time.Now()

	a.logger.Debug("Fetched pet status from SureHub", "pets", len(result))
	return result
}
