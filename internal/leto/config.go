package leto

import (
	"os"
	"time"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v2"
)

//go:generate go run generate_version.go $VERSION

type FGType int

const (
	UNKNOWN_FG FGType = iota
	EURESYS_FG
	HYPERION_FG
)

type AVersion string

const (
	ARTEMIS_0_4         AVersion = "v0.4.0"
	ARTEMIS_0_5                  = "v0.5.0"
	ARTEMIS_UNSUPPORTED          = "v0.0.0"
)

type Config struct {
	LetoPort            int    `yaml:"leto_port,omitempty"`
	ArtemisIncomingPort int    `yaml:"artemis_incoming_port,omitempty"`
	HermesBroadcastPort int    `yaml:"hermes_broadcast_port,omitempty"`
	OlympusAddress      string `yaml:"olympus_address,omitempty"`
	DevMode             bool
	FramegrabberType    FGType
	DiskLimit           int64 `yaml:"disk_limit,omitempty"`
	ArtemisVersion      AVersion
}

func localConfigPath() (string, error) {
	return xdg.ConfigFile("io.github.formicidae_tracker/leto/config.yml")
}

func DefaultConfig() Config {
	defaultConfig := Config{
		LetoPort:            4000,
		ArtemisIncomingPort: 4001,
		HermesBroadcastPort: 4002,
		FramegrabberType:    UNKNOWN_FG,
		DiskLimit:           50 * 1024 * 1024, // 50 MiB
	}

	confPath, err := localConfigPath()
	if err != nil {
		return defaultConfig
	}

	data, err := os.ReadFile(confPath)
	if err != nil {
		return defaultConfig
	}

	res := defaultConfig
	if err = yaml.Unmarshal(data, &res); err != nil {
		return defaultConfig
	}
	return res
}

const NODE_CACHE_TTL = 5 * time.Second
