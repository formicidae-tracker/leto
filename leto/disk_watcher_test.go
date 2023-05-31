package main

import (
	"context"
	"path/filepath"
	"time"

	"github.com/formicidae-tracker/leto"
	. "gopkg.in/check.v1"
)

type DiskWatcherSuite struct {
	Dir     string
	cancel  context.CancelFunc
	watcher DiskWatcher
	errs    <-chan error
}

var _ = Suite(&DiskWatcherSuite{})

func (s *DiskWatcherSuite) SetUpSuite(c *C) {
	s.Dir = c.MkDir()
}

func (s *DiskWatcherSuite) SetUpTest(c *C) {
	env := &TrackingEnvironment{
		ExperimentDir: s.Dir,
		Leto:          leto.DefaultConfig,
	}
	var err error

	env.FreeStartBytes, _, err = fsStat(s.Dir)
	c.Assert(err, IsNil)
	env.Leto.DiskLimit = env.FreeStartBytes - 10*1024
	env.Start = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	s.watcher = NewDiskWatcher(ctx, env, nil)
	s.watcher.(*diskWatcher).period = 5 * time.Millisecond
	s.cancel = cancel
	s.errs = Start(s.watcher)
}

func (s *DiskWatcherSuite) TearDownTest(c *C) {
	s.cancel()
	err := <-s.errs
	c.Check(err, IsNil)
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

func (s *DiskWatcherSuite) TestE2E(c *C) {

}
