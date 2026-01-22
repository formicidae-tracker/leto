package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/formicidae-tracker/leto/internal/leto"
	. "gopkg.in/check.v1"
)

type allowedArguments struct {
	allowed map[string]bool
}

func newAllowedArguments(filepath string) (*allowedArguments, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("could not read '%s': %w", filepath, err)
	}
	res := &allowedArguments{allowed: make(map[string]bool)}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-") == false {
			continue
		}
		options := strings.TrimSpace(strings.Split(line, ":")[0])
		if strings.Contains(options, "/") {
			for _, opt := range strings.Split(options, "/") {
				res.allowed[strings.TrimSpace(opt)] = true
			}
		} else {
			for _, opt := range strings.Split(options, ",") {
				res.allowed[strings.TrimSpace(opt)] = true
			}
		}
	}
	return res, nil
}

type TrackingEnvironmentSuite struct {
	env *TrackingEnvironment

	versions map[leto.AVersion]*allowedArguments
}

var _ = Suite(&TrackingEnvironmentSuite{})

func (s *TrackingEnvironmentSuite) TestArtemisArguments(c *C) {
	testdata := []struct {
		Version leto.AVersion
	}{
		{Version: leto.ARTEMIS_0_5},
		{Version: leto.ARTEMIS_0_4},
	}

	for _, d := range testdata {
		s.env.Leto.ArtemisVersion = d.Version
		args := s.env.TrackingCommandArgs()
		for _, a := range args {
			a = strings.Split(a, "=")[0]
			if len(a) == 0 || a[0] != '-' {
				continue
			}
			c.Check(s.versions[d.Version].allowed[a], Equals, true, Commentf("Artemis version %s does not support options '%s'", d.Version, a))
		}
	}

}

func (s *TrackingEnvironmentSuite) SetUpSuite(c *C) {
	var err error

	s.env, err = NewExperimentConfiguration(context.Background(), leto.DefaultConfig, defaultNodeConfiguration, leto.LoadDefaultConfig())
	c.Assert(err, IsNil)
	s.versions = map[leto.AVersion]*allowedArguments{}
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() || strings.HasPrefix(d.Name(), "artemis_help_v") == false || strings.HasSuffix(d.Name(), ".txt") == false {
			return nil
		}
		vStr := strings.TrimSuffix(strings.TrimPrefix(d.Name(), "artemis_help_"), ".txt")
		var v leto.AVersion = leto.ARTEMIS_UNSUPPORTED

		if strings.HasPrefix(vStr, strings.TrimSuffix(string(leto.ARTEMIS_0_4), ".0")) {
			v = leto.ARTEMIS_0_4
		} else if strings.HasPrefix(vStr, strings.TrimSuffix(string(leto.ARTEMIS_0_5), ".0")) {
			v = leto.ARTEMIS_0_5
		} else {
			return fmt.Errorf("Unsupported version '%s'", vStr)
		}
		s.versions[v], err = newAllowedArguments(path)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, IsNil)

	c.Assert(os.Mkdir(filepath.Join(os.TempDir(), "fort-tests"), 0755), IsNil)

}

func (s *TrackingEnvironmentSuite) TearDownSuite(c *C) {
	c.Assert(os.RemoveAll(filepath.Join(os.TempDir(), "fort-tests")), IsNil)
}
