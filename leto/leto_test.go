package main

import (
	"compress/gzip"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"
	"github.com/formicidae-tracker/hermes"
	"github.com/formicidae-tracker/leto"
	"github.com/gabriel-vasile/mimetype"
	. "gopkg.in/check.v1"
)

type LetoSuite struct {
	xdgDataHome string
	tmpdir      string
	l           *Leto
}

var _ = Suite(&LetoSuite{})

func checkFFMpeg() bool {
	return exec.Command("ffmpeg", "-version").Run() == nil
}

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
	if checkFFMpeg() == false {
		ffmpegCommandName = "./mock_main/ffmpeg/ffmpeg"
	}
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

func (s *LetoSuite) TearDownTest(c *C) {
	s.l.Stop()
}

func (s *LetoSuite) TestAlreadyStopped(c *C) {
	c.Check(s.l.Stop(), ErrorMatches, "already stopped")
}

// connects to the boradcaster and wait for n frame to be received
func (s *LetoSuite) waitFrames(n int) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", leto.DefaultConfig.HermesBroadcastPort))

	if err != nil {
		return err
	}

	h := &hermes.Header{}
	_, err = hermes.ReadDelimitedMessage(conn, h)
	if err != nil {
		return err
	}

	for i := 0; i < n; i++ {
		m := &hermes.FrameReadout{}
		_, err = hermes.ReadDelimitedMessage(conn, m)
		if err != nil {
			return err
		}
	}
	return nil
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

	c.Check(s.waitFrames(15), IsNil)

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

func (s *LetoSuite) readAllFrames(experimentDir string) ([]*hermes.FrameReadout, error) {
	hermesPath := filepath.Join(xdg.DataHome, "fort-experiments", experimentDir, "tracking.0000.hermes")
	f, err := os.Open(hermesPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gzip, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gzip.Close()

	h := &hermes.Header{}
	_, err = hermes.ReadDelimitedMessage(gzip, h)
	if err != nil {
		return nil, err
	}
	res := make([]*hermes.FrameReadout, 0)
	for {
		l := &hermes.FileLine{}
		_, err = hermes.ReadDelimitedMessage(gzip, l)
		if err != nil {
			return res, err
		}
		if l.Readout != nil {
			res = append(res, l.Readout)
		}
		if l.Footer != nil {
			return res, nil
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

	c.Check(s.waitFrames(15), IsNil)

	c.Check(s.l.Stop(), IsNil)
	log := s.l.LastExperimentLog()
	c.Assert(log, Not(IsNil))
	c.Check(log.HasError, Equals, false)

	// now check we got at least 15 frame saved in the experiment
	f, err := s.readAllFrames(log.ExperimentDir)
	c.Check(err, IsNil)
	c.Check(len(f) >= 15, Equals, true)

	if ffmpegCommandName != "ffmpeg" {
		// mocked ffmpeg did not produce a video file
		return
	}

	videopath := filepath.Join(xdg.DataHome, "fort-experiments", log.ExperimentDir, "stream.0000.mp4")
	mtype, err := mimetype.DetectFile(videopath)
	c.Check(err, IsNil)
	c.Check(mtype.Is("video/mp4"), Equals, true)
}

func (s *LetoSuite) TestArtemisFailure(c *C) {
	conf := &leto.TrackingConfiguration{
		ExperimentName: "detection-will-fail",
		Detection: leto.TagDetectionConfiguration{
			Family: newWithValue("36HARTag"),
		},
		Camera: leto.CameraConfiguration{
			FPS: newWithValue(100.0),
		},
	}

	c.Assert(s.l.Start(conf), IsNil)
	time.Sleep(20 * time.Millisecond)
	log := s.l.LastExperimentLog()
	c.Assert(log, Not(IsNil))
	c.Check(log.HasError, Equals, true)
}
