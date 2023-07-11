package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"time"

	"github.com/atuleu/go-humanize"
	"github.com/formicidae-tracker/leto/cmd/leto/mock_main"
	"github.com/formicidae-tracker/leto/internal/leto"
	olympuspb "github.com/formicidae-tracker/olympus/pkg/api"
	"github.com/golang/mock/gomock"
	. "gopkg.in/check.v1"
)

type DiskWatcherSuite struct {
	Dir     string
	cancel  context.CancelFunc
	env     *TrackingEnvironment
	watcher *diskWatcher
	ctrl    *gomock.Controller
	olympus *mock_main.MockOlympusTask
}

var period = 20 * time.Millisecond
var filesize int64 = 4096

var _ = Suite(&DiskWatcherSuite{})

func (s *DiskWatcherSuite) SetUpSuite(c *C) {
	s.Dir = c.MkDir()
}

func (s *DiskWatcherSuite) SetUpTest(c *C) {
	s.ctrl = gomock.NewController(c)
	s.olympus = mock_main.NewMockOlympusTask(s.ctrl)
	s.env = &TrackingEnvironment{
		ExperimentDir: s.Dir,
		Leto:          leto.DefaultConfig,
	}

	free, _, err := getDiskSize(s.Dir)

	c.Assert(err, IsNil)
	ctx, cancel := context.WithCancel(context.Background())
	s.watcher = NewDiskWatcher(ctx, s.env, s.olympus).(*diskWatcher)
	s.watcher.period = period
	s.cancel = cancel

	s.env.Start = time.Now()
	s.env.Rate = NewByteRateEstimator(free, s.env.Start)
}

func (s *DiskWatcherSuite) TearDownTest(c *C) {
	s.ctrl.Finish()
}

func (s *DiskWatcherSuite) TestCanReadFs(c *C) {
	free, total, err := getDiskSize(s.Dir)
	c.Check(err, IsNil)
	c.Check(total >= free, Equals, true)
	free, total, err = getDiskSize(filepath.Join(s.Dir, "do-no-exist"))
	c.Check(err, ErrorMatches, "could not get available size for .*: no such file or directory")
	c.Check(free, Equals, int64(0))
	c.Check(total, Equals, int64(0))
}

func (s *DiskWatcherSuite) TestWatcherFailsWhenLimitExceed(c *C) {
	timeout := 300 * time.Millisecond
	s.env.DiskLimit = s.env.Rate.freeStartBytes + 1000*1024
	s.watcher.olympus = nil
	errs := Start(s.watcher)
	select {
	case err := <-errs:
		c.Check(err, ErrorMatches, "unsufficient disk space: available: .* minimum: .*")
	case <-time.After(timeout):
		s.cancel()
		c.Fatalf("disk watcher did not fail after %s", timeout)
	}
}

func computeDiskLimit(startFreeByte int64, targetETA time.Duration) int64 {
	return startFreeByte - int64(float64(filesize)/period.Seconds()*targetETA.Seconds())
}

type AlarmUpdateMatches olympuspb.AlarmUpdate

func (m *AlarmUpdateMatches) Matches(x interface{}) bool {
	xx, ok := x.(*olympuspb.AlarmUpdate)
	if ok == false || xx == nil {
		return false
	}

	if xx.Identification != m.Identification {
		return false
	}

	if xx.Level != m.Level {
		return false
	}

	if xx.Time.AsTime().After(time.Now()) {
		return false
	}

	rx := regexp.MustCompile(m.Description)
	return rx.MatchString(xx.Description)
}

func (m *AlarmUpdateMatches) String() string {
	return fmt.Sprintf("identification: \"%s\" level: %s status: %s description:\"%s\"",
		m.Identification,
		olympuspb.AlarmLevel_name[int32(m.Level)],
		olympuspb.AlarmStatus_name[int32(m.Status)],
		m.Description)
}

func (s *DiskWatcherSuite) TestWatcherDoNotAlarmIfFarFromLimits(c *C) {
	sync := make(chan int, 1)
	// when near to minutes, the tricks do not work well, simply we do
	// not check the time computation nor the number sent.
	s.olympus.EXPECT().
		PushDiskStatus(gomock.Any(), gomock.Nil()).
		Do(func(x, y any) {
			sync <- 0
		})

	ioutil.WriteFile(filepath.Join(s.Dir, c.TestName()), make([]byte, filesize), 0644)
	s.env.DiskLimit = computeDiskLimit(s.env.Rate.freeStartBytes, humanize.Day)
	errs := Start(s.watcher)
	<-sync
	s.cancel()
	err, ok := <-errs
	c.Check(err, IsNil)
	c.Check(ok, Equals, true)
}

func (s *DiskWatcherSuite) TestWatcherWarnNearLimit(c *C) {
	s.env.DiskLimit = 10 * 1024 * 1024 //10mB
	status := &olympuspb.DiskStatus{
		FreeBytes:      20 * 1024 * 1024, // 20 MiB
		TotalBytes:     40 * 1024 * 1024, // 40 MiB
		BytesPerSecond: 485,              // 10 MiB / ( 6 * 3600 s)
	}
	update := s.watcher.buildAlarmUpdate(status, time.Unix(32, 43))
	c.Assert(update, Not(IsNil))
	c.Check(update.Identification, Equals, "tracking.disk_status")
	c.Check(update.Level, Equals, olympuspb.AlarmLevel_WARNING)
	c.Check(update.Status, Equals, olympuspb.AlarmStatus_ON)
	c.Check(update.Time.AsTime(), Equals, time.Unix(32, 43).UTC())
	c.Check(update.Description, Equals, "low free disk space ( 20.0 MiB ), will stop in ~ 6h")

	// with almost the same state (1s later), it should not build a new one
	status.FreeBytes -= 485
	c.Check(s.watcher.buildAlarmUpdate(status, time.Unix(33, 6789)), IsNil)

	// closer, it becomes an emergency
	status.FreeBytes -= 485
	status.BytesPerSecond = 3000
	update = s.watcher.buildAlarmUpdate(status, time.Unix(34, 69))
	c.Assert(update, Not(IsNil))
	c.Check(update.Identification, Equals, "tracking.disk_status")
	c.Check(update.Level, Equals, olympuspb.AlarmLevel_EMERGENCY)
	c.Check(update.Status, Equals, olympuspb.AlarmStatus_ON)
	c.Check(update.Time.AsTime(), Equals, time.Unix(34, 69).UTC())
	c.Check(update.Description, Equals, "critically low free disk space ( 20.0 MiB ), will stop in ~ 58m0s")

	// big drop in BPS will stop the alarm
	status.BytesPerSecond = 0
	update = s.watcher.buildAlarmUpdate(status, time.Unix(35, 12))
	c.Assert(update, Not(IsNil))
	c.Check(update.Identification, Equals, "tracking.disk_status")
	c.Check(update.Level, Equals, olympuspb.AlarmLevel_WARNING)
	c.Check(update.Status, Equals, olympuspb.AlarmStatus_OFF)
	c.Check(update.Time.AsTime(), Equals, time.Unix(35, 12).UTC())
	c.Check(update.Description, Equals, "")
}
