package profile

import (
	"fmt"
	"time"
)

const (
	TransportH1KeepAlive   = "h1_keepalive"
	TransportH1NoKeepAlive = "h1_no_keepalive"
)

type ServerLimits struct {
	GOMAXPROCS               string
	GOGC                     string
	GOMEMLIMIT               string
	ProcessMemoryLimitBytes  uint64
	CPUAffinityCores         int
	SQLMaxOpenConns          string
	SQLMaxIdleConns          string
	RedisPoolSize            string
	RedisIdleTimeoutSeconds  string
	RelayMaxIdleConns        string
	RelayMaxIdleConnsPerHost string
}

type Transport struct {
	Mode                string
	MaxConnsPerHost     int
	MaxIdleConns        int
	MaxIdleConnsPerHost int
}

type Relay struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
}

type Profile struct {
	Name             string
	Points           []int
	RequestsPerPoint int
	RampStep         int
	RampInterval     time.Duration
	Duration         time.Duration
	Timeout          time.Duration
	Transport        Transport
	Relay            Relay
	ServerLimits     ServerLimits
}

func Benchmark() Profile {
	return Profile{
		Name:             "benchmark",
		Points:           []int{250, 500, 750, 1000, 1250, 1500, 1750, 2000},
		RequestsPerPoint: 3000,
		RampStep:         125,
		RampInterval:     200 * time.Millisecond,
		Duration:         75 * time.Second,
		Timeout:          120 * time.Second,
		Transport: Transport{
			Mode:                TransportH1KeepAlive,
			MaxConnsPerHost:     2000,
			MaxIdleConns:        2000,
			MaxIdleConnsPerHost: 2000,
		},
		Relay: Relay{
			MaxIdleConns:        1024,
			MaxIdleConnsPerHost: 1024,
		},
		ServerLimits: benchmarkServerLimits(),
	}
}

func Smoke() Profile {
	return Profile{
		Name:             "smoke",
		Points:           []int{2},
		RequestsPerPoint: 10,
		RampStep:         1,
		RampInterval:     200 * time.Millisecond,
		Duration:         5 * time.Second,
		Timeout:          30 * time.Second,
		Transport: Transport{
			Mode:                TransportH1KeepAlive,
			MaxConnsPerHost:     4,
			MaxIdleConns:        4,
			MaxIdleConnsPerHost: 4,
		},
		Relay: Relay{
			MaxIdleConns:        64,
			MaxIdleConnsPerHost: 16,
		},
		ServerLimits: benchmarkServerLimits(),
	}
}

func ProfileByName(name string) (Profile, error) {
	switch name {
	case "benchmark":
		return Benchmark(), nil
	case "smoke":
		return Smoke(), nil
	case "h2c_diagnostic":
		return Profile{}, fmt.Errorf("h2c diagnostic profile is not implemented in this phase")
	default:
		return Profile{}, fmt.Errorf("unknown loadtest profile %q", name)
	}
}

func benchmarkServerLimits() ServerLimits {
	return ServerLimits{
		GOMAXPROCS:               "2",
		GOGC:                     "100",
		GOMEMLIMIT:               "384MiB",
		ProcessMemoryLimitBytes:  512 * 1024 * 1024,
		CPUAffinityCores:         2,
		SQLMaxOpenConns:          "64",
		SQLMaxIdleConns:          "64",
		RedisPoolSize:            "256",
		RedisIdleTimeoutSeconds:  "1",
		RelayMaxIdleConns:        "1024",
		RelayMaxIdleConnsPerHost: "1024",
	}
}
