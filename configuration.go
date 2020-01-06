package leto

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"reflect"
	"time"

	"gopkg.in/yaml.v2"
)

func MergeConfiguration(from, to interface{}) error {

	if reflect.TypeOf(from) != reflect.TypeOf(to) {
		return fmt.Errorf("Mismatching type %s and %s", reflect.TypeOf(from), reflect.TypeOf(to))
	}

	if reflect.TypeOf(from).Kind() != reflect.Ptr {
		return fmt.Errorf("Configuration can only be merged through pointers")
	}

	if reflect.ValueOf(to).IsNil() == true {
		return fmt.Errorf("Cannot merge from nil configuration")
	}

	if reflect.ValueOf(to).IsNil() == true {
		return nil
	}

	tFrom := reflect.TypeOf(from).Elem()
	vFrom := reflect.ValueOf(from).Elem()
	vTo := reflect.ValueOf(to).Elem()
	for i := 0; i < tFrom.NumField(); i++ {
		tField := tFrom.Field(i)
		if tField.Type.Kind() != reflect.Ptr {
			continue
		}

		fromField := vFrom.FieldByName(tField.Name)
		toField := vTo.FieldByName(tField.Name)

		if fromField.Elem().CanSet() == false {
			continue
		}

		if toField.IsNil() {
			continue
		}

		fromField.Elem().Set(toField.Elem())
	}

	return nil
}

type QuadDetectionConfiguration struct {
	Decimate        *float64 `long:"at-quad-decimate" description:"Decimate quads (recommended:1.0)" yaml:"decimate"`
	Sigma           *float64 `long:"at-quad-sigma" description:"Blur before finding quads (recommended:0.0)" yaml:"sigma"`
	RefineEdges     *bool    `long:"at-refine-edges" description:"Refine edges once a quad is found (not recommended)" yaml:"refine-edges"`
	MinClusterPixel *int     `long:"at-quad-min-cluster" description:"Minimum numbe rof pixel to consider a quad (recommended:25)"  yaml:"min-cluster-pixel"`
	MaxNMaxima      *int     `long:"at-quad-max-n-maxima" description:"Maximum number of corner to consider when fitting a quad (recommended:10)" yaml:"max-n-maxima"`
	CriticalRadian  *float64 `long:"at-quad-critical-radian" description:"Minimal angle for a quad corner (recommended:10°)" yaml:"critical-angle-radian"`
	MaxLineMSE      *float64 `long:"at-quad-max-line-mse" description:"Maximal MSE for a line fit (recommended:10.0)" yaml:"max-line-mean-square-error"`
	MinBWDiff       *int     `long:"at-quad-min-bw-diff" description:"Minimum local threshold to consider a black/white border (recommended:50)" yaml:"min-black-white-diff"`
	Deglitch        *bool    `long:"at-quad-deglitch" description:"Enables quad deglitching euristics (not recommended)" yaml:"deglitch"`
}

func RecommendedQuadDetectionConfiguration() QuadDetectionConfiguration {
	res := QuadDetectionConfiguration{
		Decimate:        new(float64),
		Sigma:           new(float64),
		RefineEdges:     new(bool),
		MinClusterPixel: new(int),
		MaxNMaxima:      new(int),
		CriticalRadian:  new(float64),
		MaxLineMSE:      new(float64),
		MinBWDiff:       new(int),
		Deglitch:        new(bool),
	}

	*res.Decimate = 1.0
	*res.Sigma = 0.0
	*res.RefineEdges = false
	*res.MinClusterPixel = 25
	*res.MaxNMaxima = 10
	*res.CriticalRadian = 10.0 * math.Pi / 180.0
	*res.MaxLineMSE = 10.0
	*res.MinBWDiff = 50
	*res.Deglitch = false
	return res
}

func (from *QuadDetectionConfiguration) Merge(to *QuadDetectionConfiguration) error {
	return MergeConfiguration(from, to)
}

type TagDetectionConfiguration struct {
	Family *string                    `long:"at-family" description:"tag family to use. Usual values are 36h11, 36h10, 36ARTag, Standard41H12" yaml:"family"`
	Quad   QuadDetectionConfiguration `yaml:"quad"`
}

func RecommendedDetectionConfig() TagDetectionConfiguration {
	res := TagDetectionConfiguration{
		Family: new(string),
		Quad:   RecommendedQuadDetectionConfiguration(),
	}
	*res.Family = "36h11"
	return res
}

func (from *TagDetectionConfiguration) Merge(to *TagDetectionConfiguration) error {
	if err := from.Quad.Merge(&to.Quad); err != nil {
		return err
	}
	return MergeConfiguration(from, to)
}

type CameraConfiguration struct {
	StrobeDelay    *time.Duration `long:"strobe-delay" description:"delay of the strobe signal (recommended:0us)" yaml:"strobe-delay"`
	StrobeDuration *time.Duration `long:"strobe-duration" description:"duration of the strobe signal (recommended:1500us)" yaml:"strobe-duration"`
	FPS            *float64       `long:"f" description:"FPS to use for the experiment (recommended:8.0)" yaml:"fps"`
	StubPath       *string        `yaml:"stub-path"`
}

func RecommendedCameraConfiguration() CameraConfiguration {
	res := CameraConfiguration{
		StrobeDelay:    new(time.Duration),
		StrobeDuration: new(time.Duration),
		FPS:            new(float64),
		StubPath:       new(string),
	}
	*res.StrobeDelay = 0
	*res.StrobeDuration = 1500 * time.Microsecond
	*res.FPS = 8.0
	*res.StubPath = ""
	return res
}

func (from *CameraConfiguration) Merge(to *CameraConfiguration) error {
	return MergeConfiguration(from, to)
}

type StreamConfiguration struct {
	Host            *string  `long:"stream-host" description:"host to stream to " yaml:"host"`
	BitRateKB       *int     `long:"stream-bitrate" description:"Constant encoding bitrate to use in kb/s (recommended:2000)" yaml:"bitrate"`
	BitRateMaxRatio *float64 `long:"stream-bitrate-max-ratio" description:"Constraint on the max ratio for bitrate encoding" yaml:"bitrate-max-ratio"`
	Quality         *string  `long:"stream-quality" description:"libx264 quality preset (recommended:fast)" yaml:"quality"`
	Tune            *string  `long:"stream-tune" description:"libx264 quality tuning (recommended:film)" yaml:"tuning"`
}

func RecommendedStreamConfiguration() StreamConfiguration {
	res := StreamConfiguration{
		Host:            new(string),
		BitRateKB:       new(int),
		BitRateMaxRatio: new(float64),
		Quality:         new(string),
		Tune:            new(string),
	}
	*res.Host = ""
	*res.BitRateKB = 2000
	*res.BitRateMaxRatio = 1.5
	*res.Quality = "fast"
	*res.Tune = "film"
	return res
}

func (from *StreamConfiguration) Merge(to *StreamConfiguration) error {
	return MergeConfiguration(from, to)
}

type TrackingConfiguration struct {
	ExperimentName      string                    `short:"e" long:"experiment" description:"Name of the experiment to run" yaml:"experiment"`
	DisplayOnHost       *bool                     `long:"display-on-host" description:"Opens a window and display on host the data" yaml:"host-display"`
	LegacyMode          *bool                     `long:"legacy-mode" description:"Produces a legacy mode data output" yaml:"legacy-mode"`
	NewAntOutputROISize *int                      `long:"new-ant-size" description:"Size of the image when a new ant is found (recommended:600)" yaml:"new-ant-roi"`
	NewAntRenewPeriod   *time.Duration            `long:"new-ant-renew-period" description:"Period to renew ant snapshot (recommended:2h)" yaml:"new-ant-renew-period"`
	Stream              StreamConfiguration       `yaml:"stream"`
	Camera              CameraConfiguration       `yaml:"camera"`
	Detection           TagDetectionConfiguration `yaml:"apriltag"`
	Highlights          *[]int                    `yaml:"highlights"`
}

func RecommendedTrackingConfiguration() TrackingConfiguration {
	res := TrackingConfiguration{
		NewAntOutputROISize: new(int),
		NewAntRenewPeriod:   new(time.Duration),
		LegacyMode:          new(bool),
		DisplayOnHost:       new(bool),
		Stream:              RecommendedStreamConfiguration(),
		Camera:              RecommendedCameraConfiguration(),
		Detection:           RecommendedDetectionConfig(),
		Highlights:          &([]int{}),
	}
	*res.NewAntOutputROISize = 600
	*res.NewAntRenewPeriod = 2 * time.Hour
	*res.LegacyMode = false
	*res.DisplayOnHost = false
	return res
}

func (from *TrackingConfiguration) Merge(to *TrackingConfiguration) error {
	if err := from.Stream.Merge(&to.Stream); err != nil {
		return err
	}
	if err := from.Camera.Merge(&to.Camera); err != nil {
		return err
	}
	if err := from.Detection.Merge(&to.Detection); err != nil {
		return err
	}

	if len(to.ExperimentName) > 0 {
		from.ExperimentName = to.ExperimentName
	}
	return MergeConfiguration(from, to)
}

func CheckNoNilField(v reflect.Value) error {
	if v.Type().Kind() != reflect.Struct {
		return fmt.Errorf("Field is not a struct")
	}
	for i := 0; i < v.Type().NumField(); i++ {
		f := v.Field(i)
		if f.Type().Kind() == reflect.Struct {
			if err := CheckNoNilField(f); err != nil {
				return err
			}
		}
		if f.Type().Kind() == reflect.Ptr {
			if f.IsNil() {
				return fmt.Errorf("field '%s' is nil", v.Type().Field(i).Name)
			}
		}
	}
	return nil
}

func (c *TrackingConfiguration) CheckAllFieldAreSet() error {
	return CheckNoNilField(reflect.ValueOf(*c))
}

func ReadConfiguration(filename string) (*TrackingConfiguration, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("Could not open '%s': %s", filename, err)
	}
	defer f.Close()
	txt, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("Could not read '%s': %s", filename, err)
	}

	res := &TrackingConfiguration{}
	err = yaml.Unmarshal(txt, res)

	return res, err
}

func (c *TrackingConfiguration) WriteConfiguration(filename string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("Could not encode configuration: %s", err)
	}

	err = ioutil.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("Could not write '%s': %s", filename, err)
	}

	return nil
}
