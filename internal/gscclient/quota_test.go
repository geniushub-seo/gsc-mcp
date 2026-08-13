package gscclient

import (
	"testing"
	"time"
)

func TestDailyQuota_IncAndWarn(t *testing.T) {
	t.Parallel()
	q := NewDailyQuota(10)
	q.warnAt = 9
	q.now = func() time.Time { return time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC) }

	for i := 1; i <= 8; i++ {
		count, warning := q.Inc("sc-domain:example.com")
		if count != i {
			t.Fatalf("count = %d, want %d", count, i)
		}
		if warning != "" {
			t.Fatalf("unexpected warning at count %d: %s", i, warning)
		}
	}

	count, warning := q.Inc("sc-domain:example.com")
	if count != 9 {
		t.Fatalf("count = %d, want 9", count)
	}
	if warning == "" {
		t.Fatal("expected warning at warn threshold")
	}
}

func TestDailyQuota_ResetsAcrossDays(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	q := NewDailyQuota(100)
	q.now = func() time.Time { return day }

	q.Inc("sc-domain:example.com")
	q.Inc("sc-domain:example.com")
	if got := q.Count("sc-domain:example.com"); got != 2 {
		t.Fatalf("count before reset = %d, want 2", got)
	}

	day = day.Add(24 * time.Hour)
	if got := q.Count("sc-domain:example.com"); got != 0 {
		t.Fatalf("count after day change without Inc = %d, want 0", got)
	}

	count, _ := q.Inc("sc-domain:example.com")
	if count != 1 {
		t.Fatalf("count after reset = %d, want 1", count)
	}
}

func TestDailyQuota_SeparateSites(t *testing.T) {
	t.Parallel()
	q := NewDailyQuota(100)
	q.now = func() time.Time { return time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC) }

	q.Inc("sc-domain:a.com")
	q.Inc("sc-domain:a.com")
	q.Inc("sc-domain:b.com")

	if got := q.Count("sc-domain:a.com"); got != 2 {
		t.Fatalf("a.com count = %d, want 2", got)
	}
	if got := q.Count("sc-domain:b.com"); got != 1 {
		t.Fatalf("b.com count = %d, want 1", got)
	}
}
