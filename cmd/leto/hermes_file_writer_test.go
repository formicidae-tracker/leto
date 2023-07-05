package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/formicidae-tracker/hermes"
	. "gopkg.in/check.v1"
)

type FileWriterSuite struct {
	basedir string
	writer  HermesFileWriter
	err     <-chan error
}

var _ = Suite(&FileWriterSuite{})

func (s *FileWriterSuite) SetUpSuite(c *C) {
	var err error
	s.basedir, err = os.MkdirTemp("", "leto-file-tests")
	c.Assert(err, IsNil)

}

func (s *FileWriterSuite) TearDownSuite(c *C) {
	c.Assert(os.RemoveAll(s.basedir), IsNil)
}

func (s *FileWriterSuite) SetUpTest(c *C) {
	var err error
	s.writer, err = NewFrameReadoutWriter(context.Background(), filepath.Join(s.basedir, c.TestName()+".hermes"))
	c.Assert(err, IsNil)
	s.writer.(*hermesFileWriter).period = 5 * time.Millisecond
	s.err = Start(s.writer)
}

func (s *FileWriterSuite) TestNothingHappenWouldClose(c *C) {
	close(s.writer.Incoming())

	err, ok := <-s.err
	c.Check(err, IsNil)
	c.Check(ok, Equals, true)

	err, ok = <-s.err
	c.Check(err, IsNil)
	c.Check(ok, Equals, false)

	// we check that no file were produced
	entries, err := os.ReadDir(s.basedir)
	c.Assert(err, IsNil)
	for _, e := range entries {
		if strings.Contains(e.Name(), c.TestName()) {
			c.Errorf("%s contains %s, which should not have been created", s.basedir, e.Name())
		}
	}
}

func (s *FileWriterSuite) TestE2E(c *C) {
	for i := 0; i < 100; i++ {
		s.writer.Incoming() <- &hermes.FrameReadout{FrameID: int64(i + 42)}
		time.Sleep(100 * time.Microsecond)
	}
	close(s.writer.Incoming())
	err, ok := <-s.err
	c.Check(err, IsNil)
	c.Check(ok, Equals, true)

	// no uncompressed file remains
	entries, err := os.ReadDir(s.basedir)
	c.Assert(err, IsNil)
	for _, e := range entries {
		if strings.Contains(e.Name(), c.TestName()) && strings.HasPrefix(e.Name(), "uncompressed-") {
			c.Errorf("%s contains %s, which should not remain once terminated without errors", s.basedir, e.Name())
		}
	}

}
