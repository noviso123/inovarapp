package httpapi

import (
	"context"
	"log"
	"time"
)

type MaintenanceScheduler func(context.Context, time.Time) error

func syncMaintenanceAfterMutation(ctx context.Context, scheduler MaintenanceScheduler) {
	if scheduler != nil {
		if err := scheduler(ctx, time.Now()); err != nil {
			// Keep the successfully saved service/history. Queue refresh and the
			// daily reconciliation retry scheduling; never claim delivery here.
			log.Print("maintenance reminder scheduling failed; reconciliation required")
		}
	}
}
