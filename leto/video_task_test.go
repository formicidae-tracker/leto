package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/formicidae-tracker/leto"
	. "gopkg.in/check.v1"
)

var keepcreated = flag.Bool("keepcreated", false, "Keep created artifact for inspection")

type VideoTaskSuite struct {
	config videoTaskConfig
}

func (s *VideoTaskSuite) Basedir() string {
	return filepath.Dir(s.config.baseFileName.movie)
}

var _ = Suite(&VideoTaskSuite{})

var streamConfiguration = leto.StreamConfiguration{
	Host:            newWithValue(""),
	BitRateKB:       newWithValue(3000),
	BitRateMaxRatio: newWithValue(3.0),
	Quality:         newWithValue("faster"),
	Tune:            newWithValue("stillimage"),
}

func newWithValue[T any](v T) *T {
	res := new(T)
	*res = v
	return res
}

func (s *VideoTaskSuite) SetUpSuite(c *C) {
	cmd := exec.Command("ffmpeg", "-version")
	_, err := cmd.CombinedOutput()
	if err != nil {
		c.Skip(fmt.Sprintf("could not found ffmpeg: %s", err))
	}

	dir, err := os.MkdirTemp("", "leto-video-task-tests")
	c.Assert(err, IsNil)

	s.config, err = newVideoTaskConfig(dir, 8.0, streamConfiguration)
	s.config.resolution = "240x240"

	c.Assert(err, IsNil)
}

func (s *VideoTaskSuite) TearDownSuite(c *C) {
	basedir := s.Basedir()
	if len(basedir) == 0 || basedir == "." || *keepcreated == true {
		return
	}
	c.Assert(os.RemoveAll(basedir), IsNil)
}

func (s *VideoTaskSuite) TestFFMpegClosesWhenStdinCloses(c *C) {

	encodeCmd, err := NewFFMpegCommand(s.config.encodeCommandArgs(), filepath.Join(s.Basedir(), "encode.log"))

	c.Assert(err, IsNil)

	c.Assert(encodeCmd.Start(), IsNil)

	c.Assert(encodeCmd.stdin.Close(), IsNil)

	c.Assert(encodeCmd.Wait(), IsNil)
}

func writeUint64[T int | int64 | uint | uint64](w io.Writer, v T) error {
	var data []byte = nil
	data = binary.LittleEndian.AppendUint64(data, uint64(v))
	_, err := w.Write(data)
	return err
}

func (s *VideoTaskSuite) TestE2E(c *C) {
	dir := filepath.Join(s.Basedir(), "e2e")
	c.Assert(os.MkdirAll(dir, 0755), IsNil)

	v, err := NewVideoManager(dir, 8.0, streamConfiguration)
	v.(*videoTask).config.period = 40 * time.Millisecond
	c.Assert(err, IsNil)

	in, out := io.Pipe()

	errs := StartFunc(func() error { return v.Run(in) })

	for i := 0; i < 80; i++ {
		writeUint64(out, i+42)
		writeUint64(out, 240)
		writeUint64(out, 240)
		frame := make([]byte, 240*240*3)
		v := uint8(255.0 * i / 80.0)
		for i := range frame {
			frame[i] = v
		}
		out.Write(frame)
	}
	out.Close()

	var ok bool
	err, ok = <-errs
	c.Check(err, IsNil)
	c.Check(ok, Equals, true)
}
