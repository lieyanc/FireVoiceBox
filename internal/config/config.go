// Package config loads the JSON configuration for FireVoiceBox. The default
// configuration is embedded in the binary and self-released to disk on first
// run, with secrets (admin password, cookie signing secret) auto-generated.
package config

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lieyan666/firevoicebox/internal/updater"
)

//go:embed template.json
var templateJSON []byte

// Config is the top-level application configuration.
type Config struct {
	Server    ServerConfig    `json:"server"`
	Admin     AdminConfig     `json:"admin"`
	Transcode TranscodeConfig `json:"transcode"`
	Update    updater.Config  `json:"update"`

	path string
}

// ServerConfig holds HTTP server and storage settings.
type ServerConfig struct {
	Addr         string `json:"addr"`
	DataDir      string `json:"data_dir"`
	TrustedProxy bool   `json:"trusted_proxy"`
	MaxUploadMB  int    `json:"max_upload_mb"`
	Secret       string `json:"secret"` // HMAC key for signing session cookies
}

// AdminConfig holds the global owner credentials.
type AdminConfig struct {
	Password string `json:"password"`
}

// TranscodeConfig controls optional server-side audio transcoding via ffmpeg.
type TranscodeConfig struct {
	Enabled    bool   `json:"enabled"`
	FFmpegPath string `json:"ffmpeg_path"`
	Format     string `json:"format"`
	Bitrate    string `json:"bitrate"`
	OnError    string `json:"on_error"` // "keep_original" | "reject"
}

// DBPath returns the SQLite database file path within the data directory.
func (c *Config) DBPath() string {
	return filepath.Join(c.Server.DataDir, "firevoicebox.db")
}

// AudioDir returns the directory where audio files are stored.
func (c *Config) AudioDir() string {
	return filepath.Join(c.Server.DataDir, "audio")
}

// Load reads the configuration from path. If the file does not exist, the
// embedded template is written out (self-release) with freshly generated
// secrets. The returned bool reports whether the file was created on this call.
func Load(path string) (*Config, bool, error) {
	var raw []byte
	var created bool
	if b, err := os.ReadFile(path); err == nil {
		raw = b
	} else if os.IsNotExist(err) {
		raw = templateJSON
		created = true
	} else {
		return nil, false, fmt.Errorf("read config %s: %w", path, err)
	}

	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	c.path = path

	c.applyDefaults()
	changed := c.fillSecrets()

	if created || changed {
		if err := c.save(path); err != nil {
			return nil, false, fmt.Errorf("write config %s: %w", path, err)
		}
	}
	return &c, created, nil
}

// Replace persists next to the same config file and swaps it into the live
// config only after the file write succeeds.
func (c *Config) Replace(next Config) error {
	if c.path == "" {
		return fmt.Errorf("config path is not set")
	}
	next.path = c.path
	next.applyDefaults()
	next.fillSecrets()
	if err := next.save(next.path); err != nil {
		return err
	}
	*c = next
	return nil
}

func (c *Config) applyDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Server.DataDir == "" {
		c.Server.DataDir = "./data"
	}
	if c.Server.MaxUploadMB <= 0 {
		c.Server.MaxUploadMB = 25
	}
	if c.Transcode.FFmpegPath == "" {
		c.Transcode.FFmpegPath = "ffmpeg"
	}
	if c.Transcode.Format == "" {
		c.Transcode.Format = "mp3"
	}
	if c.Transcode.Bitrate == "" {
		c.Transcode.Bitrate = "128k"
	}
	if c.Transcode.OnError == "" {
		c.Transcode.OnError = "keep_original"
	}
	if c.Update.Channel == "" {
		c.Update.Channel = "stable"
	}
	if c.Update.CheckInterval <= 0 {
		c.Update.CheckInterval = 3600
	}
	if c.Update.Repo == "" {
		c.Update.Repo = "lieyanc/FireVoiceBox"
	}
}

// fillSecrets generates any missing secrets and reports whether it changed
// the config (so the caller can persist the generated values).
func (c *Config) fillSecrets() bool {
	changed := false
	if c.Admin.Password == "" {
		c.Admin.Password = randomToken(12)
		changed = true
	}
	if c.Server.Secret == "" {
		c.Server.Secret = randomToken(32)
		changed = true
	}
	return changed
}

func (c *Config) save(path string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func randomToken(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		panic("config: failed to read random bytes: " + err.Error())
	}
	return hex.EncodeToString(b)
}
