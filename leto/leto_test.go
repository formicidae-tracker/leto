package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"
	"github.com/formicidae-tracker/leto"
	. "gopkg.in/check.v1"
)

type LetoSuite struct {
	xdgDataHome string
	tmpdir      string
	l           *Leto
}

var _ = Suite(&LetoSuite{})

func (s *LetoSuite) SetUpSuite(c *C) {
	dir := c.MkDir()
	datadir := filepath.Join(dir, "data")
	tmpdir := filepath.Join(dir, "tmp")
	os.Mkdir(tmpdir, 0755)

	s.xdgDataHome = os.Getenv("XDG_DATA_HOME")
	s.tmpdir = os.Getenv("TMPDIR")

	os.Setenv("XDG_DATA_HOME", datadir)
	os.Setenv("TMPDIR", tmpdir)
	xdg.Reload()
	c.Check(xdg.DataHome, Equals, datadir)
	c.Check(os.TempDir(), Equals, tmpdir)
	artemisCommandName = "./mock_main/artemis/artemis"
	ffmpegCommandName = "./mock_main/ffmpeg/ffmpeg"
	coaxlinkFirmwareCommandName = "./mock_main/coaxlink-firmware/coaxlink-firmware"
}

func (s *LetoSuite) TearDownSuite(c *C) {
	os.Setenv("XDG_DATA_HOME", s.xdgDataHome)
	os.Setenv("TMPDIR", s.tmpdir)
	xdg.Reload()
	artemisCommandName = "artemis"
	ffmpegCommandName = "ffmpeg"
	coaxlinkFirmwareCommandName = "coaxlink-firmware"
}

func (s *LetoSuite) SetUpTest(c *C) {
	var err error
	s.l, err = NewLeto(leto.DefaultConfig)
	c.Check(err, IsNil)
}

func (s *LetoSuite) TestAlreadyStopped(c *C) {
	c.Check(s.l.Stop(), ErrorMatches, "already stopped")
}

func (s *LetoSuite) TestE2ETestMode(c *C) {
	c.Check(s.l.LastExperimentLog(), IsNil)
	c.Assert(s.l.Start(&leto.TrackingConfiguration{}), IsNil)
	c.Assert(s.l.Start(&leto.TrackingConfiguration{}), ErrorMatches, "already started")
	time.Sleep(20 * time.Millisecond)
	c.Check(s.l.Stop(), IsNil)
	c.Check(s.l.LastExperimentLog(), Equals, nil)
}
