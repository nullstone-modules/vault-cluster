package vaultcluster

import (
	"fmt"
	"strings"

	"github.com/robfig/cron/v3"
)

func ParseBackupSchedule(expr string) (cron.Schedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, nil
	}
	sched, err := cron.ParseStandard(expr)
	if err != nil {
		return nil, fmt.Errorf("backup_schedule: %w", err)
	}
	return sched, nil
}
