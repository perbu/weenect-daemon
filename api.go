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
)

// APIServer provides HTTP API for tracker data
type APIServer struct {
	db     *Database
	server *http.Server
	logger *slog.Logger
}

// NewAPIServer creates a new API server
func NewAPIServer(db *Database, port int, logger *slog.Logger) *APIServer {
	api := &APIServer{
		db:     db,
		logger: logger,
	}

	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/trackers", api.handleGetTrackers)
	mux.HandleFunc("/api/positions/", api.handleGetPositions)
	mux.HandleFunc("/health", api.handleHealth)

	// Static file serving for web UI
	fs := http.FileServer(http.Dir("./web"))
	mux.Handle("/", fs)

	// Wrap with middleware
	handler := api.loggingMiddleware(api.corsMiddleware(mux))

	api.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
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
