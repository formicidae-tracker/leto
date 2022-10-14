package leto

import "time"

//go:generate go run generate_version.go

type Config struct {
	LetoPort            int
	ArtemisIncomingPort int
	HermesBroadcastPort int
	OlympusPort         int
}

var DefaultConfig Config

const MAJOR_FMT_VERSION int = 0
const MINOR_FMT_VERSION int = 5

const ARTEMIS_MIN_VERSION = "v0.4.0"
const NODE_CACHE_TTL = 5 * time.Second

func init() {
	DefaultConfig = Config{
		OlympusPort:         3001,
		LetoPort:            4000,
		ArtemisIncomingPort: 4001,
		HermesBroadcastPort: 4002,
	}
}
