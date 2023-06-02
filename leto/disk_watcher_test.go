package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/formicidae-tracker/leto"
	"github.com/formicidae-tracker/leto/leto/mock_main"
	olympuspb "github.com/formicidae-tracker/olympus/api"
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
	var err error

	s.env.FreeStartBytes, _, err = fsStat(s.Dir)
	c.Assert(err, IsNil)
	ctx, cancel := context.WithCancel(context.Background())
	s.watcher = NewDiskWatcher(ctx, s.env, s.olympus).(*diskWatcher)
	s.watcher.period = period
	s.cancel = cancel

	s.env.Start = time.Now()
}

func (s *DiskWatcherSuite) TearDownTest(c *C) {
	s.ctrl.Finish()
}

func (s *DiskWatcherSuite) TestCanReadFs(c *C) {
	free, total, err := fsStat(s.Dir)
	c.Check(err, IsNil)
	c.Check(total >= free, Equals, true)
	free, total, err = fsStat(filepath.Join(s.Dir, "do-no-exist"))
	c.Check(err, ErrorMatches, "could not get available size for .*: no such file or directory")
	c.Check(free, Equals, int64(0))
	c.Check(total, Equals, int64(0))
}

func (s *DiskWatcherSuite) TestWatcherFailsWhenLimitExceed(c *C) {
	timeout := 300 * time.Millisecond
	s.env.Leto.DiskLimit = s.env.FreeStartBytes + 1000*1024
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

func (s *DiskWatcherSuite) TestWatcherWarnNearLimit(c *C) {
	sync := make(chan int, 3)

	gomock.InOrder(
		s.olympus.EXPECT().
			PushDiskStatus(gomock.Any(), gomock.Any()).
			Do(func(x, y any) {
				sync <- 0
				expected := &AlarmUpdateMatches{
					Identification: "tracking.disk_status",
					Level:          olympuspb.AlarmLevel_WARNING,
					Status:         olympuspb.AlarmStatus_ON,
					Description:    "low free disk space .*, will stop in ~ 11h",
				}
				c.Logf("%s", y.(*olympuspb.AlarmUpdate).Description)
				c.Check(expected.Matches(y), Equals, true)
			}),
		s.olympus.EXPECT().
			PushDiskStatus(gomock.Any(), gomock.Any()).
			Do(func(x, y any) {
				sync <- 1
				c.Check(y, IsNil)
			}), // do not report same alarm twice
		s.olympus.EXPECT().
			PushDiskStatus(gomock.Any(), gomock.Any()). // reports when alarms stops
			Do(func(x, y any) {
				sync <- 2
				expected := &AlarmUpdateMatches{
					Identification: "tracking.disk_status",
					Level:          olympuspb.AlarmLevel_WARNING,
					Status:         olympuspb.AlarmStatus_OFF,
					Description:    "",
				}
				c.Check(expected.Matches(y), Equals, true)
			}),
	)

	filenames := []string{
		filepath.Join(s.Dir, c.TestName()+".1"),
		filepath.Join(s.Dir, c.TestName()+".2"),
	}
	s.env.Start = time.Now()
	s.env.FreeStartBytes, _, _ = fsStat(s.Dir)
	s.env.Leto.DiskLimit = computeDiskLimit(s.env.FreeStartBytes, 6*time.Hour)
	errs := Start(s.watcher)
	ioutil.WriteFile(filenames[0], make([]byte, filesize), 0644)

	//wait for first report
	<-sync

	// rewrite a new file to keep same BPS, and produce the same alarm
	ioutil.WriteFile(filenames[1], make([]byte, filesize), 0644)
	<-sync

	// removes all files to clear the alarm (effective BPS will become 0)
	c.Assert(os.Remove(filenames[0]), IsNil)
	c.Assert(os.Remove(filenames[1]), IsNil)
	<-sync
	// stops the watcher
	s.cancel()
	err, ok := <-errs
	c.Check(err, IsNil)
	c.Check(ok, Equals, true)
}

func (s *DiskWatcherSuite) TestWatcherCritsNearLimit(c *C) {
	sync := make(chan int, 1)
	// when near to minutes, the tricks do not work well, simply we do
	// not check the time computation nor the number sent.
	s.olympus.EXPECT().
		PushDiskStatus(gomock.Any(), gomock.Any()).
		Do(func(x, y any) {
			sync <- 0
			expected := &AlarmUpdateMatches{
				Identification: "tracking.disk_status",
				Level:          olympuspb.AlarmLevel_EMERGENCY,
				Status:         olympuspb.AlarmStatus_ON,
				Description:    "critically low free disk space .*, will stop in ~ .*",
			}
			c.Check(expected.Matches(y), Equals, true)
		})

	ioutil.WriteFile(filepath.Join(s.Dir, c.TestName()), make([]byte, filesize), 0644)
	s.env.Leto.DiskLimit = computeDiskLimit(s.env.FreeStartBytes, 3*time.Minute)
	errs := Start(s.watcher)
	<-sync
	s.cancel()
	err, ok := <-errs
	c.Check(err, IsNil)
	c.Check(ok, Equals, true)
}
