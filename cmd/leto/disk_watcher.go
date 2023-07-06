package main

import (
	"context"
	"fmt"
	"math"
	"path"
	"time"

	"sync/atomic"

	"github.com/atuleu/go-humanize"
	olympuspb "github.com/formicidae-tracker/olympus/pkg/api"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// getDiskSize returns the free and total bytes available on the disk
// containing the file path. The fiel must exists.
func getDiskSize(path string) (free int64, total int64, err error) {
	var stat unix.Statfs_t

	err = unix.Statfs(path, &stat)
	if err != nil {
		return 0, 0, fmt.Errorf("could not get available size for %s: %w", path, err)
	}

	return int64(stat.Bavail * uint64(stat.Bsize)), int64(stat.Blocks * uint64(stat.Bsize)), nil
}

type DiskWatcher interface {
	Task
}

type diskWatcher struct {
	env     *TrackingEnvironment
	ctx     context.Context
	olympus OlympusTask
	update  *olympuspb.AlarmUpdate
	period  time.Duration

	counter atomic.Int64
}

func NewDiskWatcher(ctx context.Context, env *TrackingEnvironment, olympus OlympusTask) DiskWatcher {
	res := &diskWatcher{
		env:     env,
		ctx:     ctx,
		olympus: olympus,
		period:  5 * time.Second,
	}
	res.counter.Store(0)

	otel.Meter(instrumentationName).
		Int64ObservableUpDownCounter(path.Join("leto", "diskUsage"),
			metric.WithInt64Callback(BuildAtomicInt64Callback(&res.counter)),
		)

	return res
}

func (w *diskWatcher) Run() error {
	ticker := time.NewTicker(w.period)
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

	w.counter.Store(total - free)

	if status.FreeBytes < w.env.Leto.DiskLimit {
		return fmt.Errorf("unsufficient disk space: available: %s minimum: %s",
			humanize.ByteSize(status.FreeBytes), humanize.ByteSize(w.env.Leto.DiskLimit))
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

func (w *diskWatcher) computeAlarmUpdate(status *olympuspb.DiskStatus, now time.Time) *olympuspb.AlarmUpdate {
	eta := w.computeETA(status)

	update := &olympuspb.AlarmUpdate{
		Identification: "tracking.disk_status",
		Status:         olympuspb.AlarmStatus_OFF,
		Level:          olympuspb.AlarmLevel_WARNING,
		Time:           timestamppb.New(now),
	}

	if eta < 12*time.Hour {
		update.Status = olympuspb.AlarmStatus_ON
		update.Description = fmt.Sprintf("low free disk space ( %s ), will stop in ~ %s",
			humanize.ByteSize(status.FreeBytes), humanize.Duration(eta.Round(10*time.Minute)))
	}

	if eta < 1*time.Hour {
		update.Status = olympuspb.AlarmStatus_ON
		update.Level = olympuspb.AlarmLevel_EMERGENCY
		update.Description = fmt.Sprintf("critically low free disk space ( %s ), will stop in ~ %s",
			humanize.ByteSize(status.FreeBytes), humanize.Duration(eta.Round(time.Minute)))
	}

	return update
}

func (w *diskWatcher) buildAlarmUpdate(status *olympuspb.DiskStatus, now time.Time) *olympuspb.AlarmUpdate {
	update := w.computeAlarmUpdate(status, now)

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
