package main

import (
	"context"
	"fmt"
	"math"
	"time"

	olympuspb "github.com/formicidae-tracker/olympus/api"
	"golang.org/x/sys/unix"
)

func fsStat(path string) (free int64, total int64, err error) {
	var stat unix.Statfs_t

	err = unix.Statfs(path, &stat)
	if err != nil {
		return 0, 0, err
	}

	return int64(stat.Bfree * uint64(stat.Bsize)), int64(stat.Blocks * uint64(stat.Bsize)), nil
}

type DiskWatcher interface {
	Task
}

type diskWatcher struct {
	env     *TrackingEnvironment
	ctx     context.Context
	olympus OlympusTask
	update  *olympuspb.AlarmUpdate
}

func NewDiskWatcher(ctx context.Context, env *TrackingEnvironment, olympus OlympusTask) DiskWatcher {
	return &diskWatcher{
		env:     env,
		ctx:     ctx,
		olympus: olympus,
	}
}

func (w *diskWatcher) Run() error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := w.pollDisk(now); err != nil {
				return err
			}
		}
	}
}

func (w *diskWatcher) pollDisk(now time.Time) error {
	free, total, bps, err := w.env.WatchDisk(now)
	if err != nil {
		return err
	}
	status := &olympuspb.DiskStatus{
		FreeBytes:      free,
		TotalBytes:     total,
		BytesPerSecond: bps,
	}

	if status.FreeBytes < w.env.Leto.DiskLimit {
		return fmt.Errorf("unsufficient disk space: available: %s minimum: %s",
			ByteSize(status.FreeBytes), ByteSize(w.env.Leto.DiskLimit))
	}

	if w.olympus == nil {
		return nil
	}

	update := w.buildAlarmUpdate(status, now)

	w.olympus.PushDiskStatus(status, update)

	return nil
}

func (w *diskWatcher) computeETA(status *olympuspb.DiskStatus) time.Duration {
	if status.BytesPerSecond <= 0 {
		return math.MaxInt64
	}

	remaining := status.FreeBytes - w.env.Leto.DiskLimit

	return time.Duration(float64(remaining) / float64(status.BytesPerSecond) * float64(time.Second))
}

func (w *diskWatcher) computeAlarmUpdate(status *olympuspb.DiskStatus) *olympuspb.AlarmUpdate {
	eta := w.computeETA(status)

	update := &olympuspb.AlarmUpdate{
		Identification: "tracking.disk_status",
		Status:         olympuspb.AlarmStatus_OFF,
		Level:          olympuspb.AlarmLevel_WARNING,
	}

	if eta < 12*time.Hour {
		update.Status = olympuspb.AlarmStatus_ON
		update.Description = fmt.Sprintf("low disk free disk space %s, will stop in ~ %s",
			ByteSize(status.FreeBytes), eta.Round(time.Hour))
	} else if eta < 1*time.Hour {
		update.Status = olympuspb.AlarmStatus_ON
		update.Level = olympuspb.AlarmLevel_EMERGENCY
		update.Description = fmt.Sprintf("critically low free disk space %s, will stop in ~ %s",
			ByteSize(status.FreeBytes), eta.Round(time.Minute))
	}

	return update
}

func (w *diskWatcher) buildAlarmUpdate(status *olympuspb.DiskStatus, now time.Time) *olympuspb.AlarmUpdate {
	update := w.computeAlarmUpdate(status)

	last := w.update
	if last == nil {
		last = &olympuspb.AlarmUpdate{
			Status:      olympuspb.AlarmStatus_OFF,
			Level:       update.Level,
			Description: update.Description,
		}
	}

	defer func() {
		w.update = update
	}()

	if last.Status == update.Status &&
		last.Level == update.Level &&
		last.Description == update.Description {
		return nil
	}
	return update
}
