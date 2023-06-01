package main

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/formicidae-tracker/leto"
	"github.com/golang/mock/gomock"
	. "gopkg.in/check.v1"
)

type DiskWatcherSuite struct {
	Dir     string
	cancel  context.CancelFunc
	env     *TrackingEnvironment
	watcher *diskWatcher
	ctrl    *gomock.Controller
	olympus *MockOlympusTask
}

var period = 5 * time.Millisecond
var filesize int64 = 1024

var _ = Suite(&DiskWatcherSuite{})

func (s *DiskWatcherSuite) SetUpSuite(c *C) {
	s.Dir = c.MkDir()
}

func (s *DiskWatcherSuite) SetUpTest(c *C) {
	s.ctrl = gomock.NewController(c)
	s.olympus = NewMockOlympusTask(s.ctrl)
	s.env = &TrackingEnvironment{
		ExperimentDir: s.Dir,
		Leto:          leto.DefaultConfig,
	}
	var err error

	s.env.FreeStartBytes, _, err = fsStat(s.Dir)
	c.Assert(err, IsNil)
	s.env.Start = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	s.watcher = NewDiskWatcher(ctx, s.env, s.olympus).(*diskWatcher)
	s.watcher.period = period
	s.cancel = cancel

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
	s.env.Leto.DiskLimit = s.env.FreeStartBytes + 100*1024
	err := <-Start(s.watcher)
	c.Check(err, ErrorMatches, "unsufficient disk space: available: .* minimum: .*")
}

func computeDiskLimit(startFreeByte int64, targetETA time.Duration) int64 {
	return startFreeByte - int64(float64(filesize)/period.Seconds()*targetETA.Seconds())
}

func (s *DiskWatcherSuite) TestWatcherWarnNearLimit(c *C) {
	ioutil.WriteFile(filepath.Join(s.Dir, c.TestName()), make([]byte, filesize), 0644)
	s.env.Leto.DiskLimit = computeDiskLimit(s.env.FreeStartBytes, 6*time.Hour)

	s.olympus.EXPECT().PushDiskStatus(gomock.Any(), gomock.Any())

	errs := Start(s.watcher)
	time.Sleep(time.Duration(1.2 * float64(period)))
	s.cancel()
	err, ok := <-errs
	c.Check(err, IsNil)
	c.Check(ok, Equals, true)
}
