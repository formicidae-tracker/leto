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

type Config struct {
	LetoPort            int
	ArtemisIncomingPort int
	HermesBroadcastPort int
	OlympusPort         int
	DevMode             bool
	FramegrabberType    FGType
	DiskLimit           int64
}

var DefaultConfig Config

const ARTEMIS_MIN_VERSION = "v0.4.0"
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
