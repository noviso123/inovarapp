package alertcron

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNextScheduledRunsUseSaoPauloWallClock(t *testing.T) {
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name          string
		now           time.Time
		daily, hourly string
	}{
		{"before daily", time.Date(2026, 10, 4, 11, 0, 0, 0, time.UTC), "2026-10-04 09:00 -03:00", "2026-10-04 09:00 -03:00"},
		{"after daily", time.Date(2026, 10, 4, 12, 30, 0, 0, time.UTC), "2026-10-05 09:00 -03:00", "2026-10-04 10:00 -03:00"},
		{"at daily boundary", time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), "2026-10-05 09:00 -03:00", "2026-10-04 10:00 -03:00"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			for _, check := range []struct {
				got  time.Time
				want string
			}{{NextDailyRun(test.now, location), test.daily}, {NextHourlyRun(test.now, location), test.hourly}} {
				if formatted := check.got.Format("2006-01-02 15:04 -07:00"); formatted != check.want {
					t.Errorf("scheduled run=%s want=%s", formatted, check.want)
				}
			}
		})
	}
}

func TestRunScheduledStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (Handler{}).RunScheduled(ctx, time.Now, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("scheduler error=%v", err)
	}
}
