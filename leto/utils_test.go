package main

import (
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"
)

type UtilsSuite struct {
	tmpDir string
}

var _ = Suite(&UtilsSuite{})

func (s *UtilsSuite) SetUpSuite(c *C) {
	s.tmpDir = c.MkDir()
}

func (s *UtilsSuite) TestNameSuffix(c *C) {
	testdata := []struct {
		Base     string
		i        int
		Expected string
	}{
		{"out.txt", 0, "out.0000.txt"},
		{"out.0000.txt", 0, "out.0000.txt"},
		{"bar.foo.2.txt", 3, "bar.foo.0003.txt"},
		{"../some/path/out.0042.txt", 2, "../some/path/out.0002.txt"},
		{"../some/path/out.0042.txt", 0, "../some/path/out.0000.txt"},
	}

	for _, d := range testdata {
		c.Check(FilenameWithSuffix(d.Base, d.i), Equals, d.Expected)
	}
}

func (s *UtilsSuite) TestCreateWithoutOverwrite(c *C) {
	files := []string{"out.0000.txt", "out.0001.txt", "out.0003.txt"}

	for _, f := range files {
		ff, err := os.Create(filepath.Join(s.tmpDir, f))
		c.Assert(err, IsNil)
		defer ff.Close()
	}

	fname, i, err := FilenameWithoutOverwrite(filepath.Join(s.tmpDir, files[0]))
	c.Check(err, IsNil)
	c.Check(i, Equals, 2)
	ff, err := os.Create(fname)
	c.Check(err, IsNil)
	defer ff.Close()
	c.Assert(fname, Equals, filepath.Join(s.tmpDir, "out.0002.txt"))
	_, err = os.Stat(fname)
	c.Check(err, IsNil)

}

func (s *UtilsSuite) TestDoNotFailIfParentDoNotExist(c *C) {
	fname, i, err := FilenameWithoutOverwrite(filepath.Join(s.tmpDir, "do-not-exists", "out.txt"))
	c.Check(err, IsNil)
	c.Check(i, Equals, 0)
	c.Check(fname, Equals, filepath.Join(s.tmpDir, "do-not-exists", "out.0000.txt"))
}
