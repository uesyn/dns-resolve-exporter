package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"
)

func MainApp() *cli.App {
	var (
		dnsServer           string
		logLevel            string
		interval            time.Duration
		timeout             time.Duration
		shutdownGracePeriod time.Duration
	)

	return &cli.App{
		Name:            "dns-resolve-exporter",
		Usage:           "Prometheus exporter for dns resolution requests",
		UsageText:       "dns-resolve-exporter [global options] [probe target...]",
		HideHelpCommand: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "server",
				Usage:       "dns server address",
				Required:    true,
				Aliases:     []string{"s"},
				Destination: &dnsServer,
				EnvVars:     []string{"DNS_RESOLVE_EXPORTER_DNSServer"},
			},
			&cli.DurationFlag{
				Name:        "interval",
				Usage:       "probe interval seconds",
				Aliases:     []string{"i"},
				Value:       5 * time.Second,
				Destination: &interval,
				EnvVars:     []string{"DNS_RESOLVE_EXPORTER_PROBE_INTERVAL"},
			},
			&cli.DurationFlag{
				Name:        "timeout",
				Usage:       "probe timeout seconds",
				Value:       5 * time.Second,
				Destination: &timeout,
				EnvVars:     []string{"DNS_RESOLVE_EXPORTER_PROBE_TIMEOUT"},
			},
			&cli.DurationFlag{
				Name:        "shutdown-grace-period",
				Usage:       "the time that server will wait to gracefully shut down",
				Value:       1 * time.Second,
				Destination: &shutdownGracePeriod,
				EnvVars:     []string{"DNS_RESOLVE_EXPORTER_SHUTDOWN_GRACE_PERIOD"},
			},
			&cli.StringFlag{
				Name:        "log-level",
				Usage:       "log level",
				Value:       "info",
				Destination: &logLevel,
				EnvVars:     []string{"DNS_RESOLVE_EXPORTER_LOG_LEVEL"},
			},
		},
		Before: func(cliCtx *cli.Context) error {
			if timeout > interval {
				return fmt.Errorf("interval must be larger than timeout")
			}

			probeTargets := cliCtx.Args().Slice()
			if len(probeTargets) == 0 {
				return fmt.Errorf("must set probe targets")
			}

			if _, err := SetLogLevel(logLevel); err != nil {
				return err
			}
			return nil
		},
		Action: func(cliCtx *cli.Context) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
			defer cancel()

			showBuildInfo()

			registry := prometheus.NewRegistry()
			prober := NewProber(dnsServer, timeout, interval, registry)

			probeTargets := cliCtx.Args().Slice()
			for _, target := range probeTargets {
				go func(target string) {
					prober.Start(ctx, target)
				}(target)
			}

			mux := http.NewServeMux()
			mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(http.StatusText(http.StatusOK)))
			})
			mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-ctx.Done():
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				default:
				}
				w.WriteHeader(http.StatusOK)
			})
			mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

			server := http.Server{
				Addr:    "0.0.0.0:8080",
				Handler: mux,
			}

			go func() {
				<-ctx.Done()
				Logger().Info("shutdown signal received")
				time.Sleep(shutdownGracePeriod)
				if err := server.Shutdown(ctx); err != nil {
					Logger().Errorw("http server shutdown failed", "error", err)
				}
			}()

			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				return err
			}
			return nil
		},
	}
}

func showBuildInfo() {
	info, _ := debug.ReadBuildInfo()
	var gitCommit string
	var dirty bool

	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			gitCommit = s.Value
		}
		if s.Key == "vcs.modified" {
			dirty = s.Value == "true"
		}
	}
	if dirty {
		gitCommit = gitCommit + "-dirty"
	}
	Logger().Infow("build info", "goVersion", info.GoVersion, "gitCommit", gitCommit)
}
