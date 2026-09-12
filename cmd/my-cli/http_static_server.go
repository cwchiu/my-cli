package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// Environment variables consulted by the http-static-server command.
//
// #nosec G101 -- these are environment variable names, not credentials.
const (
	envHTTPStaticListen = "MYCLI_HTTP_STATIC_SERVER_LISTEN"
	envHTTPStaticPort   = "MYCLI_HTTP_STATIC_SERVER_PORT"
	envHTTPStaticFolder = "MYCLI_HTTP_STATIC_SERVER_FOLDER"
)

const (
	// defaultHSSListen binds to loopback only, so a stray server never
	// exposes the folder to the network; pass --listen 0.0.0.0 to opt out.
	defaultHSSListen = "127.0.0.1"
	// defaultHSSPort is used when neither --port nor
	// MYCLI_HTTP_STATIC_SERVER_PORT is set.
	defaultHSSPort = "8080"
)

const (
	// defaultHSSReadHeaderTimeout guards against slowloris-style
	// connection exhaustion (gosec G114 requires explicit timeouts).
	defaultHSSReadHeaderTimeout = 10 * time.Second
	// defaultHSSShutdownGrace bounds how long in-flight requests may
	// finish after an interrupt signal.
	defaultHSSShutdownGrace = 5 * time.Second
)

// httpStaticServerConfig holds the raw flag values for one
// http-static-server run. The port stays a string so an explicit --port 0
// (OS-assigned port) is distinguishable from an unset flag.
type httpStaticServerConfig struct {
	listen string
	port   string
	folder string
}

// httpStaticServerSettings is the validated configuration used to run the
// server.
type httpStaticServerSettings struct {
	addr   string
	folder string
}

// newHTTPStaticServerCmd returns the `http-static-server` subcommand, which
// serves a folder over HTTP until interrupted.
func newHTTPStaticServerCmd() *cobra.Command {
	var raw httpStaticServerConfig

	cmd := &cobra.Command{
		Use:   "http-static-server",
		Short: "Serve a folder over HTTP until interrupted",
		Long: `Serve a folder over HTTP with the standard library file server.

The server keeps running until it receives an interrupt (Ctrl-C) or SIGTERM,
then drains in-flight requests before exiting. Requests are logged to stderr
in a structured format; nothing is written to stdout.

By default the server binds to 127.0.0.1 only, so the folder is reachable
from the local machine alone. Pass --listen 0.0.0.0 to expose it to the
network.

Directory requests serve index.html when present, otherwise a listing.
Path traversal outside the folder is rejected by the standard file server.`,
		Example: `  my-cli http-static-server --folder ./report
  my-cli http-static-server --folder ./report --listen 0.0.0.0 --port 9000
  MYCLI_HTTP_STATIC_SERVER_FOLDER=./report my-cli http-static-server`,
		Args: wrapUsage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			settings, err := resolveHTTPStaticServerConfig(raw)
			if err != nil {
				return fmt.Errorf("%w: %w", errUsage, err)
			}

			logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

			return runHTTPStaticServer(cmd.Context(), settings, logger)
		},
	}

	cmd.Flags().StringVar(&raw.listen, "listen", defaultHSSListen,
		"IP address to bind (falls back to MYCLI_HTTP_STATIC_SERVER_LISTEN)")
	cmd.Flags().StringVar(&raw.port, "port", "",
		"TCP port to bind, 0 picks a free port (default 8080, falls back to MYCLI_HTTP_STATIC_SERVER_PORT)")
	cmd.Flags().StringVar(&raw.folder, "folder", "",
		"folder to serve (falls back to MYCLI_HTTP_STATIC_SERVER_FOLDER)")

	return cmd
}

// resolveHTTPStaticServerConfig validates the raw flag values and fills in
// the settings derived from the environment, failing fast before any
// filesystem or network I/O.
func resolveHTTPStaticServerConfig(raw httpStaticServerConfig) (httpStaticServerSettings, error) {
	listen := raw.listen
	if listen == "" {
		listen = os.Getenv(envHTTPStaticListen)
	}

	if listen == "" {
		listen = defaultHSSListen
	}

	if net.ParseIP(listen) == nil {
		return httpStaticServerSettings{}, fmt.Errorf("invalid listen address %q", listen)
	}

	portRaw := raw.port
	if portRaw == "" {
		portRaw = os.Getenv(envHTTPStaticPort)
	}

	if portRaw == "" {
		portRaw = defaultHSSPort
	}

	port, err := strconv.Atoi(portRaw)
	if err != nil || port < 0 || port > 65535 {
		return httpStaticServerSettings{}, fmt.Errorf("invalid port %q", portRaw)
	}

	folder := raw.folder
	if folder == "" {
		folder = os.Getenv(envHTTPStaticFolder)
	}

	if folder == "" {
		return httpStaticServerSettings{}, fmt.Errorf(
			"missing folder to serve: set --folder or %s", envHTTPStaticFolder)
	}

	// The folder is intentionally user-supplied: serving a directory of the
	// operator's choice is the whole purpose of this command, and
	// http.FileServer confines request paths to that directory.
	info, err := os.Stat(folder) //nolint:gosec // G703: user-selected folder is the feature, not an injection sink
	if err != nil {
		return httpStaticServerSettings{}, fmt.Errorf("stat folder %q: %w", folder, err)
	}

	if !info.IsDir() {
		return httpStaticServerSettings{}, fmt.Errorf("folder %q is not a directory", folder)
	}

	return httpStaticServerSettings{
		addr:   net.JoinHostPort(listen, strconv.Itoa(port)),
		folder: folder,
	}, nil
}

// runHTTPStaticServer serves the folder until ctx is cancelled by an
// interrupt signal, then drains in-flight requests within the shutdown
// grace period.
func runHTTPStaticServer(ctx context.Context, settings httpStaticServerSettings, logger *slog.Logger) error {
	handler := withHTTPAccessLog(logger, http.FileServer(http.Dir(settings.folder)))

	srv := &http.Server{
		Addr:              settings.addr,
		Handler:           handler,
		ReadHeaderTimeout: defaultHSSReadHeaderTimeout,
	}

	var lc net.ListenConfig

	listener, err := lc.Listen(ctx, "tcp", settings.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", settings.addr, err)
	}

	logger.Info("http server listening", "addr", listener.Addr().String(), "folder", settings.folder)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)

	go func() {
		serveErr <- srv.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("http server failed: %w", err)
	case <-ctx.Done():
		return shutdownHTTPStaticServer(ctx, srv, logger)
	}
}

// shutdownHTTPStaticServer drains in-flight requests within the shutdown
// grace period.
func shutdownHTTPStaticServer(ctx context.Context, srv *http.Server, logger *slog.Logger) error {
	logger.Info("shutting down", "grace", defaultHSSShutdownGrace.String())

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultHSSShutdownGrace)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http server shutdown: %w", err)
	}

	return nil
}

// statusRecorder captures the status code written by the wrapped handler so
// the access log can report it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader records the status code before passing it through.
func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// withHTTPAccessLog wraps next with a middleware that logs one structured
// line per request to the logger.
func withHTTPAccessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).String(),
			"remote", r.RemoteAddr,
		)
	})
}
