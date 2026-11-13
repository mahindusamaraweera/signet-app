package jobs

import (
	"testing"
	"time"
)

func TestSharePeriodLogTime(t *testing.T) {
	cases := []struct {
		periodStart time.Time
		want        string
	}{
		{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "2026-10-31 18:29:59"},
		{time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), "2026-09-30 18:29:59"},
		{time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), "2026-12-31 18:29:59"},
		{time.Date(2028, 2, 1, 0, 0, 0, 0, time.UTC), "2028-02-29 18:29:59"},
	}
	for _, c := range cases {
		got := sharePeriodLogTime(c.periodStart)
		if got.Location() != time.UTC {
			t.Errorf("%s: location = %v, want UTC", c.periodStart.Format("2006-01"), got.Location())
		}
		if s := got.Format("2006-01-02 15:04:05"); s != c.want {
			t.Errorf("%s: got %s, want %s", c.periodStart.Format("2006-01"), s, c.want)
		}
	}
}
