package main

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter wraps rate.Limiter for API throttling
type RateLimiter struct {
	limiter *rate.Limiter
	logger  *slog.Logger
}

// newRateLimiter creates a new rate limiter
// rps is requests per second
func newRateLimiter(rps float64, logger *slog.Logger) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(rps), 1),
		logger:  logger,
	}
}

// Wait blocks until rate limit allows another request
func (r *RateLimiter) Wait(ctx context.Context) error {
	// Check if we need to wait
	reservation := r.limiter.Reserve()
	if !reservation.OK() {
		return ctx.Err()
	}

	delay := reservation.Delay()
	if delay > 0 {
		r.logger.Debug("Rate limiter: waiting before request", "delay_ms", delay.Milliseconds())
	}

	// Actually wait
	time.Sleep(delay)
	return nil
}