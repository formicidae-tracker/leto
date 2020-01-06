package leto

import (
	"testing"
	"time"

	. "gopkg.in/check.v1"
	yaml "gopkg.in/yaml.v2"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type ConfigurationSuite struct{}

var _ = Suite(&ConfigurationSuite{})

func (s *ConfigurationSuite) TestHasADefaultConfiguration(c *C) {
	config := RecommendedTrackingConfiguration()
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

	to.Detection.Family = new(string)
	*to.Detection.Family = "36ARTag"
	*expected.Detection.Family = "36ARTag"

	to.Detection.Quad.MinBWDiff = new(int)
	*to.Detection.Quad.MinBWDiff = 120
	*expected.Detection.Quad.MinBWDiff = 120

	c.Assert(from.Merge(to), IsNil)
	c.Check(from, DeepEquals, expected)

}

func (s *ConfigurationSuite) TestYAMLParsing(c *C) {

	expected := RecommendedTrackingConfiguration()
	expected.ExperimentName = "test-configuration"
	expected.Highlights = &([]int{1, 42, 16})
	*expected.Detection.Quad.CriticalRadian = 0.17453

	*expected.Camera.StubPath = "foo.png"

	txt := `
experiment: test-configuration
legacy-mode: false
new-ant-roi: 600
new-ant-renew-period: 2h
host-display: false
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
  stub-path: foo.png
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
