# Architecture: Technical Flow-Through

How tr-engine works from startup to shutdown. For what exists and how to use it, see [CLAUDE.md](../CLAUDE.md).

## 1. Startup Sequence

`cmd/tr-engine/main.go` boots the application in this order:

```
1. Parse CLI flags (--listen, --log-level, --database-url, etc.)
2. Load config: .env file → env vars → CLI overrides (config.Load)
3. TR auto-discovery (if TR_DIR set): read config.json + docker-compose.yaml
4. Validate config (cfg.Validate)
5. Initialize zerolog logger
6. Create shutdown context: signal.NotifyContext(SIGINT, SIGTERM)
7. Connect to PostgreSQL (database.Connect)
8. InitSchema — apply schema.sql on fresh DB (no-op if tables exist)
9. Migrate — run incremental migrations (skip already-applied)
10. Initialize audio storage (storage.New)
11. Start storage background services (pruner, reconciler)
12. Start AsyncUploader if S3 async mode (2 workers, 500 queue)
13. Connect MQTT client (if MQTT_BROKER_URL set)
14. Build transcription provider (if STT_PROVIDER set)
15. Create ingest Pipeline (NewPipeline)
16. Start Pipeline (load identity cache → warmup gate → goroutines)
17. Wire MQTT message handler → Pipeline.HandleMessage
18. Import talkgroup/unit CSVs from TR discovery
19. Start FileWatcher (if WATCH_DIR set)
20. Create HTTP server (api.NewServer) and wire all routes
21. Start HTTP server in background goroutine
22. Log "tr-engine ready"
23. Block on shutdown signal or server error
```

## 2. Configuration Layering

Priority (highest wins): **CLI flags > environment variables > .env file > defaults**

```
main.go                      config.go
┌──────────────────┐         ┌──────────────────────────┐
│ flag.Parse()     │         │ godotenv.Load(envFile)    │
│ overrides struct │───────> │ env.Parse(&Config{})      │
│                  │         │ Apply overrides on top    │
└──────────────────┘         │ cfg.Validate()            │
                             └──────────────────────────┘
```

`config.Load()`:
1. `godotenv.Load(envFile)` — loads `.env` (silent if missing)
2. `env.Parse(&cfg)` — struct tags with `envDefault` provide defaults
3. CLI overrides applied field-by-field (non-empty strings only)
4. If `AUTH_TOKEN` empty and `AUTH_ENABLED=true`, auto-generate random token

Docker Compose uses `${VAR:-default}` interpolation in `docker-compose.yml` so all settings work with zero `.env`.

## 3. Database Lifecycle

### Connect

`database.Connect()` in `internal/database/database.go`:

```
pgxpool.ParseConfig(databaseURL)
  ├── MaxConns = 20
  ├── MinConns = 4
  └── pgxpool.NewWithConfig()
        └── pool.Ping() — fail fast if unreachable
```

Returns `DB{Pool, Q (sqlc queries), log}`.

### InitSchema

`db.InitSchema()` in `internal/database/schema.go`:

```
SELECT EXISTS (SELECT FROM pg_tables
  WHERE schemaname='public' AND tablename='systems')
  │
  ├── true  → no-op (schema already loaded)
  └── false → Exec(embedded schema.sql)
               Creates all 20+ tables, indexes, triggers,
               helper functions, initial partitions
```

### Migrate

`db.Migrate()` in `internal/database/migrations.go`:

```
for each migration in migrations slice:
  ├── run check query → returns true? skip
  └── run SQL (IF NOT EXISTS / IF EXISTS for idempotency)
       └── on failure → return MigrationError with remaining SQL
```

Migrations handle post-`schema.sql` changes (new columns, replaced indexes). Fatal on failure since queries depend on the schema being current.

### Partition Maintenance

Run by `Pipeline.maintenanceLoop()` — once immediately on startup, then every 24h:

1. Create monthly partitions 3 months ahead (calls, call_frequencies, call_transmissions, unit_events, trunking_messages)
2. Create weekly partitions 3 weeks ahead (mqtt_raw_messages)
3. Decimate state tables (recorder_snapshots, decode_rates): full→1/min after 1 week→1/hour after 1 month
4. Purge expired data (console_messages 30d, plugin_statuses 30d, checkpoints 7d)
5. Drop old weekly partitions (mqtt_raw_messages, 7-day retention)
6. Purge stale RECORDING calls (no call_end/audio after 1 hour)
7. Clean orphaned call_groups
8. Expire stale in-memory active calls (>1 hour old)

On-demand partition creation: if an INSERT fails with "no partition found", `ensurePartitionsFor()` creates the needed partition and the caller retries.

## 4. Audio Storage

### Decision Tree

```
storage.New(cfg.S3, audioDir, log)
  │
  ├── S3 not enabled → LocalStore (filesystem only)
  │
  ├── S3 enabled, LocalCache=false → S3Store (S3 only)
  │
  └── S3 enabled, LocalCache=true → TieredStore
        ├── local: LocalStore (source of truth, fast serving)
        ├── s3: S3Store (backup/durability)
        └── background services:
              ├── CachePruner (if retention or maxGB set)
              │     runs every 1h, verifies S3 before deleting local
              └── UploadReconciler
                    runs every 5min (2min initial delay),
                    re-uploads local files missing from S3
```

### Write Paths

Local disk is always written first. S3 failure is never fatal — the reconciler catches it.

```
Sync mode (S3_UPLOAD_MODE=sync or default):
  TieredStore.Save() → local first (fatal), then S3 (warning on failure)

Async mode (S3_UPLOAD_MODE=async):
  TieredStore.SaveLocal() → local disk immediately
  AsyncUploader.Enqueue() → buffered channel (cap 500)
    └── 2 worker goroutines → S3Store.Save() with 30s timeout
        └── on failure: logged, file safe on local disk
```

### Read Path

Local disk first, S3 fallback with cache-on-read. When S3 serves a file that's
missing locally (e.g. after cache pruning), it's saved back to local disk so
subsequent requests hit the fast path.

```
TieredStore.Open()
  ├── local.Open() → found? return local file (fast path)
  └── s3.Open() → fallback to S3
        └── on success: save copy to local disk (best-effort)
              → next request hits local fast path

GetCallAudio (API):
  1. LocalPath → serve directly (fastest)
  2. Open → stream via TieredStore (triggers cache-on-read)
  3. TR_AUDIO_DIR fallback (file watch mode)
```

### LocalStore Safety

All writes use atomic temp-file-then-rename. `safePath()` rejects path traversal attempts.

## 5. Ingest Pipeline

### Construction

`NewPipeline()` in `internal/ingest/pipeline.go` creates:

| Component | Purpose |
|-----------|---------|
| `IdentityResolver` | Maps (instance_id, sys_name) → (system_id, site_id) |
| `activeCallMap` | Tracks in-flight calls: tr_call_id → call metadata |
| `affiliationMap` | Tracks unit→talkgroup affiliations |
| `EventBus(4096)` | SSE pub/sub with ring buffer for replay |
| `rawBatcher` | Batch-inserts mqtt_raw_messages (100 items / 2s) |
| `recorderBatcher` | Batch-inserts recorder_snapshots (100 items / 2s) |
| `trunkingBatcher` | Batch-inserts trunking_messages (100 items / 2s) |
| `transcriber` | Optional WorkerPool for STT (if configured) |

### Start Sequence

`Pipeline.Start()`:

```
1. identity.LoadCache() — pre-populate from DB (all sites)
2. Warmup gate decision:
   ├── cache non-empty → skip warmup (not a fresh DB)
   └── cache empty → activate warmup gate
         buffer non-identity messages for up to 5s
         until system registration establishes sysid/wacn
3. backfillAffiliations() — load recent join events from DB
4. Spawn background goroutines:
   ├── statsLoop (60s interval: log msg counts, active calls)
   ├── maintenanceLoop (24h: partitions, decimation, purges)
   ├── talkgroupStatsLoop (5min: refresh cached TG stats)
   ├── dedupCleanupLoop (10s: sweep expired unit event dedup entries)
   └── affiliationEvictionLoop (5min: evict entries >24h stale)
5. Start transcriber WorkerPool (if configured)
```

### Batcher

Generic `Batcher[T]` in `internal/ingest/batcher.go`:

```
Add(item)
  ├── items >= maxSize? → flush immediately (async goroutine)
  └── first item? → start timer (interval)
        └── timer fires → flush (async goroutine)

Stop() → flush remaining → wg.Wait() for in-flight flushes
```

All three batchers use maxSize=100, interval=2s. Flush functions use `CopyFrom` batch inserts with 10s timeout.

## 6. MQTT Message Flow

### Entry Point

```
mqttclient.Client
  └── SetMessageHandler(pipeline.HandleMessage)
```

### HandleMessage

`pipeline.go:HandleMessage()`:

```
topic + payload arrive
  │
  ├── msgCount.Add(1)
  ├── ParseTopic(topic) → Route{Handler, SysName}
  │     router.go: match on trailing segments only (prefix-agnostic)
  │     ├── .../trunk_recorder/status → "status"
  │     ├── .../call_start → "call_start"
  │     ├── .../{sys_name}/message → "trunking_message"
  │     ├── .../{sys_name}/{event} → "unit_event"
  │     └── nil → unknown topic
  │
  ├── json.Unmarshal → Envelope{InstanceID}
  ├── archiveRaw(handler, topic, payload, instanceID)
  │     check RAW_STORE, RAW_INCLUDE_TOPICS, RAW_EXCLUDE_TOPICS
  │     strip base64 audio data before archival
  │     add to rawBatcher
  │
  ├── UpdateTRInstanceStatus(instanceID, "connected", now)
  │
  └── dispatch(route, topic, payload, env)
        │
        ├── warmup gate check:
        │     if !warmupDone && handler not in {systems,system,config,status}
        │       → buffer message, return
        │
        └── switch route.Handler:
              status → handleStatus
              systems → handleSystems (triggers completeWarmup)
              call_start → handleCallStart
              call_end → handleCallEnd
              recorders → handleRecorders
              unit_event → handleUnitEvent
              trunking_message → handleTrunkingMessage
              console → handleConsoleLog
              ... (14 handlers total)
```

### call_start Walkthrough

```
handleCallStart(payload)
  │
  ├── json.Unmarshal → CallStartMessage
  ├── identity.Resolve(instanceID, sysName)
  │     ├── fast path: RLock → cache hit → return
  │     └── slow path: Lock → double-check → upsert instance
  │           → FindOrCreateSystem → FindOrCreateSite → cache
  │
  ├── db.InsertCall() → call_id (or conflict → update)
  ├── activeCalls.Set(trCallID, entry)
  │
  └── PublishEvent(EventData{Type: "call_start", ...})
        → EventBus.Publish() → ring buffer + subscribers
```

### call_end Walkthrough

```
handleCallEnd(payload)
  │
  ├── json.Unmarshal → CallEndMessage (includes audio metadata)
  ├── identity.Resolve(instanceID, sysName)
  │
  ├── Find active call:
  │     ├── activeCalls.Get(trCallID) — exact match
  │     └── activeCalls.FindByTgidAndTime(tgid, startTime, ±5s)
  │           fuzzy match handles TR's start_time shift
  │
  ├── Save audio (if audio data present):
  │     ├── store.Save(key, data, contentType) — or SaveToCache+Enqueue
  │     └── update call record with audio_file_path
  │
  ├── db.UpdateCallEnd() — set duration, freq_list, src_list, etc.
  ├── activeCalls.Delete(trCallID)
  ├── Assign call_group (dedup across sites)
  ├── Enqueue transcription (if configured, duration in range)
  │
  └── PublishEvent(EventData{Type: "call_end", ...})
```

### Identity Resolution

`internal/ingest/identity.go`:

```
Resolve(ctx, instanceID, sysName)
  │
  ├── Fast path (RLock):
  │     cache[instanceID:sysName] → hit? return immediately
  │     (hot path — most messages resolve here)
  │
  └── Slow path (Lock):
        ├── Double-check cache (another goroutine may have added it)
        ├── UpsertInstance(instanceID) → instance DB ID
        ├── FindOrCreateSystem(instanceID, sysName)
        │     P25: match on (sysid, wacn)
        │     Conventional: match on (instance_id, sys_name)
        │     Creates new system if no match
        ├── FindOrCreateSite(systemID, instanceID, sysName)
        │     Match on (system_id, instance_id, sys_name)
        │     Creates new site if no match
        └── Cache the resolved identity
```

### Raw Archival

```
archiveRaw(handler, topic, payload, instanceID)
  │
  ├── RAW_STORE=false? → return (disabled)
  ├── RAW_INCLUDE_TOPICS set? → allowlist check
  ├── RAW_EXCLUDE_TOPICS set? → denylist check
  ├── handler=="audio"? → stripAudioBase64(payload)
  │     removes audio_m4a_base64 / audio_wav_base64 from JSON
  │     (audio already saved to disk, ~60KB savings per message)
  └── rawBatcher.Add(RawMessageRow{topic, payload, time, instanceID})
```

## 7. File Watch Flow

`internal/ingest/watcher.go` — alternative to MQTT for users without the MQTT plugin.

```
FileWatcher.Start()
  │
  ├── fsnotify.NewWatcher()
  ├── WalkDir(watchDir) — add all existing directories
  ├── spawn watchLoop goroutine
  └── spawn backfill goroutine (if backfillDays >= 0)

watchLoop:
  fsnotify event (Create|Write)
    │
    ├── directory? → addDirRecursive (watch new subdirs, process .json)
    └── .json file? → scheduleProcess(path)
          debounce 500ms (coalesce Create+Write events)
            └── processJSONFile(path)
                  ├── ReadFile → json.Unmarshal → AudioMetadata
                  ├── skip if tgid <= 0
                  └── pipeline.processWatchedFile(instanceID, meta, path)
                        same pipeline as MQTT call_end:
                        identity resolve → insert call → assign group
                        → save audio → transcribe → publish SSE

backfill:
  ├── WalkDir → collect all .json files
  ├── Filter by cutoff (backfillDays)
  ├── Sort oldest-first
  ├── Ensure partitions for full date range
  └── Process with 8 worker goroutines
        progress logged every 5000 files
```

Watch mode only produces `call_end` events (files appear after calls complete). MQTT is the upgrade path for `call_start`, unit events, recorder state, and decode rates.

## 8. HTTP Upload Flow

`internal/api/upload.go` + `internal/ingest/handler_upload.go`

```
POST /api/v1/call-upload
  │
  ├── UploadAuth middleware (accepts Bearer token, ?token=, or form key/api_key)
  ├── MaxBodySize(50 MB)
  │
  ├── ParseMultipartForm
  │     auto-detect format from field names:
  │     ├── "audioFile" field → rdio-scanner format
  │     │     ParseRdioScannerFields(form) → CallUploadData
  │     └── "audio" field → OpenMHz format
  │           ParseOpenMHzFields(form) → CallUploadData
  │
  └── pipeline.ProcessUploadedCall(ctx, data)
        ├── identity.Resolve(uploadInstanceID, sysName)
        ├── Dedup check: FindCallByTgidStartTime
        ├── InsertCall (status=COMPLETED)
        ├── Save audio file (store.Save)
        ├── Assign call_group
        ├── Enqueue transcription
        └── PublishEvent("call_end")
```

## 9. HTTP Request Flow

### Middleware Stack

`internal/api/server.go:NewServer()` — exact order as wired:

```
Global middleware (all routes):
  1. RequestID      — generate/passthrough X-Request-ID header
  2. CORS           — origin check, preflight handling
  3. RateLimiter    — per-IP token bucket (default 20 rps / 40 burst)
  4. Recoverer      — catch panics → JSON 500
  5. Logger         — structured request logging (zerolog/hlog)

Unauthenticated routes (before auth middleware):
  GET /api/v1/health
  GET /metrics (if METRICS_ENABLED)
  GET /api/v1/auth-init (serves read token for web UI)

Upload route group:
  6. MaxBodySize(50 MB)
  7. UploadAuth     — Bearer || ?token= || form key/api_key

Authenticated route group:
  6. MaxBodySize(10 MB)
  7. [InstrumentHandler if metrics enabled]
  8. BearerAuth     — accepts AUTH_TOKEN or WRITE_TOKEN
  9. WriteAuth      — POST/PATCH/PUT/DELETE require WRITE_TOKEN
  10. ResponseTimeout — http.TimeoutHandler (skips SSE + audio)

  All /api/v1/* handler routes mounted here
```

### Auth Model

```
Two-tier token system:
  │
  ├── AUTH_TOKEN (read token)
  │     auto-generated if not set, logged on startup
  │     served to web UI via GET /api/v1/auth-init
  │     gates all API access
  │
  └── WRITE_TOKEN (write token, optional)
        required for POST/PATCH/PUT/DELETE when set
        never exposed by any endpoint
        if not set, AUTH_TOKEN accepted for writes

BearerAuth accepts either token for any request.
WriteAuth additionally checks WRITE_TOKEN for mutating methods.
```

### HTTP Server Config

```go
http.Server{
    Addr:         cfg.HTTPAddr,       // default :8080
    ReadTimeout:  cfg.ReadTimeout,    // default 5s
    IdleTimeout:  cfg.IdleTimeout,    // default 120s
    WriteTimeout: 0,                  // disabled for SSE
}
```

`WriteTimeout=0` allows long-lived SSE connections. Non-streaming handlers are bounded by `ResponseTimeout` middleware (default 30s) and DB query timeouts.

## 10. SSE Event System

### EventBus

`internal/ingest/eventbus.go`:

```
EventBus
  ├── subscribers: map[uint64]subscriber
  │     each has: chan SSEEvent (buffered 64), EventFilter
  ├── ring buffer: []SSEEvent (4096 slots, ~60s at high rate)
  ├── seq: atomic counter for unique event IDs
  └── separate RWMutex for subscribers vs ring buffer
```

### Publish

```
EventBus.Publish(EventData)
  │
  ├── JSON marshal payload
  ├── Generate ID: "{unix_millis}-{seq}"
  ├── Write to ring buffer (ringMu write lock)
  │     ring[ringHead] = event
  │     ringHead = (ringHead + 1) % ringSize
  │
  └── Distribute to subscribers (mu read lock)
        for each subscriber:
          matchesFilter(event, sub.filter)?
            ├── yes → non-blocking send to sub.ch
            │         (drop if subscriber is slow)
            └── no  → skip
```

### Subscribe

```
EventBus.Subscribe(filter)
  → returns (<-chan SSEEvent, cancelFn)
  channel buffered to 64 events
  cancel removes subscriber and closes channel
```

### Filter Logic

`matchesFilter()` — all filter dimensions AND-ed:

```
1. emergency_only: skip non-emergency events
2. types: match event Type (or Type:SubType for compound filters)
   "unit_event" matches all unit events
   "unit_event:call" matches only unit call events
3. systems: match SystemID (zero SystemID passes through)
4. sites: match SiteID (zero SiteID passes through)
5. tgids: match Tgid (zero Tgid passes through)
6. units: match UnitID (zero UnitID passes through)
```

Zero-value fields pass through their filter dimension. This lets events like `recorder_update` (no SystemID) reach subscribers filtering by system.

### Replay (Last-Event-ID)

```
ReplaySince(lastEventID, filter)
  │
  ├── Scan ring buffer from oldest to newest
  │     find lastEventID → return filtered events after it
  │
  └── lastEventID not found (ring wrapped)?
        → return ALL available filtered events
        (better to replay duplicates than miss events)
```

### SSE Handler

`GET /api/v1/events/stream`:

```
1. Parse filter params from query string
2. ReplaySince(Last-Event-ID header) → send missed events
3. Subscribe(filter) → channel
4. Loop:
   ├── event from channel → write SSE frame
   ├── 15s ticker → write ": keepalive" comment
   └── client disconnect → cancel subscription
Headers: Content-Type: text/event-stream, X-Accel-Buffering: no
```

## 11. Shutdown

Triggered by SIGINT or SIGTERM. The `defer` ordering in `main.go` determines teardown sequence (defers execute LIFO):

```
Signal received → ctx.Done()
  │
  ├── srv.Shutdown(10s timeout)      — stop accepting, drain HTTP connections
  │
  ├── pipeline.Stop()                — (defer, runs after HTTP shutdown)
  │     ├── warmupTimer.Stop()
  │     ├── watcher.Stop()           — close fsnotify
  │     ├── transcriber.Stop()       — drain transcription queue
  │     ├── uploader.Stop()          — drain async S3 uploads
  │     ├── rawBatcher.Stop()        — flush + wait
  │     ├── recorderBatcher.Stop()   — flush + wait
  │     ├── trunkingBatcher.Stop()   — flush + wait
  │     └── cancel()                 — cancel pipeline context
  │                                    stops all background goroutines
  │
  ├── mqtt.Close()                   — (defer) disconnect MQTT client
  │
  ├── storage services Stop()        — (defer) stop pruner, reconciler
  │
  └── db.Close()                     — (defer) close pgxpool
```

The 10-second timeout context bounds the entire shutdown. Pipeline batchers flush remaining items before stopping. Background goroutines exit via `<-p.ctx.Done()` checks in their ticker loops.
