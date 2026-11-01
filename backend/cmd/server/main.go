// Command server runs the system design simulator API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"sysdesign/internal/api"
	"sysdesign/internal/designs"
)

const shutdownGrace = 5 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}

// config is everything the flags and the environment decide.
type config struct {
	addr        string
	static      string
	origins     string
	idle        time.Duration
	maxSessions int
	maxRuns     int
	dataDir     string
	maxDesigns  int
	healthcheck bool
}

func parseConfig(args []string, out io.Writer) (config, error) {
	var c config
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&c.addr, "addr", envOr("ADDR", ":8090"), "listen address")
	fs.StringVar(&c.static, "static", envOr("STATIC_DIR", ""), "directory with the built frontend to serve (optional)")
	fs.StringVar(&c.origins, "origins", envOr("ALLOWED_ORIGINS", "localhost:*,127.0.0.1:*"), "comma-separated WebSocket origin patterns")
	fs.DurationVar(&c.idle, "idle-timeout", api.DefaultIdleTimeout, "close stopped or paused sessions after this much silence")
	fs.IntVar(&c.maxSessions, "max-sessions", envInt("MAX_SESSIONS", api.DefaultMaxSessions), "maximum concurrent live simulations")
	fs.IntVar(&c.maxRuns, "max-runs", envInt("MAX_RUNS", 0), "maximum concurrent headless runs (0 = one per CPU core, at least 2)")
	fs.StringVar(&c.dataDir, "data-dir", envOr("DATA_DIR", ""), "directory for shared designs (empty keeps them in memory only)")
	fs.IntVar(&c.maxDesigns, "max-designs", envInt("MAX_DESIGNS", designs.DefaultMax), "how many shared designs to keep before dropping the oldest")
	fs.BoolVar(&c.healthcheck, "healthcheck", false, "probe a running server on -addr and exit 0 if it is healthy (for container health checks)")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if strings.TrimSpace(c.addr) == "" {
		return config{}, errors.New("-addr must not be empty")
	}
	return c, nil
}

// run starts the server and returns when ctx is cancelled (after a graceful shutdown) or serving fails.
func run(ctx context.Context, args []string, stdout io.Writer) error {
	cfg, err := parseConfig(args, os.Stderr)
	if err != nil {
		return err
	}
	if cfg.healthcheck {
		if err := checkHealth(healthURL(cfg.addr)); err != nil {
			return fmt.Errorf("unhealthy: %w", err)
		}
		return nil
	}

	logger := slog.New(slog.NewJSONHandler(stdout, nil))
	store, err := designs.New(cfg.dataDir, cfg.maxDesigns)
	if err != nil {
		return fmt.Errorf("opening design store: %w", err)
	}
	app := api.New(api.Config{StaticDir: cfg.static, OriginPatterns: strings.Split(cfg.origins, ","), MaxSessions: cfg.maxSessions, MaxRuns: cfg.maxRuns, IdleTimeout: cfg.idle, Designs: store, Logger: logger})
	srv := &http.Server{
		Addr:              cfg.addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second, // bodies are small; a hijacked WebSocket connection is exempt
		IdleTimeout:       2 * time.Minute,
		// No WriteTimeout: a long headless run or sweep answers only when it finishes.
	}
	ln, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("listening", "addr", ln.Addr().String(), "static", cfg.static, "dataDir", cfg.dataDir)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	logger.Info("shutting down")
	app.Close() // Shutdown does not touch hijacked WebSocket sessions
	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func envInt(k string, d int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil && n > 0 {
		return n
	}
	return d
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
