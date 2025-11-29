# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go daemon that syncs GPS position data from Weenect trackers to a local SQLite database. It runs on a schedule (default: nightly at 2am) and supports manual syncs and historical data backfill.

## Build and Run

```bash
# Build the binary
go build -o weenect-daemon

# Run daemon with scheduled syncs
./weenect-daemon run

# Manual sync
./weenect-daemon sync-now

# Backfill historical data
./weenect-daemon backfill --start-date 2024-01-01

# View status
./weenect-daemon status

# View statistics
./weenect-daemon stats
```

## Configuration

Configuration is loaded from (in priority order):
1. Environment variables (highest)
2. Config file from default locations: `./config.json`, `./weenect-daemon.json`, `~/.config/weenect/config.json`, `~/.weenect/config.json`
3. Command-line defaults

Required configuration:
- `WEENECT_USERNAME` or `username` in config.json
- `WEENECT_PASSWORD` or `password` in config.json

Copy `config.example.json` to `config.json` to get started.

## Architecture

The daemon consists of several key components:

### main.go
Entry point with command routing. Commands: `run`, `sync-now`, `backfill`, `status`, `stats`, `version`.

### config.go
Configuration loading from files and environment variables. Uses `loadConfig()` which searches default paths automatically.

### worker.go (SyncWorker)
Core sync logic that:
- Authenticates with Weenect API using github.com/perbu/weenect-go client
- Fetches tracker list and position data
- Splits large date ranges into 24-hour chunks (API limitation)
- Stores positions idempotently in SQLite
- Updates `last_sync_timestamp` after each successful chunk for resumability

Key methods:
- `SyncAll(ctx)` - Sync all trackers incrementally from last sync time
- `SyncTracker(ctx, trackerID)` - Sync specific tracker
- `BackfillAll(ctx, start, end)` - Backfill all trackers for date range
- `BackfillTracker(ctx, trackerID, start, end)` - Backfill specific tracker
- `fetchAndStorePositions()` - Internal method that handles 24h chunking

### database.go
SQLite operations using modernc.org/sqlite driver. Three tables:
- `trackers` - Tracker metadata with `last_sync_timestamp`
- `positions` - GPS positions (idempotent inserts by position ID)
- `sync_log` - Sync operation history

All position inserts use `ON CONFLICT(id) DO NOTHING` for idempotency.

### scheduler.go
Cron-based scheduling using github.com/robfig/cron/v3. Runs sync jobs on configured schedule with graceful shutdown support.

### ratelimit.go
Token bucket rate limiter to respect API limits (default: 4 req/sec).

### logger.go
Structured logging using slog. Debug level logs all API requests/responses and rate limiter activity.

## Key Design Patterns

**Idempotency**: Position records use API-provided IDs as primary keys with `ON CONFLICT DO NOTHING`, allowing safe re-syncing without duplicates.

**Resumability**: Each tracker stores `last_sync_timestamp`. Syncs are incremental from this timestamp. Updated after each successful 24h chunk, so interrupted syncs can resume.

**24-hour chunking**: API has 24h max range. `fetchAndStorePositions()` automatically splits larger ranges and updates sync time after each chunk.

**Rate limiting**: All API calls go through `rateLimiter.Wait()` before execution.

## Testing Changes

Since this is a daemon, test by:
1. Building: `go build`
2. Running manual sync: `./weenect-daemon sync-now` (requires valid credentials)
3. Checking database: `sqlite3 weenect.db "SELECT COUNT(*) FROM positions"`
4. Enabling debug logs: `export WEENECT_LOG_LEVEL=debug`

## Dependencies

- github.com/perbu/weenect-go - Weenect API client
- github.com/robfig/cron/v3 - Cron scheduler
- modernc.org/sqlite - Pure Go SQLite driver
- Standard library: log/slog, database/sql, context, time, encoding/json