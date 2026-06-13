package version

import (
	"crypto/sha256"
	"encoding/hex"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func Info() map[string]string {
	return map[string]string{
		"version":    Version,
		"commit":     Commit,
		"build_time": BuildTime,
	}
}

func ClientCacheKey() string {
	sum := sha256.Sum256([]byte(Version + "\x00" + Commit + "\x00" + BuildTime))
	return hex.EncodeToString(sum[:8])
}
