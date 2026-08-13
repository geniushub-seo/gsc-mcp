// Command gsc-mcp is a local MCP server for Google Search Console.
// It speaks stdio JSON-RPC. Credentials: ADC (preferred) or service account.
//
//	gsc-mcp                  # run MCP server on stdio
//	gsc-mcp setup [--dry-run]  # check ADC / write MCP client config
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/geniushub-seo/gsc-mcp/internal/config"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"github.com/geniushub-seo/gsc-mcp/internal/setup"
	"github.com/geniushub-seo/gsc-mcp/internal/tools"
)

// version is set by -ldflags at release time. The default "dev" value makes
// local builds identifiable without requiring any build flags during development.
var version = "dev"

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "setup" {
		if err := runSetup(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "gsc-mcp setup: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := runServer(); err != nil {
		fmt.Fprintf(os.Stderr, "gsc-mcp: %v\n", err)
		os.Exit(1)
	}
}

func runSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dryRun := fs.Bool("dry-run", false, "print actions without writing files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	bin, err := filepath.Abs(exe)
	if err != nil {
		return err
	}

	_, err = setup.Run(setup.Options{
		DryRun:     *dryRun,
		BinaryPath: bin,
		Stderr:     os.Stderr,
	})
	return err
}

func runServer() error {
	var (
		credentialsFile    string
		serviceAccountFile string
	)
	flag.StringVar(&credentialsFile, "credentials-file", "", "path to Google credentials JSON (service account or ADC authorized_user)")
	flag.StringVar(&serviceAccountFile, "service-account-file", "", "alias for --credentials-file (compat)")
	flag.Parse()

	path := credentialsFile
	if path == "" {
		path = serviceAccountFile
	}

	cfg, err := config.Resolve(path)
	if err != nil {
		return fmt.Errorf("resolve config: %w", err)
	}

	setupLogging(cfg.LogLevel)
	warnSilentlyIgnoredFlags(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client, err := gscclient.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("create GSC client: %w", err)
	}

	srv := newServer(client, cfg)
	return srv.Run(ctx, &mcp.StdioTransport{})
}

func setupLogging(level slog.Level) {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}

func warnSilentlyIgnoredFlags(cfg config.Config) {
	if cfg.AllowDestructive && !cfg.EnableWrite {
		slog.Warn("GSC_ALLOW_DESTRUCTIVE is ignored because GSC_ENABLE_WRITE is not set; set both env vars to enable destructive actions")
	}
	if cfg.IsADC() && cfg.EnableWrite {
		slog.Warn("GSC_ENABLE_WRITE has no effect with ADC (authorized_user) credentials: OAuth scopes are fixed at gcloud login time and cannot be expanded later. To enable write access, re-run: gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters,https://www.googleapis.com/auth/cloud-platform")
	}
}

func newServer(client *gscclient.Client, cfg config.Config) *mcp.Server {
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "gsc-mcp", Version: version},
		nil,
	)
	tools.Register(srv, client, tools.WriteFlags{
		EnableWrite:      cfg.EnableWrite,
		AllowDestructive: cfg.AllowDestructive,
	})
	return srv
}
