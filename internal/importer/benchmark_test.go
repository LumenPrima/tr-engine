package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trunk-recorder/tr-engine/internal/storage"
	"github.com/trunk-recorder/tr-engine/internal/testutil"
	"go.uber.org/zap"
)

func BenchmarkImporter(b *testing.B) {
	// Setup test DB
	opts := testutil.DefaultTestDBOptions()
	// Use persistent path to avoid init overhead if possible, or just standard opts
	opts.Ephemeral = true

	testDB := testutil.NewTestDB(b, opts)
	defer testDB.Close()

	// Create temp audio dir
	tempDir, err := os.MkdirTemp("", "tr-benchmark")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Generate data - 100 files
	systemName := "benchmark_sys"
	fileCount := 100
	generateTestFiles(b, tempDir, systemName, fileCount)

	logger := zap.NewNop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Clean DB
		testDB.Reset(b)
		// Insert default instance with ID 1
		testDB.MustExec(b, `INSERT INTO instances (id, instance_id, instance_key, config_json, last_seen) VALUES (1, 'default', 'key', '{}', NOW())`)
		// Remove checkpoint file if exists
		os.Remove(".tr-engine-import-checkpoint")

		cfg := Config{
			AudioPath:   tempDir,
			StorageMode: "external",
			BatchSize:   100,
			Throttle:    0, // Unlimited (will be parallel after changes)
		}
		importer := New(testDB.DB, cfg, logger)
		b.StartTimer()

		if err := importer.Run(context.Background()); err != nil {
			b.Fatalf("Import failed: %v", err)
		}
	}
}

func generateTestFiles(t testing.TB, basePath, system string, count int) {
	// Create dir structure: {basePath}/{system}/2023/1/1/
	dir := filepath.Join(basePath, system, "2023", "1", "1")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < count; i++ {
		ts := time.Now().Add(time.Duration(i) * time.Minute).Unix()
		fileBase := fmt.Sprintf("%d_%d", ts, i)

		// Create a sidecar with minimal required fields
		sidecar := storage.AudioSidecar{
			Freq:       850000000,
			StartTime:  ts,
			StopTime:   ts + 10,
			Talkgroup:  1000 + i,
			CallLength: 10,
			SrcList: []struct {
				Src          int64   `json:"src"`
				Time         int64   `json:"time"`
				Pos          float32 `json:"pos"`
				Emergency    int     `json:"emergency"`
				SignalSystem string  `json:"signal_system"`
				Tag          string  `json:"tag"`
			}{
				{Src: int64(2000 + i), Time: ts, Pos: 0},
			},
		}

		data, _ := json.Marshal(sidecar)
		if err := os.WriteFile(filepath.Join(dir, fileBase+".json"), data, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fileBase+".m4a"), []byte("dummy audio"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
