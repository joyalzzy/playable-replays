package api

import "time"

const (
	sessionMutationLimit  = 30
	sessionMutationWindow = 10 * time.Second
)

type fixedWindowLimiter struct {
	started time.Time
	used    int
}

func (l *fixedWindowLimiter) allow(now time.Time, limit int, window time.Duration) (bool, time.Duration) {
	if l.started.IsZero() || !now.Before(l.started.Add(window)) {
		l.started = now
		l.used = 0
	}
	if l.used >= limit {
		return false, l.started.Add(window).Sub(now)
	}
	l.used++
	return true, 0
}
