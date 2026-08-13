package gscclient

import (
	"fmt"
	"sync"
	"time"
)

// Default URL Inspection daily limit used only for local soft warnings.
// Description text must not hardcode this number; link to the official limits page.
const defaultInspectDailyLimit = 2000

// DailyQuota is a per-site in-memory daily counter. It never hard-fails; callers
// may surface a warning when usage approaches the configured limit.
type DailyQuota struct {
	mu            sync.Mutex
	limit         int
	warnAt        int
	counts        map[string]int
	day           string // YYYY-MM-DD in UTC
	now           func() time.Time
}

// NewDailyQuota creates a counter with the given daily limit. A non-positive
// limit falls back to defaultInspectDailyLimit. Warnings fire at 90% usage.
func NewDailyQuota(limit int) *DailyQuota {
	if limit <= 0 {
		limit = defaultInspectDailyLimit
	}
	return &DailyQuota{
		limit:  limit,
		warnAt: int(float64(limit) * 0.9),
		counts: make(map[string]int),
		now:    time.Now,
	}
}

// Inc increments the counter for siteURL and returns the new count plus an
// optional warning when usage is at or above the warn threshold.
func (q *DailyQuota) Inc(siteURL string) (count int, warning string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	today := q.now().UTC().Format("2006-01-02")
	if q.day != today {
		q.day = today
		q.counts = make(map[string]int)
	}

	q.counts[siteURL]++
	count = q.counts[siteURL]
	if count >= q.warnAt {
		warning = fmt.Sprintf(
			"URL Inspection quota for this property is at %d of ~%d local soft limit today; see https://developers.google.com/webmaster-tools/limits",
			count, q.limit,
		)
	}
	return count, warning
}

// Count returns the current day's count for siteURL without incrementing.
func (q *DailyQuota) Count(siteURL string) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	today := q.now().UTC().Format("2006-01-02")
	if q.day != today {
		return 0
	}
	return q.counts[siteURL]
}
