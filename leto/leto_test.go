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

func (s *LetoSuite) TestTestMode(c *C) {
	c.Check(s.l.LastExperimentLog(), IsNil)
	conf := &leto.TrackingConfiguration{
		Camera: leto.CameraConfiguration{
			FPS: newWithValue(100.0),
		},
	}

	c.Assert(s.l.Start(conf), IsNil)
	c.Assert(s.l.Start(&leto.TrackingConfiguration{}), ErrorMatches, "already started")
	time.Sleep(500 * time.Millisecond)
	c.Check(s.l.Stop(), IsNil)
	log := s.l.LastExperimentLog()
	c.Assert(log, Not(IsNil))
	c.Check(log.HasError, Equals, false)

	entries, err := os.ReadDir(filepath.Join(os.TempDir(), "fort-tests"))
	c.Check(err, IsNil)
	if c.Check(entries, HasLen, 0) == false {
		for _, e := range entries {
			c.Errorf("unexpected file %s", e.Name())
		}
	}
}

func (s *LetoSuite) TestE2E(c *C) {
	conf := &leto.TrackingConfiguration{
		ExperimentName: "test-e2e",
		Camera: leto.CameraConfiguration{
			FPS: newWithValue(100.0),
		},
	}

	c.Check(s.l.LastExperimentLog(), IsNil)

	c.Assert(s.l.Start(conf), IsNil)
	time.Sleep(500 * time.Millisecond)
	c.Check(s.l.Stop(), IsNil)
	log := s.l.LastExperimentLog()
	c.Assert(log, Not(IsNil))
	c.Check(log.HasError, Equals, false)

}
