package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"
	"github.com/formicidae-tracker/hermes/src/go/hermes"
	"github.com/formicidae-tracker/leto/internal/leto"
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

func (s *LetoSuite) setArtemis(version leto.AVersion) {
	switch version {
	case leto.ARTEMIS_UNSUPPORTED:
		os.Setenv("MOCK_ARTEMIS_VERSION", "")
		artemisCommandName = "artemis"
	case leto.ARTEMIS_0_4:
		os.Setenv("MOCK_ARTEMIS_VERSION", "0.4")
		artemisCommandName = "./mock_main/artemis/artemis"
	case leto.ARTEMIS_0_5:
		os.Setenv("MOCK_ARTEMIS_VERSION", "0.5")
		artemisCommandName = "./mock_main/artemis/artemis"
	}
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
	if checkFFMpeg() == false {
		ffmpegCommandName = "./mock_main/ffmpeg/ffmpeg"
	}
	coaxlinkFirmwareCommandName = "./mock_main/coaxlink-firmware/coaxlink-firmware"
}

func (s *LetoSuite) TearDownSuite(c *C) {
	os.Setenv("XDG_DATA_HOME", s.xdgDataHome)
	os.Setenv("TMPDIR", s.tmpdir)
	xdg.Reload()
	s.setArtemis(leto.ARTEMIS_UNSUPPORTED)
	ffmpegCommandName = "ffmpeg"
	coaxlinkFirmwareCommandName = "coaxlink-firmware"
}

func (s *LetoSuite) setUpTest(c *C, version leto.AVersion) bool {
	s.setArtemis(version)
	var err error
	s.l, err = NewLeto(leto.DefaultConfig)
	return c.Check(err, IsNil) && c.Check(s.l, Not(IsNil))
}

func (s *LetoSuite) TearDownTest(c *C) {
	if s.l == nil {
		return
	}
	s.l.Stop(context.Background())
	s.l = nil
}

func (s *LetoSuite) TestAlreadyStopped(c *C) {
	if s.setUpTest(c, leto.ARTEMIS_0_5) == false {
		return
	}
	c.Check(s.l.Stop(context.Background()), ErrorMatches, "already stopped")
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
	if s.setUpTest(c, leto.ARTEMIS_0_5) == false {
		return
	}
	c.Check(s.l.LastExperimentLog(), IsNil)
	conf := &leto.TrackingConfiguration{
		Camera: leto.CameraConfiguration{
			FPS: newWithValue(100.0),
		},
	}

	c.Assert(s.l.Start(context.Background(), conf), IsNil)
	c.Assert(s.l.Start(context.Background(), &leto.TrackingConfiguration{}), ErrorMatches, "already started")

	c.Check(s.waitFrames(15), IsNil)

	c.Check(s.l.Stop(context.Background()), IsNil)
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

func (s *LetoSuite) TestE2E_artemis_0_4(c *C) {
	if s.setUpTest(c, leto.ARTEMIS_0_4) == false {
		return
	}

	conf := &leto.TrackingConfiguration{
		ExperimentName: "test-e2e",
		Camera: leto.CameraConfiguration{
			FPS: newWithValue(100.0),
		},
	}

	c.Check(s.l.LastExperimentLog(), IsNil)

	c.Assert(s.l.Start(context.Background(), conf), IsNil)

	c.Check(s.waitFrames(15), IsNil)

	c.Check(s.l.Stop(context.Background()), IsNil)
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

func (s *LetoSuite) TestE2E_artemis_0_5(c *C) {
	if s.setUpTest(c, leto.ARTEMIS_0_5) == false {
		return
	}

	conf := &leto.TrackingConfiguration{
		ExperimentName: "test-e2e",
		Camera: leto.CameraConfiguration{
			FPS: newWithValue(100.0),
		},
	}

	c.Check(s.l.LastExperimentLog(), IsNil)

	c.Assert(s.l.Start(context.Background(), conf), IsNil)

	c.Check(s.waitFrames(15), IsNil)

	c.Check(s.l.Stop(context.Background()), IsNil)
	log := s.l.LastExperimentLog()
	c.Assert(log, Not(IsNil))
	c.Check(log.HasError, Equals, false)

	// now check we got at least 15 frame saved in the experiment
	f, err := s.readAllFrames(log.ExperimentDir)
	c.Check(err, IsNil)
	c.Check(len(f) >= 15, Equals, true)

	// we are not testing the video file generation, it is now the responsability of artemis.
}

func (s *LetoSuite) TestArtemisFailure(c *C) {
	if s.setUpTest(c, leto.ARTEMIS_0_5) == false {
		return
	}

	conf := &leto.TrackingConfiguration{
		ExperimentName: "detection-will-fail",
		Detection: leto.TagDetectionConfiguration{
			Family: newWithValue("36HARTag"),
		},
		Camera: leto.CameraConfiguration{
			FPS: newWithValue(100.0),
		},
	}

	c.Assert(s.l.Start(context.Background(), conf), IsNil)
	time.Sleep(20 * time.Millisecond)
	log := s.l.LastExperimentLog()
	c.Assert(log, Not(IsNil))
	c.Check(log.HasError, Equals, true)
}

func (s *LetoSuite) TestCanCheckVersion(c *C) {
	if s.setUpTest(c, leto.ARTEMIS_0_5) == false {
		return
	}

	testdata := []struct {
		Version         string
		ExpectedVersion leto.AVersion
		Expected        string
	}{
		{
			"v1.2.3",
			leto.ARTEMIS_UNSUPPORTED,
			`unsupported artemis version 'v1.2.3'`,
		},
		{
			"v0.4.3",
			leto.ARTEMIS_0_4,
			``,
		},
		{
			"v0.5.0-rc1+123-gfffffff",
			leto.ARTEMIS_0_5,
			``,
		},
		{
			"v1.2.3.4",
			leto.ARTEMIS_UNSUPPORTED,
			`could not parse version 'v1.2.3.4': Invalid character\(s\) found in patch number ".*"`,
		},
		{
			"v0.3.3",
			leto.ARTEMIS_UNSUPPORTED,
			`unsupported artemis version 'v0.3.3'`,
		},
	}

	for _, d := range testdata {
		res, err := getArtemisVersion(d.Version)
		if len(d.Expected) == 0 {
			c.Check(err, IsNil)
			continue
		}
		c.Check(res, Equals, d.ExpectedVersion)
		c.Check(err, ErrorMatches, d.Expected)
	}
}
