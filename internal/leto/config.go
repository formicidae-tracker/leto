package leto

import "time"

//go:generate go run generate_version.go

type Config struct {
	LetoPort            int
	ArtemisIncomingPort int
	HermesBroadcastPort int
	OlympusPort         int
	DevMode             bool
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
		DiskLimit:           50 * 1024 * 1024, // 50 MiB
	}
}
