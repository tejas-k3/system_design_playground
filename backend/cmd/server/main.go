// Command server runs the system design simulator API.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"sysdesign/internal/api"
)

func main() {
	addr := flag.String("addr", envOr("ADDR", ":8090"), "listen address")
	static := flag.String("static", envOr("STATIC_DIR", ""), "directory with the built frontend to serve (optional)")
	origins := flag.String("origins", envOr("ALLOWED_ORIGINS", "localhost:*,127.0.0.1:*"), "comma-separated WebSocket origin patterns")
	maxSessions := flag.Int("max-sessions", envInt("MAX_SESSIONS", api.DefaultMaxSessions), "maximum concurrent live simulations")
	flag.Parse()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.New(api.Config{StaticDir: *static, OriginPatterns: strings.Split(*origins, ","), MaxSessions: *maxSessions}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("listening on %s (static=%q)", *addr, *static)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
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
