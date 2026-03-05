package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	trengine "github.com/snarg/tr-engine"
	"github.com/snarg/tr-engine/internal/config"
	"github.com/snarg/tr-engine/internal/database"
	"github.com/snarg/tr-engine/internal/export"
)

func runExport(args []string, overrides config.Overrides) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	output := fs.String("output", "", "Output file path (required)")
	systems := fs.String("systems", "", "Comma-separated system IDs to export (default: all)")
	fs.StringVar(&overrides.EnvFile, "env-file", overrides.EnvFile, "Path to .env file")
	fs.StringVar(&overrides.DatabaseURL, "database-url", overrides.DatabaseURL, "PostgreSQL connection URL")
	fs.Parse(args)

	if *output == "" {
		fmt.Fprintln(os.Stderr, "error: --output is required")
		fs.Usage()
		os.Exit(1)
	}

	log := zerolog.New(os.Stdout).With().Timestamp().Logger()

	cfg, err := config.Load(overrides)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	db, err := database.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	// Try to apply migrations (non-fatal for export — read-only operation)
	if err := db.InitSchema(ctx, trengine.SchemaSQL); err != nil {
		log.Warn().Err(err).Msg("schema initialization failed (continuing anyway)")
	}
	if err := db.Migrate(ctx); err != nil {
		log.Warn().Err(err).Msg("schema migration failed (some columns may be missing)")
	}

	// Parse system IDs
	var systemIDs []int
	if *systems != "" {
		for _, s := range strings.Split(*systems, ",") {
			id, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil {
				log.Fatal().Str("value", s).Msg("invalid system ID")
			}
			systemIDs = append(systemIDs, id)
		}
	}

	f, err := os.Create(*output)
	if err != nil {
		log.Fatal().Err(err).Str("path", *output).Msg("failed to create output file")
	}
	defer f.Close()

	opts := export.ExportOptions{
		SystemIDs: systemIDs,
		Version:   fmt.Sprintf("%s (commit=%s)", version, commit),
	}

	log.Info().Str("output", *output).Ints("systems", systemIDs).Msg("starting metadata export")

	if err := export.ExportMetadata(ctx, db, f, opts); err != nil {
		os.Remove(*output) // clean up partial file
		log.Fatal().Err(err).Msg("export failed")
	}

	info, _ := f.Stat()
	log.Info().
		Str("output", *output).
		Int64("size_bytes", info.Size()).
		Msg("export complete")
}
