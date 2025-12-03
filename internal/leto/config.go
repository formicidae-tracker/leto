package leto

import (
	"time"
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
	LetoPort            int
	ArtemisIncomingPort int
	HermesBroadcastPort int
	OlympusPort         int
	DevMode             bool
	FramegrabberType    FGType
	DiskLimit           int64
	ArtemisVersion      AVersion
}

var DefaultConfig Config

const NODE_CACHE_TTL = 5 * time.Second

func init() {
	DefaultConfig = Config{
		OlympusPort:         3001,
		LetoPort:            4000,
		ArtemisIncomingPort: 4001,
		HermesBroadcastPort: 4002,
		FramegrabberType:    UNKNOWN_FG,
		DiskLimit:           50 * 1024 * 1024, // 50 MiB
	}
}
