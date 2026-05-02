package schedule

import (
	"time"

	"github.com/robfig/cron/v3"
)

// NextRunAfter computes the next occurrence of the given cron expression after
// the specified time using robfig/cron/v3.
func NextRunAfter(cronExpr string, after time.Time) (time.Time, error) {
	sched, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(after), nil
}
