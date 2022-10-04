package leto

import "time"

//go:generate go run generate_version.go

const MAJOR_FMT_VERSION int = 0
const MINOR_FMT_VERSION int = 5
const LETO_PORT int = 4000
const ARTEMIS_IN_PORT int = 4001
const ARTEMIS_OUT_PORT int = 4002

const ARTEMIS_MIN_VERSION = "v0.4.0"
const NODE_CACHE_TTL = 5 * time.Second
