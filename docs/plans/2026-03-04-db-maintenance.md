# DB Maintenance: Configurable + Visible — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the existing daily maintenance loop configurable via env vars and visible via admin API endpoints.

**Architecture:** Add 5 retention duration fields to config, replace hardcoded durations in pipeline.runMaintenance(), store last-run results in a `MaintenanceResult` struct on the pipeline, expose via two new admin endpoints (GET status, POST trigger). The pipeline already implements `LiveDataSource` — we'll add a `MaintenanceStatus()` method to that interface so the admin handler can read results without circular imports.

**Tech Stack:** Go, Chi router, zerolog, caarlos0/env

---

### Task 1: Add retention config fields

**Files:**
- Modify: `internal/config/config.go`
- Modify: `sample.env`

**Step 1: Add retention fields to Config struct**

In `internal/config/config.go`, add after the `PreprocessAudio` field (line 104):

```go
	// Retention / maintenance
	RetentionRawMessages  time.Duration `env:"RETENTION_RAW_MESSAGES" envDefault:"168h"`   // 7d
	RetentionConsoleLogs  time.Duration `env:"RETENTION_CONSOLE_LOGS" envDefault:"720h"`   // 30d
	RetentionPluginStatus time.Duration `env:"RETENTION_PLUGIN_STATUS" envDefault:"720h"`  // 30d
	RetentionCheckpoints  time.Duration `env:"RETENTION_CHECKPOINTS" envDefault:"168h"`    // 7d
	RetentionStaleCalls   time.Duration `env:"RETENTION_STALE_CALLS" envDefault:"1h"`
```

Note: `caarlos0/env` natively parses `time.Duration` strings like `168h`, `720h`, `1h`. Users can also write `168h0m0s`. No custom parser needed. For human-friendly `7d`/`30d` shorthand, we'll add a post-parse step.

**Step 2: Add duration shorthand post-parse in Load()**

After `env.Parse(cfg)` returns (around line 179), add a helper call to expand `d`-suffix values. But since `caarlos0/env` parses before we see the raw strings, and `time.Duration` doesn't support `d`, we need a different approach. Use `env:"" envDefault:""` with a custom string field and parse manually? No — simpler: just document that values must be Go duration strings (`168h` for 7 days). The defaults already use `h`. This is consistent with existing fields like `S3_CACHE_RETENTION=720h`.

Actually, looking at the env library behavior — it will error on `7d`. Let's keep it simple: document hours-based format, defaults are clear. Skip the shorthand.

**Step 3: Add retention section to sample.env**

Append before the Transcription section:

```env
# =============================================================================
# Retention / Maintenance (optional)
# =============================================================================

# Data retention periods for automatic cleanup (runs daily).
# Values are Go duration strings: e.g. 168h = 7 days, 720h = 30 days.

# Raw MQTT message archive (weekly partitions dropped after this)
# RETENTION_RAW_MESSAGES=168h

# Console log messages
# RETENTION_CONSOLE_LOGS=720h

# Plugin status records
# RETENTION_PLUGIN_STATUS=720h

# Active call checkpoints (crash recovery data)
# RETENTION_CHECKPOINTS=168h

# Stale incomplete calls (RECORDING with no audio or call_end)
# RETENTION_STALE_CALLS=1h
```

**Step 4: Commit**

```bash
git add internal/config/config.go sample.env
git commit -m "feat: add configurable retention period env vars"
```

---

### Task 2: Add MaintenanceResult type and interface method

**Files:**
- Modify: `internal/api/live_data.go` (add `MaintenanceStatus()` to `LiveDataSource` interface + result types)

**Step 1: Add maintenance types to live_data.go**

Add after `IngestMetricsData` (around line 179):

```go
// MaintenanceStatusData reports the current maintenance configuration and last run results.
type MaintenanceStatusData struct {
	Config  MaintenanceConfigData `json:"config"`
	LastRun *MaintenanceRunData   `json:"last_run"`
}

// MaintenanceConfigData reports the active retention settings.
type MaintenanceConfigData struct {
	RetentionRawMessages  string `json:"retention_raw_messages"`
	RetentionConsoleLogs  string `json:"retention_console_logs"`
	RetentionPluginStatus string `json:"retention_plugin_status"`
	RetentionCheckpoints  string `json:"retention_checkpoints"`
	RetentionStaleCalls   string `json:"retention_stale_calls"`
	Schedule              string `json:"schedule"`
}

// MaintenanceRunData reports the results of a single maintenance run.
type MaintenanceRunData struct {
	StartedAt  time.Time                    `json:"started_at"`
	DurationMs int64                        `json:"duration_ms"`
	Decimation map[string]DecimationResult  `json:"decimation"`
	Purged     map[string]int64             `json:"purged"`
	PartitionsCreated int                   `json:"partitions_created"`
	PartitionsDropped []string              `json:"partitions_dropped"`
}

// DecimationResult reports rows deleted in each decimation phase.
type DecimationResult struct {
	Phase1Deleted int64 `json:"phase1_deleted"`
	Phase2Deleted int64 `json:"phase2_deleted"`
}
```

**Step 2: Add methods to LiveDataSource interface**

Add to the `LiveDataSource` interface:

```go
	// MaintenanceStatus returns the current maintenance config and last run results.
	MaintenanceStatus() *MaintenanceStatusData

	// RunMaintenance triggers an immediate maintenance run.
	// Returns the results, or an error if maintenance is already running.
	RunMaintenance(ctx context.Context) (*MaintenanceRunData, error)
```

**Step 3: Commit**

```bash
git add internal/api/live_data.go
git commit -m "feat: add MaintenanceStatus types and interface methods"
```

---

### Task 3: Implement maintenance tracking in pipeline

**Files:**
- Modify: `internal/ingest/pipeline.go`

**Step 1: Add fields to Pipeline struct**

Add to the Pipeline struct (after `trInstanceStatus sync.Map`):

```go
	// Maintenance state
	maintenanceRunning atomic.Bool
	lastMaintenance    atomic.Pointer[api.MaintenanceRunData]
	retentionCfg       retentionConfig
```

Add a new struct:

```go
type retentionConfig struct {
	RawMessages  time.Duration
	ConsoleLogs  time.Duration
	PluginStatus time.Duration
	Checkpoints  time.Duration
	StaleCalls   time.Duration
}
```

**Step 2: Add retentionCfg to PipelineOptions and NewPipeline**

Add to `PipelineOptions`:

```go
	RetentionRawMessages  time.Duration
	RetentionConsoleLogs  time.Duration
	RetentionPluginStatus time.Duration
	RetentionCheckpoints  time.Duration
	RetentionStaleCalls   time.Duration
```

In `NewPipeline`, populate `retentionCfg` from opts.

**Step 3: Refactor runMaintenance to use config and track results**

Replace the hardcoded durations (lines 586-588, 599, 608) with `p.retentionCfg` fields. Collect all results into a `MaintenanceRunData` struct. Store it via `p.lastMaintenance.Store(&result)`. Use `p.maintenanceRunning` as a guard.

The refactored `runMaintenance` should:
1. Check/set `maintenanceRunning` (return error if already running)
2. Record `start` time
3. Run all existing steps, collecting counts into the result struct
4. Store result via atomic pointer
5. Clear `maintenanceRunning`

**Step 4: Add MaintenanceStatus() and RunMaintenance() methods**

```go
func (p *Pipeline) MaintenanceStatus() *api.MaintenanceStatusData {
	return &api.MaintenanceStatusData{
		Config: api.MaintenanceConfigData{
			RetentionRawMessages:  p.retentionCfg.RawMessages.String(),
			RetentionConsoleLogs:  p.retentionCfg.ConsoleLogs.String(),
			RetentionPluginStatus: p.retentionCfg.PluginStatus.String(),
			RetentionCheckpoints:  p.retentionCfg.Checkpoints.String(),
			RetentionStaleCalls:   p.retentionCfg.StaleCalls.String(),
			Schedule:              "every 24h",
		},
		LastRun: p.lastMaintenance.Load(),
	}
}

func (p *Pipeline) RunMaintenance(ctx context.Context) (*api.MaintenanceRunData, error) {
	result, err := p.runMaintenanceWithResult()
	if err != nil {
		return nil, err
	}
	return result, nil
}
```

Split `runMaintenance()` into `runMaintenanceWithResult() (*api.MaintenanceRunData, error)` that returns results, and keep `runMaintenance()` as a wrapper that calls it and logs.

**Step 5: Commit**

```bash
git add internal/ingest/pipeline.go
git commit -m "feat: track maintenance results and use configurable retention"
```

---

### Task 4: Wire retention config from main.go to pipeline

**Files:**
- Modify: `cmd/tr-engine/main.go`

**Step 1: Pass retention config to PipelineOptions**

In `main.go` where `ingest.NewPipeline(ingest.PipelineOptions{...})` is called (around line 219), add the retention fields:

```go
		RetentionRawMessages:  cfg.RetentionRawMessages,
		RetentionConsoleLogs:  cfg.RetentionConsoleLogs,
		RetentionPluginStatus: cfg.RetentionPluginStatus,
		RetentionCheckpoints:  cfg.RetentionCheckpoints,
		RetentionStaleCalls:   cfg.RetentionStaleCalls,
```

**Step 2: Commit**

```bash
git add cmd/tr-engine/main.go
git commit -m "feat: wire retention config to pipeline"
```

---

### Task 5: Add admin maintenance endpoints

**Files:**
- Modify: `internal/api/admin.go`
- Modify: `internal/api/server.go`

**Step 1: Extend AdminHandler to accept LiveDataSource**

```go
type AdminHandler struct {
	db            *database.DB
	live          LiveDataSource
	onSystemMerge func(sourceID, targetID int)
}

func NewAdminHandler(db *database.DB, live LiveDataSource, onSystemMerge func(int, int)) *AdminHandler {
	return &AdminHandler{db: db, live: live, onSystemMerge: onSystemMerge}
}
```

**Step 2: Add GET /admin/maintenance handler**

```go
func (h *AdminHandler) GetMaintenance(w http.ResponseWriter, r *http.Request) {
	if h.live == nil {
		WriteError(w, http.StatusServiceUnavailable, "pipeline not running")
		return
	}
	status := h.live.MaintenanceStatus()
	WriteJSON(w, http.StatusOK, status)
}
```

**Step 3: Add POST /admin/maintenance handler**

```go
func (h *AdminHandler) RunMaintenance(w http.ResponseWriter, r *http.Request) {
	if h.live == nil {
		WriteError(w, http.StatusServiceUnavailable, "pipeline not running")
		return
	}
	result, err := h.live.RunMaintenance(r.Context())
	if err != nil {
		WriteError(w, http.StatusConflict, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, result)
}
```

**Step 4: Register routes**

In `AdminHandler.Routes()`:

```go
func (h *AdminHandler) Routes(r chi.Router) {
	r.Post("/admin/systems/merge", h.MergeSystems)
	r.Get("/admin/maintenance", h.GetMaintenance)
	r.Post("/admin/maintenance", h.RunMaintenance)
}
```

**Step 5: Update NewAdminHandler call in server.go**

Change line 153 from:
```go
NewAdminHandler(opts.DB, opts.OnSystemMerge).Routes(r)
```
to:
```go
NewAdminHandler(opts.DB, opts.Live, opts.OnSystemMerge).Routes(r)
```

**Step 6: Commit**

```bash
git add internal/api/admin.go internal/api/server.go
git commit -m "feat: add GET/POST /admin/maintenance endpoints"
```

---

### Task 6: Update OpenAPI spec

**Files:**
- Modify: `openapi.yaml`

**Step 1: Add maintenance endpoints and schemas**

Add two new paths under `/admin/maintenance`:
- `GET` with 200 response returning `MaintenanceStatus` schema
- `POST` with 200 response returning `MaintenanceRun` schema, 409 for already running

Add schemas: `MaintenanceStatus`, `MaintenanceConfig`, `MaintenanceRun`, `DecimationResult`.

All endpoints require bearer auth (same as existing admin endpoints).

**Step 2: Commit**

```bash
git add openapi.yaml
git commit -m "docs: add maintenance endpoints to OpenAPI spec"
```

---

### Task 7: Build and verify

**Step 1: Build**

```bash
bash build.sh
```

Expected: clean build, no errors.

**Step 2: Verify defaults match existing behavior**

The defaults (`168h`, `720h`, `720h`, `168h`, `1h`) should produce identical behavior to the current hardcoded values:
- `168h` = 7 * 24h = 7 days ✓
- `720h` = 30 * 24h = 30 days ✓
- `1h` = 1 hour ✓

**Step 3: Commit (if any fixes needed)**

---

### Task 8: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Add retention env vars to the config documentation section**

In the env-only settings paragraph, add the 5 retention vars with their defaults and descriptions.

**Step 2: Add admin/maintenance to the Implementation Status section**

Under the "Completed" list, update the admin bullet or add a maintenance bullet.

**Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document maintenance API and retention config in CLAUDE.md"
```
