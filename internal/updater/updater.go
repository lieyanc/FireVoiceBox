package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lieyan666/firevoicebox/internal/version"
)

type Config struct {
	Enabled       bool   `json:"enabled"`
	Channel       string `json:"channel"`
	CheckInterval int    `json:"check_interval"`
	Tag           string `json:"tag"`
	Repo          string `json:"repo"`
}

type Status struct {
	State            string  `json:"state"`
	CurrentVersion   string  `json:"current_version"`
	LatestVersion    string  `json:"latest_version,omitempty"`
	IsPrerelease     bool    `json:"is_prerelease"`
	Progress         float64 `json:"progress,omitempty"`
	DownloadProgress float64 `json:"download_progress,omitempty"`
	Error            string  `json:"error,omitempty"`
	LastCheck        string  `json:"last_check,omitempty"`
	ReleaseNotes     string  `json:"release_notes,omitempty"`
}

type CheckResult struct {
	HasUpdate      bool   `json:"has_update"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version,omitempty"`
	IsPrerelease   bool   `json:"is_prerelease"`
	ReleaseNotes   string `json:"release_notes,omitempty"`
	Channel        string `json:"channel"`
}

type RestartHooks struct {
	BeforeExec    func(tag string) error
	OnExecFailure func(error)
}

type Updater struct {
	cfg     func() Config
	dataDir func() string
	logger  *log.Logger
	hooks   RestartHooks

	mu     sync.RWMutex
	status Status

	bgCtx context.Context

	pendingBinaryPath string
	pendingTag        string
}

const (
	progressChecking      = 5
	progressReleaseFound  = 10
	progressDownloadStart = 10
	progressDownloadDone  = 90
	progressVerifyStart   = 92
	progressVerifyDone    = 95
	progressApplying      = 98
	progressComplete      = 100
)

var githubReleaseBaseURL = "https://github.com"

func newGitHubRequestWithContext(ctx context.Context, method, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cache-Control", "no-cache, no-store, max-age=0")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")
	return req, nil
}

func New(cfg func() Config, dataDir func() string, logger *log.Logger, hooks RestartHooks) *Updater {
	if logger == nil {
		logger = log.Default()
	}
	return &Updater{
		cfg:     cfg,
		dataDir: dataDir,
		logger:  logger,
		hooks:   hooks,
		status: Status{
			State:          "idle",
			CurrentVersion: version.Version,
		},
	}
}

func (u *Updater) Status() Status {
	u.mu.RLock()
	defer u.mu.RUnlock()
	s := u.status
	s.CurrentVersion = version.Version
	return s
}

func (u *Updater) CheckOnly(ctx context.Context) (CheckResult, error) {
	cfg := normalizeConfig(u.cfg())
	result := CheckResult{
		CurrentVersion: version.Version,
		Channel:        cfg.Channel,
	}

	if _, err := u.targetName(); err != nil {
		return result, err
	}

	release, hasUpdate, err := u.checkForUpdate(ctx, cfg)
	if err != nil {
		return result, err
	}

	u.mu.Lock()
	u.status.LastCheck = time.Now().UTC().Format(time.RFC3339)
	u.mu.Unlock()

	if release == nil {
		return result, nil
	}

	result.HasUpdate = hasUpdate
	result.LatestVersion = release.displayVersion()
	result.IsPrerelease = release.Prerelease
	result.ReleaseNotes = release.Body

	u.mu.Lock()
	u.status.LatestVersion = release.displayVersion()
	u.status.IsPrerelease = release.Prerelease
	u.status.ReleaseNotes = release.Body
	u.mu.Unlock()

	return result, nil
}

func (u *Updater) StartUpdate(_ context.Context) {
	go u.performUpdate(u.bgContext())
}

func (u *Updater) ApplyPending(_ context.Context) error {
	u.mu.Lock()
	state := u.status.State
	path := u.pendingBinaryPath
	tag := u.pendingTag

	if state != "ready" || path == "" {
		u.mu.Unlock()
		return fmt.Errorf("no pending update to apply")
	}

	u.status.State = "applying"
	u.status.Progress = progressApplying
	u.status.DownloadProgress = 0
	u.pendingBinaryPath = ""
	u.pendingTag = ""
	u.mu.Unlock()

	go func() {
		time.Sleep(200 * time.Millisecond)
		if err := u.applyUpdate(path, tag); err != nil {
			u.notifyExecFailure(err)
			u.setError("apply failed: " + err.Error())
		}
	}()
	return nil
}

func (u *Updater) DismissPending() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.status.State == "ready" {
		if u.pendingBinaryPath != "" {
			_ = os.Remove(u.pendingBinaryPath)
		}
		u.pendingBinaryPath = ""
		u.pendingTag = ""
		u.status.State = "idle"
		u.status.LatestVersion = ""
		u.status.Progress = 0
		u.status.DownloadProgress = 0
		u.status.Error = ""
	}
}

func (u *Updater) StartBackground(ctx context.Context) {
	cfg := normalizeConfig(u.cfg())
	u.bgCtx = ctx
	if !cfg.Enabled {
		u.logger.Printf("update: disabled")
		return
	}
	u.logger.Printf("update: enabled, channel=%s, interval=%ds", cfg.Channel, cfg.CheckInterval)
	go u.loop(ctx)
}

func (u *Updater) bgContext() context.Context {
	if u.bgCtx != nil {
		return u.bgCtx
	}
	return context.Background()
}

func (u *Updater) loop(ctx context.Context) {
	select {
	case <-time.After(30 * time.Second):
	case <-ctx.Done():
		return
	}

	u.checkAndUpdate(ctx)

	for {
		cfg := normalizeConfig(u.cfg())
		interval := time.Duration(cfg.CheckInterval) * time.Second
		if interval < time.Minute {
			interval = time.Minute
		}
		select {
		case <-time.After(interval):
			u.checkAndUpdate(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (u *Updater) checkAndUpdate(ctx context.Context) {
	cfg := normalizeConfig(u.cfg())
	if !cfg.Enabled {
		return
	}
	u.performUpdate(ctx)
}

func (u *Updater) performUpdate(ctx context.Context) {
	cfg := normalizeConfig(u.cfg())

	u.mu.Lock()
	if u.status.State == "checking" || u.status.State == "ready" || u.status.State == "downloading" || u.status.State == "applying" {
		u.mu.Unlock()
		return
	}
	u.status.State = "checking"
	u.status.Progress = progressChecking
	u.status.Error = ""
	u.status.DownloadProgress = 0
	u.mu.Unlock()

	release, hasUpdate, err := u.checkForUpdate(ctx, cfg)
	if err != nil {
		u.setError("check failed: " + err.Error())
		return
	}
	if release == nil || !hasUpdate {
		u.mu.Lock()
		u.status.State = "idle"
		u.status.Progress = 0
		u.status.DownloadProgress = 0
		u.status.LastCheck = time.Now().UTC().Format(time.RFC3339)
		u.mu.Unlock()
		return
	}

	u.mu.Lock()
	u.status.LatestVersion = release.displayVersion()
	u.status.IsPrerelease = release.Prerelease
	u.status.ReleaseNotes = release.Body
	u.status.LastCheck = time.Now().UTC().Format(time.RFC3339)
	u.status.Progress = progressReleaseFound
	u.mu.Unlock()

	binaryPath, err := u.download(ctx, cfg, release)
	if err != nil {
		u.setError("download failed: " + err.Error())
		return
	}

	if cfg.Channel == "stable" {
		u.mu.Lock()
		u.status.State = "applying"
		u.status.Progress = progressApplying
		u.status.DownloadProgress = 0
		u.mu.Unlock()
		if err := u.applyUpdate(binaryPath, release.TagName); err != nil {
			u.notifyExecFailure(err)
			u.setError("apply failed: " + err.Error())
		}
		return
	}

	u.mu.Lock()
	u.status.State = "ready"
	u.status.Progress = progressVerifyDone
	u.status.DownloadProgress = 0
	u.pendingBinaryPath = binaryPath
	u.pendingTag = release.TagName
	u.mu.Unlock()
	u.logger.Printf("update: pre-release %s ready, waiting for admin confirmation", release.TagName)
}

func (u *Updater) setError(msg string) {
	u.logger.Printf("update: %s", msg)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.status.State = "failed"
	u.status.Error = msg
	u.status.LastCheck = time.Now().UTC().Format(time.RFC3339)
}

func clampProgress(progress float64) float64 {
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func overallDownloadProgress(downloadProgress float64) float64 {
	downloadProgress = clampProgress(downloadProgress)
	span := progressDownloadDone - progressDownloadStart
	return progressDownloadStart + downloadProgress*float64(span)/100
}

func (u *Updater) notifyExecFailure(err error) {
	if err == nil || u.hooks.OnExecFailure == nil {
		return
	}
	u.hooks.OnExecFailure(err)
}

type releaseInfo struct {
	TagName    string
	Prerelease bool
	Body       string
	Version    string
	Commit     string
	BuildTime  string
}

type releaseVersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	Tag       string `json:"tag"`
}

func (r releaseInfo) displayVersion() string {
	if strings.TrimSpace(r.Version) != "" {
		return strings.TrimSpace(r.Version)
	}
	return r.TagName
}

func (u *Updater) checkForUpdate(ctx context.Context, cfg Config) (*releaseInfo, bool, error) {
	if cfg.Repo == "" {
		return nil, false, fmt.Errorf("update repo is not configured")
	}

	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	targetName, err := u.targetName()
	if err != nil {
		return nil, false, err
	}

	tag, err := u.resolveReleaseTag(checkCtx, cfg, targetName)
	if err != nil {
		return nil, false, err
	}
	release := releaseInfo{
		TagName:    tag,
		Prerelease: cfg.Channel != "stable",
	}

	binaryURL := releaseDownloadURL(cfg.Repo, tag, targetName)
	u.logger.Printf("update: checking %s", binaryURL)

	if err := u.ensureAssetAvailable(checkCtx, binaryURL); err != nil {
		return nil, false, err
	}

	if cfg.Channel != "stable" {
		if err := u.loadReleaseVersion(checkCtx, cfg, &release); err != nil {
			u.logger.Printf("update: version metadata unavailable for %s: %v", tag, err)
		}
	}

	if !u.isNewer(release, cfg.Channel) {
		u.logger.Printf("update: already up to date (%s)", release.displayVersion())
		return &release, false, nil
	}

	return &release, true, nil
}

func (u *Updater) loadReleaseVersion(ctx context.Context, cfg Config, release *releaseInfo) error {
	versionURL := releaseDownloadURL(cfg.Repo, release.TagName, "version.json")
	req, err := newGitHubRequestWithContext(ctx, http.MethodGet, versionURL)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("version metadata returned status %d", resp.StatusCode)
	}

	var info releaseVersionInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16*1024)).Decode(&info); err != nil {
		return fmt.Errorf("decode version metadata: %w", err)
	}
	release.Version = strings.TrimSpace(info.Version)
	release.Commit = strings.TrimSpace(info.Commit)
	release.BuildTime = strings.TrimSpace(info.BuildTime)
	return nil
}

func (u *Updater) resolveReleaseTag(ctx context.Context, cfg Config, targetName string) (string, error) {
	if cfg.Tag != "" {
		return cfg.Tag, nil
	}
	if cfg.Channel != "stable" {
		return "dev", nil
	}
	return u.resolveLatestStableTag(ctx, cfg.Repo, targetName)
}

func (u *Updater) resolveLatestStableTag(ctx context.Context, repo, targetName string) (string, error) {
	req, err := newGitHubRequestWithContext(ctx, http.MethodHead, latestReleaseDownloadURL(repo, targetName))
	if err != nil {
		return "", err
	}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("latest release redirect returned status %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("latest release redirect missing location")
	}
	locationURL, err := req.URL.Parse(location)
	if err != nil {
		return "", fmt.Errorf("parse latest release redirect: %w", err)
	}
	tag, ok := releaseTagFromDownloadPath(locationURL.EscapedPath())
	if !ok {
		tag, ok = releaseTagFromDownloadPath(locationURL.Path)
	}
	if !ok || tag == "" {
		return "", fmt.Errorf("latest release redirect did not include a tag")
	}
	u.logger.Printf("update: resolved latest release tag %s", tag)
	return tag, nil
}

func (u *Updater) ensureAssetAvailable(ctx context.Context, assetURL string) error {
	req, err := newGitHubRequestWithContext(ctx, http.MethodHead, assetURL)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("release asset not found: %s", assetURL)
	}
	return fmt.Errorf("release asset check returned status %d", resp.StatusCode)
}

func (u *Updater) isNewer(release releaseInfo, channel string) bool {
	current := version.Version
	if current == "dev" {
		return true
	}
	remoteTag := release.TagName
	if channel == "stable" {
		remoteVersion := release.displayVersion()
		if remoteVersion != "" {
			return semverGreater(remoteVersion, current)
		}
		return semverGreater(remoteTag, current)
	}

	remoteCommit := normalizeCommit(release.Commit)
	currentCommit := normalizeCommit(version.Commit)
	if remoteCommit != "" && currentCommit != "" {
		return remoteCommit != currentCommit
	}

	remoteVersion := release.displayVersion()
	if remoteTag == "dev" && remoteVersion == "dev" {
		u.logger.Printf("update: dev release missing comparable commit current=%s remote=%s, skipping", current, remoteTag)
		return false
	}

	remoteNum, remoteSHA := parseDevTag(remoteVersion)
	localNum, localSHA := parseDevTag(current)
	if remoteSHA != "" && localSHA != "" && remoteSHA == localSHA {
		return false
	}
	if remoteNum > 0 && localNum > 0 {
		return remoteNum > localNum
	}
	u.logger.Printf("update: cannot compare versions current=%s remote=%s, skipping", current, remoteTag)
	return false
}

func normalizeCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if commit == "" || commit == "unknown" {
		return ""
	}
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func semverGreater(a, b string) bool {
	av := parseSemver(strings.TrimPrefix(a, "v"))
	bv := parseSemver(strings.TrimPrefix(b, "v"))
	for i := 0; i < 3; i++ {
		if av[i] > bv[i] {
			return true
		}
		if av[i] < bv[i] {
			return false
		}
	}
	return false
}

func parseSemver(s string) [3]int {
	var result [3]int
	parts := strings.SplitN(s, ".", 3)
	for i, p := range parts {
		if i >= 3 {
			break
		}
		if idx := strings.IndexByte(p, '-'); idx >= 0 {
			p = p[:idx]
		}
		n, _ := strconv.Atoi(p)
		result[i] = n
	}
	return result
}

func parseDevTag(tag string) (runNumber int, sha string) {
	parts := strings.SplitN(tag, "-", 4)
	if len(parts) >= 4 && parts[0] == "dev" {
		n, _ := strconv.Atoi(parts[1])
		return n, parts[3]
	}
	return 0, ""
}

func releaseDownloadURL(repo, tag, asset string) string {
	return strings.TrimRight(githubReleaseBaseURL, "/") + "/" + strings.Trim(repo, "/") +
		"/releases/download/" + url.PathEscape(tag) + "/" + url.PathEscape(asset)
}

func latestReleaseDownloadURL(repo, asset string) string {
	return strings.TrimRight(githubReleaseBaseURL, "/") + "/" + strings.Trim(repo, "/") +
		"/releases/latest/download/" + url.PathEscape(asset)
}

func releaseTagFromDownloadPath(path string) (string, bool) {
	const relSegment = "/releases/download/"
	idx := strings.Index(path, relSegment)
	if idx < 0 {
		return "", false
	}
	rest := path[idx+len(relSegment):]
	end := strings.IndexByte(rest, '/')
	if end < 0 {
		return "", false
	}
	tag, err := url.PathUnescape(rest[:end])
	if err != nil {
		return "", false
	}
	return tag, true
}

func (u *Updater) targetName() (string, error) {
	return releaseTargetName(runtime.GOOS, runtime.GOARCH)
}

func releaseTargetName(goos, goarch string) (string, error) {
	switch {
	case goos == "linux" && goarch == "amd64":
		return "firevoicebox-linux-amd64", nil
	case goos == "linux" && goarch == "arm64":
		return "firevoicebox-linux-arm64", nil
	case goos == "darwin" && goarch == "amd64":
		return "firevoicebox-darwin-amd64", nil
	case goos == "darwin" && goarch == "arm64":
		return "firevoicebox-darwin-arm64", nil
	default:
		return "", fmt.Errorf("updates are not supported on %s/%s", goos, goarch)
	}
}

func (u *Updater) download(ctx context.Context, cfg Config, release *releaseInfo) (string, error) {
	u.mu.Lock()
	u.status.State = "downloading"
	u.status.Progress = progressDownloadStart
	u.status.DownloadProgress = 0
	u.mu.Unlock()

	targetName, err := u.targetName()
	if err != nil {
		return "", err
	}

	updateDir := filepath.Join(u.dataDir(), "updates")
	if err := os.MkdirAll(updateDir, 0o755); err != nil {
		return "", fmt.Errorf("create update dir: %w", err)
	}

	finalName := "firevoicebox-" + sanitizePathPart(release.TagName)
	if runtime.GOOS == "windows" {
		finalName += ".exe"
	}
	tmpPath := filepath.Join(updateDir, finalName+".tmp")
	finalPath := filepath.Join(updateDir, finalName)

	downloadURL := releaseDownloadURL(cfg.Repo, release.TagName, targetName)
	if err := u.downloadFile(ctx, downloadURL, tmpPath, 0); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("download binary: %w", err)
	}

	sha256URL := releaseDownloadURL(cfg.Repo, release.TagName, targetName+".sha256")
	expectedHash, hasHash, err := u.fetchSHA256(ctx, sha256URL)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("fetch sha256: %w", err)
	}
	if hasHash {
		u.mu.Lock()
		u.status.Progress = progressVerifyStart
		u.mu.Unlock()

		actualHash, err := fileSHA256(tmpPath)
		if err != nil {
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("compute sha256: %w", err)
		}
		if !strings.EqualFold(actualHash, expectedHash) {
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("sha256 mismatch: expected %s, got %s", expectedHash, actualHash)
		}
		u.logger.Printf("update: SHA256 verified for %s", release.TagName)
	}

	u.mu.Lock()
	u.status.Progress = progressVerifyDone
	u.mu.Unlock()

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	u.logger.Printf("update: downloaded %s to %s", release.TagName, finalPath)
	return finalPath, nil
}

func (u *Updater) downloadFile(ctx context.Context, url, destPath string, expectedSize int64) error {
	req, err := newGitHubRequestWithContext(ctx, http.MethodGet, url)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	totalSize := resp.ContentLength
	if totalSize <= 0 && expectedSize > 0 {
		totalSize = expectedSize
	}

	var written int64
	var lastProgress float64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				return wErr
			}
			written += int64(n)
			if totalSize > 0 {
				progress := float64(written) / float64(totalSize) * 100
				if progress-lastProgress >= 1 || progress >= 100 {
					u.mu.Lock()
					u.status.DownloadProgress = clampProgress(progress)
					u.status.Progress = overallDownloadProgress(progress)
					u.mu.Unlock()
					lastProgress = progress
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}

	u.mu.Lock()
	u.status.DownloadProgress = progressComplete
	u.status.Progress = overallDownloadProgress(progressComplete)
	u.mu.Unlock()
	return nil
}

func (u *Updater) fetchSHA256(ctx context.Context, url string) (string, bool, error) {
	req, err := newGitHubRequestWithContext(ctx, http.MethodGet, url)
	if err != nil {
		return "", false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		u.logger.Printf("update: checksum asset not found, skipping SHA256 verification")
		return "", false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("sha256 download returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", false, err
	}
	parts := strings.Fields(strings.TrimSpace(string(body)))
	if len(parts) == 0 {
		return "", false, fmt.Errorf("empty sha256 file")
	}
	return parts[0], true, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (u *Updater) applyUpdate(newBinaryPath, tag string) error {
	u.mu.Lock()
	u.status.State = "applying"
	u.status.Progress = progressApplying
	u.mu.Unlock()

	if runtime.GOOS == "windows" {
		return u.applyUpdateWindows(newBinaryPath, tag)
	}
	return u.applyUpdateUnix(newBinaryPath, tag)
}

func (u *Updater) applyUpdateUnix(newBinaryPath, tag string) error {
	if u.hooks.BeforeExec != nil {
		if err := u.hooks.BeforeExec(tag); err != nil {
			return fmt.Errorf("prepare restart: %w", err)
		}
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	backupPath := execPath + ".bak"
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}
	if err := copyFile(newBinaryPath, execPath); err != nil {
		_ = os.Rename(backupPath, execPath)
		return fmt.Errorf("install new binary: %w", err)
	}
	if err := os.Chmod(execPath, 0o755); err != nil {
		_ = os.Rename(backupPath, execPath)
		_ = os.Remove(newBinaryPath)
		return fmt.Errorf("chmod new binary: %w", err)
	}

	_ = os.Remove(backupPath)
	_ = os.Remove(newBinaryPath)

	u.logger.Printf("update: restarting with new binary %s", tag)
	u.mu.Lock()
	u.status.Progress = progressComplete
	u.mu.Unlock()
	return replaceProcess(execPath, os.Args, os.Environ())
}

func (u *Updater) applyUpdateWindows(newBinaryPath, tag string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	execPath, err = filepath.Abs(execPath)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	updateDir := filepath.Dir(newBinaryPath)
	scriptPath := filepath.Join(updateDir, "apply-"+sanitizePathPart(tag)+".ps1")
	backupPath := execPath + ".bak"
	script := strings.Join([]string{
		"$ErrorActionPreference = 'Stop'",
		fmt.Sprintf("$pidToWait = %d", os.Getpid()),
		"$exe = " + psQuote(execPath),
		"$new = " + psQuote(newBinaryPath),
		"$bak = " + psQuote(backupPath),
		"$argsList = " + psArray(os.Args[1:]),
		"$workDir = " + psQuote(cwd),
		"while (Get-Process -Id $pidToWait -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 250 }",
		"if (Test-Path $bak) { Remove-Item -Force $bak }",
		"if (Test-Path $exe) { Move-Item -Force $exe $bak }",
		"Copy-Item -Force $new $exe",
		"Remove-Item -Force $new",
		"Start-Process -FilePath $exe -ArgumentList $argsList -WorkingDirectory $workDir",
		"Remove-Item -Force $PSCommandPath",
		"",
	}, "\r\n")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return fmt.Errorf("write apply script: %w", err)
	}

	if u.hooks.BeforeExec != nil {
		if err := u.hooks.BeforeExec(tag); err != nil {
			return fmt.Errorf("prepare restart: %w", err)
		}
	}

	proc, err := os.StartProcess("powershell.exe", []string{
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
	}, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Env:   os.Environ(),
	})
	if err != nil {
		return fmt.Errorf("start apply script: %w", err)
	}
	_ = proc.Release()

	u.logger.Printf("update: restarting with new binary %s", tag)
	u.mu.Lock()
	u.status.Progress = progressComplete
	u.mu.Unlock()
	os.Exit(0)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.Channel) == "" {
		cfg.Channel = "stable"
	}
	cfg.Channel = strings.ToLower(strings.TrimSpace(cfg.Channel))
	if cfg.Channel != "stable" {
		cfg.Channel = "dev"
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 3600
	}
	cfg.Tag = strings.TrimSpace(cfg.Tag)
	cfg.Repo = strings.TrimSpace(cfg.Repo)
	if cfg.Repo == "" {
		cfg.Repo = "lieyanc/FireVoiceBox"
	}
	return cfg
}

func sanitizePathPart(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "update"
	}
	return b.String()
}

func psQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func psArray(values []string) string {
	if len(values) == 0 {
		return "@()"
	}
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, psQuote(value))
	}
	return "@(" + strings.Join(quoted, ", ") + ")"
}
