package leto

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	. "gopkg.in/check.v1"
	yaml "gopkg.in/yaml.v2"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type ConfigurationSuite struct {
	testDir string
}

func (s *ConfigurationSuite) SetUpSuite(c *C) {
	var err error
	s.testDir, err = ioutil.TempDir("", "leto-test")
	c.Assert(err, IsNil)
}

func (s *ConfigurationSuite) TearDownSuite(c *C) {
	err := os.RemoveAll(s.testDir)
	c.Assert(err, IsNil)
}

var _ = Suite(&ConfigurationSuite{})

func (s *ConfigurationSuite) TestHasADefaultConfiguration(c *C) {
	config := RecommendedTrackingConfiguration()
	config.Loads = new(LoadBalancing)
	c.Check(config.CheckAllFieldAreSet(), IsNil)
}

func (s *ConfigurationSuite) TestCanBeMerged(c *C) {
	from := RecommendedTrackingConfiguration()

	to := &TrackingConfiguration{}

	expected := RecommendedTrackingConfiguration()

	to.ExperimentName = "foobar"
	expected.ExperimentName = "foobar"

	to.NewAntRenewPeriod = new(time.Duration)
	*to.NewAntRenewPeriod = 10 * time.Minute
	*expected.NewAntRenewPeriod = 10 * time.Minute

	to.Stream.Host = new(string)
	*to.Stream.Host = "google.com"
	*expected.Stream.Host = "google.com"

	to.Camera.FPS = new(float64)
	*to.Camera.FPS = 10.0
	*expected.Camera.FPS = 10.0
	to.Camera.StubPaths = &[]string{"foo.png", "bar.png"}
	*expected.Camera.StubPaths = []string{"foo.png", "bar.png"}

	to.Detection.Family = new(string)
	*to.Detection.Family = "36ARTag"
	*expected.Detection.Family = "36ARTag"

	to.Detection.Quad.MinBWDiff = new(int)
	*to.Detection.Quad.MinBWDiff = 120
	*expected.Detection.Quad.MinBWDiff = 120

	to.Loads = &LoadBalancing{"single-node", map[string]string{"localhost": "single-node"}, map[int]string{0: "single-node"}, 640, 480}
	expected.Loads = &LoadBalancing{"single-node", map[string]string{"localhost": "single-node"}, map[int]string{0: "single-node"}, 640, 480}

	c.Assert(from.Merge(to), IsNil)
	c.Check(from, DeepEquals, expected)

}

func (s *ConfigurationSuite) TestYAMLParsing(c *C) {

	expected := RecommendedTrackingConfiguration()
	expected.ExperimentName = "test-configuration"
	expected.Highlights = &([]int{1, 42, 16})
	*expected.Detection.Quad.CriticalRadian = 0.17453

	*expected.Camera.StubPaths = []string{"foo.png", "bar.png"}

	*expected.Detection.Family = "36h11"
	txt := `
experiment: test-configuration
legacy-mode: false
new-ant-roi: 600
image-renew-period: 2h
threads: 0
stream:
  host: ""
  bitrate: 2000
  bitrate-max-ratio: 1.5
  quality: fast
  tuning: film
camera:
  fps: 8.0
  strobe-delay: 0us
  strobe-duration: 1500us
  stub-image-paths:
    - foo.png
    - bar.png
apriltag:
  family: 36h11
  quad:
    decimate: 1.0
    sigma: 0.0
    refine-edges: false
    min-cluster-pixel: 25
    max-n-maxima: 10
    critical-angle-radian: 0.17453
    max-line-mean-square-error: 10
    min-black-white-diff: 50
    deglitch: false
highlights:
  - 1
  - 42
  - 16
`

	result := &TrackingConfiguration{}
	c.Assert(yaml.Unmarshal([]byte(txt), result), IsNil)

	c.Assert(result, DeepEquals, &expected)

}

func (s *ConfigurationSuite) TestMergingFailCheck(c *C) {
	var nilValue *CameraConfiguration = nil

	type FooConfig struct {
		data string
	}

	testdata := []struct {
		From, To interface{}
		Expected string
	}{
		{
			&CameraConfiguration{},
			&StreamConfiguration{},
			`Mismatching type .* and .*`,
		},
		{
			CameraConfiguration{},
			CameraConfiguration{},
			`Configuration can only be merged through pointers`,
		},
		{
			nilValue,
			&CameraConfiguration{},
			`Cannot merge from nil configuration`,
		},
		{
			&CameraConfiguration{},
			nilValue,
			``,
		},
		{
			&FooConfig{},
			&FooConfig{},
			"",
		},
	}

	for _, d := range testdata {
		err := MergeConfiguration(d.From, d.To)

		if len(d.Expected) == 0 {
			c.Check(err, IsNil)
			continue
		}

		if c.Check(err, Not(IsNil)) == false {
			continue
		}
		c.Check(err, ErrorMatches, d.Expected)
	}

}

func (s *ConfigurationSuite) TestConfigurationIO(c *C) {
	goodConfigPath := filepath.Join(s.testDir, "good-config.yml")
	badConfigPath := filepath.Join(s.testDir, "bad-config.yml")
	unexistConfigPath := filepath.Join(s.testDir, "does-not-exist-config.yml")

	err := ioutil.WriteFile(goodConfigPath, nil, 0644)
	c.Assert(err, IsNil)

	err = ioutil.WriteFile(badConfigPath,
		[]byte(`:foobar
`), 0644)
	c.Assert(err, IsNil)

	config, err := ReadConfiguration(goodConfigPath)
	c.Check(err, IsNil)
	_, err = ReadConfiguration(unexistConfigPath)
	c.Check(err, ErrorMatches, `Could not read '.*': open .*`)
	_, err = ReadConfiguration(badConfigPath)
	c.Check(err, ErrorMatches, `yaml: unmarshal errors:
  line 1:.*`)

	err = config.WriteConfiguration(unexistConfigPath)
	c.Check(err, IsNil)

	err = config.WriteConfiguration(filepath.Join(s.testDir, "foo/bar.yml"))
	c.Check(err, ErrorMatches, `Could not write '.*': open .*`)

}

func (s *ConfigurationSuite) TestNoNilFieldCheck(c *C) {
	testdata := []struct {
		Data     interface{}
		Expected string
	}{
		{
			1.0,
			`Field is not a struct`,
		},
		{
			struct{ A struct{ A *int } }{A: struct{ A *int }{nil}},
			`field 'A' is nil`,
		},
	}

	for _, d := range testdata {
		err := CheckNoNilField(reflect.ValueOf(d.Data))
		if len(d.Expected) == 0 {
			c.Check(err, IsNil)
			continue
		}
		c.Check(err, ErrorMatches, d.Expected)
	}

}
