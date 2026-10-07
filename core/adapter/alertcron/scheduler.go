package alertcron

import (
	"context"
	"time"
)

const dailyRunHour = 9

type ScheduleReporter func(job string, result map[string]any, err error)

// RunScheduled runs the existing Go cron use cases from a dedicated singleton
// worker. Keep this worker at one replica while the schedule uses in-process
// timers; the HTTP endpoints remain available for managed external schedulers.
func (h Handler) RunScheduled(ctx context.Context, now func() time.Time, location *time.Location, report ScheduleReporter) error {
	if now == nil {
		now = time.Now
	}
	if location == nil {
		location = saoPauloLocation()
	}
	for {
		current := now()
		daily := NextDailyRun(current, location)
		hourly := NextHourlyRun(current, location)
		next := daily
		if hourly.Before(next) {
			next = hourly
		}
		wait := next.Sub(current)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
		current = now().In(location)
		if !current.Before(daily) {
			result, err := h.runDaily(ctx, current.UTC())
			if report != nil {
				report("alertas-diarios", result, err)
			}
			current = now().In(location)
		}
		if !current.Before(hourly) {
			result, err := h.runOneHour(ctx, current.UTC())
			if report != nil {
				report("push-agenda-hora", result, err)
			}
		}
	}
}

// NextDailyRun returns the next 09:00 local execution time.
func NextDailyRun(now time.Time, location *time.Location) time.Time {
	if location == nil {
		location = saoPauloLocation()
	}
	local := now.In(location)
	next := time.Date(local.Year(), local.Month(), local.Day(), dailyRunHour, 0, 0, 0, location)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// NextHourlyRun returns the next top-of-hour boundary in the supplied zone.
func NextHourlyRun(now time.Time, location *time.Location) time.Time {
	if location == nil {
		location = saoPauloLocation()
	}
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour()+1, 0, 0, 0, location)
}

func saoPauloLocation() *time.Location {
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*60*60)
	}
	return location
}
