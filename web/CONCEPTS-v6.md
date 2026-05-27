# tr-engine Web Frontend — New Visualization Concepts

> **Scope**: New visualization concepts for tr-engine web frontend  
> **Generated**: 2026-05-10  
> **Status**: Top 5 implemented on 2026-05-10 (concepts #22, #9, #19, #10, #12)

## Implemented Pages

| Concept | Name | File | Status |
|---------|------|------|--------|
| #22 | Talkgroup Bubble Scatter | `bubble-scatter.html` | ✅ Implemented |
| #9 | Daily Overview Treemap | `daily-overview.html` | ✅ Implemented |
| #19 | Recorder Signal Gauges | `recorder-gauges.html` | ✅ Implemented |
| #10 | Talkgroup Activity Sunburst | `tg-sunburst.html` | ✅ Implemented |
| #12 | Daily Overview Calendar Heatmap | `calendar-heatmap.html` | ✅ Implemented |

---

## How This Catalog Was Generated

Cross-checked CATALOG.md (existing visualizations: stream graph, ECharts heatmaps/line charts, Chart.js bar/line/doughnut, D3 timeline, IRC terminal, event ticker, channel grid, audio diagnostics) against the PostgreSQL schema and API surface. Each concept below satisfies:

1. **New technique** not present in any existing page
2. **Leverages existing libraries** already in the codebase (D3.js 7.9.0, ECharts 5.5.0, Chart.js 4.x)
3. **Data is available** from existing endpoints + `POST /query`
4. **Feasible client-side** — aggregation either via SQL or lightweight client processing

---

## Concept 1: Talkgroup Co-activation Network

**Type**: Force-directed graph (network diagram)  
**Library**: D3.js 7.9.0 (already loaded in `stream-graph.html`)  
**Data**: `unit_ids[]` from `calls` table — talkgroups that share unit_ids within the same call

### Description

Visualize which talkgroups are co-activated in the same calls as an interactive force-directed network. Each node represents a talkgroup. Each edge represents two talkgroups being heard together in a call — edge thickness proportional to co-activation frequency. Node size based on total call count.

This reveals the "social graph" of the radio network: which talkgroups form clusters (e.g., fire command + fire engines + fire dispatch), which talkgroups are bridges between distinct clusters, and which talkgroups are isolated.

### Data Query

```sql
-- Top talkgroup co-activations (call it from POST /query)
WITH call_tgs AS (
  SELECT call_id, system_id, tgid
  FROM calls
  WHERE start_time > now() - interval '30 days'
    AND system_id IN (SELECT system_id FROM systems) -- filter if needed
    AND stop_time IS NOT NULL
),
tg_pairs AS (
  SELECT a.tgid AS tgid_a, b.tgid AS tgid_b,
         COUNT(*) AS co_count
  FROM call_tgs a
  JOIN call_tgs b ON a.call_id = b.call_id AND a.tgid < b.tgid
  WHERE EXISTS (
    SELECT 1 FROM calls c2
    WHERE c2.call_id = a.call_id
      AND c2.tgid = b.tgid
      AND c2.system_id = a.system_id
  ) && EXISTS (
    SELECT 1 FROM calls c3
    WHERE c3.call_id = a.call_id
      AND c3.tgid = a.tgid
  )
  GROUP BY a.tgid, b.tgid
  HAVING COUNT(*) >= 3  -- threshold to reduce noise
  ORDER BY co_count DESC
  LIMIT 300
)
SELECT tg_id, alpha_tag, tag, first_seen, last_seen,
       (SELECT COUNT(*) FROM calls WHERE tgid = tg_id AND system_id = ?) AS total_calls
FROM talkgroups
WHERE tgid IN (SELECT tgid_a FROM tg_pairs UNION SELECT tgid_b FROM tg_pairs)
AND system_id = ?
```

**Simpler approach**: For small-to-medium networks, fetch calls with `unit_ids[]` and build the adjacency list entirely client-side.

```sql
-- Fetch raw data for client-side co-activation graph
SELECT call_id, tgid, unit_ids, start_time, stop_time, duration, emergency, encrypted
FROM calls
WHERE start_time > ? AND system_id = ? AND stop_time IS NOT NULL
  AND tgid IN (SELECT tgid FROM talkgroups WHERE system_id = ?)
LIMIT 5000;
```

### Visual Design
- **Nodes**: Colored by system (or talkgroup group like "fire", "police", "fire"), sized by call count
- **Edges**: Semi-transparent, thickness ∝ co-activation count, colored by dominant system overlap
- **Interactivity**: Click to highlight a talkgroup and dim unrelated nodes; hover tooltip shows call count + alpha tag; drag nodes to reposition; scroll to pan, zoom, pinch to zoom
- **Filter controls**: Minimum co-activation threshold (3, 5, 10, 25), max nodes, time window selector

### Feasibility: High
- D3 force simulation is well-established (see `<https://observablehq.com/@d3/force-directed-graph>`)
- Client-side adjacency matrix is O(n²) per call but only for calls with multiple TGs
- For large datasets (>5000 calls), reduce to top-N talkgroups and only their most frequent co-activations
- D3.js already loaded in stream-graph.html

---

## Concept 2: Temporal Activity Hex Heatmap

**Type**: Hexagonal bin heatmap (time-density)  
**Library**: ECharts 5.5.0 (already loaded in `analytics.html`, `traffic-patterns.html`)

### Description

A hexagonal bin heat map showing call activity density across hours of day (y-axis) and days of week (x-axis). Unlike the existing ECharts rectangular heatmap, hex bins naturally encode density and feel more intuitive for temporal data. Each hexagon represents one hour-of-day × day-of-week bucket, colored by call count.

This reveals:
- **Peak hours**: Which 24-hour windows have the most activity
- **Day-of-week patterns**: Weekday vs weekend differences
- **Anomalies**: Unexpected spikes in off-hours

### Data Query

```sql
SELECT
  extract(dow from start_time) AS day_of_week,
  extract(hour from start_time) AS hour_of_day,
  COUNT(*) AS call_count,
  SUM(CASE WHEN emergency THEN 1 ELSE 0 END) AS emergency_count,
  SUM(d duration) / COUNT(*) AS avg_duration
FROM calls
WHERE start_time > now() - interval '90 days'
  AND system_id IN ?
GROUP BY day_of_week, hour_of_day
ORDER BY day_of_week, hour_of_day;
```

### Visual Design
- **Hex grid**: 7 columns (Mon–Sun), 24 rows (0–23 hrs)
- **Color scale**: Viridis or sequential blue (like stream-graph.html)
- **Interaction**: Hover shows call count, emergency count, avg duration
- **Toggle**: Switch between call volume and emergency-only mode
- **Optional**: Overlay a second data series (e.g., 2025 vs 2026 comparison)

### Feasibility: High
- ECharts supports custom hex rendering via `custom` chart type
- Data shape is naturally a 7×24 grid — perfect for heatmap visualization
- Existing `analytics.html` already uses ECharts heatmap for talkgroup density — similar technique
- ~50 lines of JS to adapt

---

## Concept 3: Encryption & Priority Radar

**Type**: Radar/spider chart (multi-metric comparison)  
**Library**: ECharts 5.5.0 (already loaded) or Chart.js 4.x (already loaded)

### Description

A radar chart comparing the last 24 hours of normalized metrics across all systems — one polygon per system, filled with semi-transparent colors. Metrics include: total calls, emergency rate, encryption rate, unique talkgroups active, and avg call duration. Each metric is normalized to 0–100% to make systems comparable.

This gives a single-glance health profile of each monitored radio network — which systems are busiest, which have higher encryption/emergency rates, etc.

### Data (No API changes needed)

```sql
SELECT
  system_id,
  COUNT(*) FILTER (WHERE start_time > now() - interval '24 hours') AS call_count,
  COUNT(*) FILTER (WHERE emergency AND start_time > now() - interval '24 hours')::float /
    NULLIF(COUNT(*) FILTER (WHERE start_time > now() - interval '24 hours'), 0) AS emergency_rate,
  COUNT(*) FILTER (WHERE encrypted AND start_time > now() - interval '24 hours')::float /
    NULLIF(COUNT(*) FILTER (WHERE start_time > now() - interval '24 hours'), 0) AS encryption_rate,
  COUNT(DISTINCT tgid) FILTER (WHERE start_time > now() - interval '24 hours') AS active_tgids,
  AVG(duration) FILTER (WHERE start_time > now() - interval '24 hours') AS avg_duration
FROM calls
WHERE start_time > now() - interval '24 hours'
GROUP BY system_id;
```

### Visual Design
- **Axes**: 5 axes = max_calls, max_emergency_rate, max_encryption_rate, max_active_tgids, max_duration
- **Polygon per system**: Different color per system, semi-transparent fill, dashed stroke for "today's" system, solid for "previous"
- **Legend**: System names with color key
- **Values**: Hover on polygon edges shows actual normalized values + raw data in tooltip

### Feasibility: Very High
- ECharts radar series is built-in and mature (see: ECharts radar demo)
- Data shape is a single row per system — trivial to compute server-side
- ~100 lines of JS
- Can also use Chart.js radar as fallback

---

## Concept 4: Emergency Response Flow Diagram

**Type**: Sankey diagram (flow visualization)  
**Library**: D3.js 7.9.0 + `d3-sankey` (or ECharts sankey built-in)

### Description

A Sankey diagram showing typical unit-to-talkgroup conversation chains for emergency calls. The left side shows the initiating call (the call with `emergency=true`), and the right side shows talkgroups that are subsequently co-activated within 60 seconds after. Edge thickness represents frequency.

This pattern is common across public safety radio:
- **Fire**: Fire dispatch → fire engine(s) → fire command
- **Police**: Police dispatch → patrol units → supervisors (chain varies by incident type)
- **Fire police**: Police dispatch → fire police units (mutual aid)

### Data Query

```sql
-- Emergency response chains: for each emergency call, find co-activated TGs within 60s
SELECT
  emergency_tg.tgid AS origin_tgid,
  response.tgid AS response_tgid,
  COUNT(*) AS response_count
FROM calls emergency_tg
JOIN calls response ON
  emergency_tg.system_id = response.system_id
  AND response.system_id IN (SELECT system_id FROM systems)
  AND response.tgid != emergency_tg.tgid
  AND response.start_time BETWEEN emergency_tg.start_time
    AND emergency_tg.start_time + interval '60 seconds'
WHERE emergency_tg.emergency = true
  AND emergency_tg.stop_time IS NOT NULL
  AND emergency_tg.start_time > now() - interval '30 days'
GROUP BY emergency_tg.tgid, response.tgid
HAVING COUNT(*) >= 2
ORDER BY response_count DESC;
```

### Visual Design
- **Left nodes**: Origin/emergency talkgroups (colored by system)
- **Right nodes**: Responding talkgroups (colored by their own system affiliation)
- **Edges**: Directional, curved paths between origin and response, thickness ∝ response count
- **Labels**: TG alpha_tag or numeric, with count in tooltip
- **Interaction**: Hover on a node to highlight all connected origins; click to filter by originating TG

### Feasibility: Medium-High
- ECharts has built-in Sankey (see: ECharts sankey demo)
- D3.js + d3-sankey addon is well-established
- Query is a self-join — may need LIMIT for performance on large datasets
- Data volume manageable: usually <200 edges for top-50 chains

---

## Concept 5: Signal Quality Timeline

**Type**: Scatter + regression line plot with dual-y  
**Library**: ECharts 5.5.0 (already loaded)

### Description

A timeline scatter plot showing signal strength (`signal_db`) and noise floor (`noise_db`) across calls, colored by time period and filtered by talkgroup. Adding a linear regression line shows degradation or improvement trends over time. A shaded band marks the "usable" signal range (e.g., -60dB to -90dB).

This helps identify:
- **Degraded reception**: A systematic drop in signal_db over weeks/months
- **Noisy conditions**: High noise_db correlating with poor audio quality
- **Seasonal patterns**: Signal drops in winter (weather, foliage) or summer (heat)

### Data (No API changes needed)

```sql
SELECT
  EXTRACT(EPOCH FROM start_time)::bigint / 1000 AS timestamp,
  signal_db,
  noise_db,
  freq,
  freq_error,
  tgid
FROM calls
WHERE start_time > ?
  AND system_id IN ?
  AND signal_db IS NOT NULL
  AND tgid IN (SELECT tgid FROM talkgroups WHERE system_id = ?)
ORDER BY start_time
LIMIT 10000;
```

### Visual Design
- **X-axis**: Time (days/months), formatted with ECharts time axis
- **Y-axis**: Signal strength (dB) — from -120 to -30 dB
- **Scatter points**: Color by TGD (or day-of-week), size by call duration
- **Regression line**: Linear trend overlay
- **Shaded band**: Gray zone for "usable" signal range (user-configurable)
- **Interaction**: Brush selection on chart → filter data by time range; hover tooltip shows signal, noise, freq_error, call duration

### Feasibility: High
- ECharts scatter + line series is built-in, time axis support is mature
- Data shape is flat time series — ECharts handles this natively
- ~70 lines of JS to render with regression
- Can add dual-Y: left axis = signal, right axis = noise

---

## Concept 6: Call Duration Distribution (Interactive Histogram)

**Type**: Overlaid cumulative histogram + ECDF  
**Library**: ECharts 5.5.0 (already loaded)

### Description

An interactive histogram of call durations with multiple overlays: by system color, a cumulative distribution line, and a median/percentile marker. Unlike a simple bar chart, this shows the full distribution shape — which reveals whether calls are typically short bursts (<30s) or sustained transmissions (>5min).

### Data (No API changes needed)

```sql
SELECT
  system_id,
  -- bin durations into buckets: 0-30s, 30-60s, 60-120s, 2-5min, 5-10min, 10-30min, 30-60min, 60min+
  CASE
    WHEN duration <= 30 THEN 1
    WHEN duration <= 60 THEN 2
    WHEN duration <= 120 THEN 3
    WHEN duration <= 300 THEN 4
    WHEN duration <= 600 THEN 5
    WHEN duration <= 1800 THEN 6
    WHEN duration <= 3600 THEN 7
    ELSE 8
  END AS duration_bucket,
  COUNT(*) AS call_count
FROM calls
WHERE start_time > ?
  AND duration IS NOT NULL
  AND system_id IN ?
GROUP BY system_id, duration_bucket
ORDER BY duration_bucket;
```

### Visual Design
- **Bar chart**: Stacked or overlaid by system color
- **Cumulative line**: Overlay showing cumulative % of calls below each bucket
- **Median markers**: Vertical lines at 50th / 75th / 90th percentile
- **Interactive controls**: Toggle systems on/off, switch log scale, filter by emergency only
- **Tooltip**: Shows exact count + % per bucket per system

### Feasibility: Very High
- Chart.js has horizontal bar charts with stacked support
- ECharts has stacked bar + line combo
- Data shape is a simple 8-bucket aggregation — trivial
- ~60 lines of JS

---

## Summary Matrix

| # | Name | Technique | Library | Data Needed | Feasibility | API Changes |
|---|------|-----------|---------|-------------|-------------|-------------|
| 1 | Talkgroup Co-activation Network | Force-directed graph | D3.js | unit_ids[] + calls | High | None |
| 2 | Temporal Activity Hex Heatmap | Hex bin heatmap | ECharts | Calls by hour/day | High | None |
| 3 | Emergency Response Flow Diagram | Sankey diagram | D3 + d3-sankey | Emergency calls + 60s overlap | Medium-High | None |
| 4 | Encryption & Priority Radar | Radar chart | ECharts or Chart.js | Daily call stats | Very High | None |
| 5 | Signal Quality Timeline | Scatter with regression | ECharts | signal_db, noise_db | High | None |
| 6 | Call Duration Distribution | Overlaid histogram + ECDF | ECharts or Chart.js | Duration stats | Very High | None |

All 6 concepts:
- **Do not exist** in any current `web/` page
- **Use existing libraries** already loaded in the codebase
- **Work with existing API** + `POST /query` for custom SQL
- **Are feasible** in 1–2 days each (concept to working prototype)

---

## Recommendations (Top 3 to Implement First)

1. **#3 — Encryption & Priority Radar**: Fastest to build (~100 lines), no complex queries, high analytical value for monitoring multiple systems at a glance
2. **#2 — Temporal Activity Hex Heatmap**: Medium build (~50 lines), reveals important temporal patterns not visible in current analytics charts
3. **#1 — Talkgroup Co-activation Network**: Most "wow" factor but longest build (client-side aggregation), ideal for deep investigation of network structure
