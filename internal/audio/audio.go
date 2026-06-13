// Package audio handles persistence of uploaded voice recordings on the
// filesystem, with an optional ffmpeg transcoding mode that normalizes all
// uploads to a single format (e.g. mp3) using the system ffmpeg binary.
package audio

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lieyan666/firevoicebox/internal/config"
)

// Storer saves audio blobs under a base directory.
type Storer struct {
	baseDir  string // .../data/audio
	mu       sync.RWMutex
	cfg      config.TranscodeConfig
	ffmpegOK bool
}

// New creates a Storer. If transcoding is enabled it probes the configured
// ffmpeg binary; if probing fails it logs a warning and falls back to storing
// uploads in their original format.
func New(audioDir string, cfg config.TranscodeConfig) *Storer {
	s := &Storer{baseDir: audioDir, cfg: cfg}
	s.ffmpegOK = ffmpegAvailable(cfg)
	return s
}

// Transcoding reports whether uploads will be transcoded.
func (s *Storer) Transcoding() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Enabled && s.ffmpegOK
}

// UpdateTranscodeConfig applies new transcoding settings for future uploads.
func (s *Storer) UpdateTranscodeConfig(cfg config.TranscodeConfig) {
	ffmpegOK := ffmpegAvailable(cfg)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	s.ffmpegOK = ffmpegOK
}

func ffmpegAvailable(cfg config.TranscodeConfig) bool {
	if !cfg.Enabled {
		return false
	}
	if err := probeFFmpeg(cfg.FFmpegPath); err != nil {
		log.Printf("audio: transcoding enabled but ffmpeg unavailable (%v); falling back to native format", err)
		return false
	}
	log.Printf("audio: transcoding enabled -> %s @ %s via %s", cfg.Format, cfg.Bitrate, cfg.FFmpegPath)
	return true
}

func probeFFmpeg(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, path, "-version").Run()
}

// Save writes the recording for a submission. srcMime is the browser-reported
// MIME type of the uploaded blob. It returns the path relative to the audio
// base directory, the final MIME type, and the stored size in bytes.
//
// When transcoding is active the source is written to a temp file, converted
// with ffmpeg, and the temp file removed. On transcode failure the behaviour
// depends on cfg.OnError ("keep_original" or "reject").
func (s *Storer) Save(projectID, subID, srcMime string, data []byte) (relPath, mime string, size int64, err error) {
	s.mu.RLock()
	cfg := s.cfg
	ffmpegOK := s.ffmpegOK
	s.mu.RUnlock()

	dir := filepath.Join(s.baseDir, projectID)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", "", 0, err
	}

	if cfg.Enabled && ffmpegOK {
		rel, m, sz, terr := s.saveTranscoded(dir, projectID, subID, srcMime, data, cfg)
		if terr == nil {
			return rel, m, sz, nil
		}
		if cfg.OnError == "reject" {
			return "", "", 0, fmt.Errorf("transcode failed: %w", terr)
		}
		log.Printf("audio: transcode failed for %s (%v); keeping original", subID, terr)
	}

	return s.saveNative(dir, projectID, subID, srcMime, data)
}

func (s *Storer) saveNative(dir, projectID, subID, srcMime string, data []byte) (string, string, int64, error) {
	ext := extForMime(srcMime)
	name := subID + ext
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return "", "", 0, err
	}
	mime := srcMime
	if mime == "" {
		mime = mimeForExt(ext)
	}
	return filepath.Join(projectID, name), mime, int64(len(data)), nil
}

func (s *Storer) saveTranscoded(dir, projectID, subID, srcMime string, data []byte, cfg config.TranscodeConfig) (string, string, int64, error) {
	tmp, err := os.CreateTemp(dir, "tmp-"+subID+"-*"+extForMime(srcMime))
	if err != nil {
		return "", "", 0, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", "", 0, err
	}
	tmp.Close()

	outName := subID + "." + cfg.Format
	outPath := filepath.Join(dir, outName)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// -vn drops any video track; -y overwrites; map only audio to the target codec.
	cmd := exec.CommandContext(ctx, cfg.FFmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", tmpPath, "-vn", "-b:a", cfg.Bitrate, outPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(outPath)
		return "", "", 0, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}

	info, err := os.Stat(outPath)
	if err != nil {
		return "", "", 0, err
	}
	return filepath.Join(projectID, outName), mimeForExt("." + cfg.Format), info.Size(), nil
}

// AbsPath returns the absolute path on disk for a stored relative path.
func (s *Storer) AbsPath(relPath string) string {
	return filepath.Join(s.baseDir, relPath)
}

// Delete removes a stored audio file. Missing files are not an error.
func (s *Storer) Delete(relPath string) error {
	err := os.Remove(s.AbsPath(relPath))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RemoveProject deletes a project's entire audio directory.
func (s *Storer) RemoveProject(projectID string) error {
	return os.RemoveAll(filepath.Join(s.baseDir, projectID))
}

func extForMime(mime string) string {
	// Strip codec parameters, e.g. "audio/webm;codecs=opus".
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	switch strings.TrimSpace(mime) {
	case "audio/webm", "video/webm":
		return ".webm"
	case "audio/mp4", "video/mp4", "audio/x-m4a", "audio/m4a":
		return ".mp4"
	case "audio/ogg", "application/ogg":
		return ".ogg"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	default:
		return ".bin"
	}
}

func mimeForExt(ext string) string {
	switch ext {
	case ".webm":
		return "audio/webm"
	case ".mp4":
		return "audio/mp4"
	case ".ogg":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}
