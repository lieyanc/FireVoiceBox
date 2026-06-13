package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lieyan666/firevoicebox/internal/audio"
	"github.com/lieyan666/firevoicebox/internal/config"
	"github.com/lieyan666/firevoicebox/internal/server"
	"github.com/lieyan666/firevoicebox/internal/store"
	"github.com/lieyan666/firevoicebox/internal/updater"
	"github.com/lieyan666/firevoicebox/internal/version"
	"github.com/lieyan666/firevoicebox/internal/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("firevoicebox", flag.ContinueOnError)
	configPath := fs.String("config", "config.json", "JSON config path")
	addr := fs.String("addr", "", "HTTP listen address override")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, created, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *addr != "" {
		cfg.Server.Addr = *addr
	}
	if err := os.MkdirAll(cfg.Server.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	st, err := store.Open(cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	var closeOnce sync.Once
	closeResources := func() {
		closeOnce.Do(func() {
			if err := st.Close(); err != nil {
				log.Printf("store: close: %v", err)
			}
		})
	}
	defer closeResources()

	au := audio.New(cfg.AudioDir(), cfg.Transcode)
	app := server.New(cfg, st, au, web.Dist())

	bgCtx, cancelBackground := context.WithCancel(context.Background())
	defer cancelBackground()

	var restartMu sync.Mutex
	restarting := false
	restartErrCh := make(chan error, 1)
	markRestarting := func() {
		restartMu.Lock()
		restarting = true
		restartMu.Unlock()
	}
	isRestarting := func() bool {
		restartMu.Lock()
		defer restartMu.Unlock()
		return restarting
	}

	var httpServer *http.Server
	ota := updater.New(
		func() updater.Config { return cfg.Update },
		func() string { return cfg.Server.DataDir },
		log.Default(),
		updater.RestartHooks{
			BeforeExec: func(tag string) error {
				markRestarting()
				cancelBackground()

				shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
					return err
				}
				closeResources()
				log.Printf("update: prepared restart for %s", tag)
				return nil
			},
			OnExecFailure: func(err error) {
				select {
				case restartErrCh <- err:
				default:
				}
			},
		},
	)
	app.SetUpdater(ota)

	httpServer = &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ota.StartBackground(bgCtx)

	errCh := make(chan error, 1)
	go func() {
		if created {
			log.Printf("config: created %s with generated secrets", *configPath)
			log.Printf("config: admin password = %s  (change it in %s if desired)", cfg.Admin.Password, *configPath)
		}
		log.Printf("FireVoiceBox %s (commit=%s, built=%s)", version.Version, version.Commit, version.BuildTime)
		log.Printf("FireVoiceBox listening on http://%s (data: %s)", displayAddr(cfg.Server.Addr), cfg.Server.DataDir)
		errCh <- httpServer.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			if isRestarting() {
				return <-restartErrCh
			}
			return nil
		}
		return err
	case <-sigCh:
		cancelBackground()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := httpServer.Shutdown(ctx)
		closeResources()
		return err
	case err := <-restartErrCh:
		return err
	}
}

func displayAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "localhost:8080"
	}
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}
