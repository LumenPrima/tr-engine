# tr-engine Web Frontend — New Visualization Concepts (Batch 2)

> **Scope**: 20 new visualization concepts beyond the first 6  
> **Generated**: 2026-05-10  
> **Status**: Concept review phase — no implementation

## How These Were Generated

- **Existing visualizations avoided**: D3 stream graph, ECharts heatmap/line/area, Chart.js bar/line/doughnut, Drift timeline, IRC terminal, event ticker, channel grid, audio diagnostics
- **Research targets**: Parallel coordinates, scatterplot matrices, treemaps/sunburst, calendar heatmaps, word clouds, analog gauges, chord diagrams, Sankey, radar charts, waterfall, ridgeline, bubble charts, horizon charts, barchart race, treemap scatter, slope charts, dot plots, stacked area, streamgraph (already exists), treemap (D3 built-in)
- **Constraints**: Existing libraries only (D3.js 7.9.0, ECharts 5.5.0, Chart.js 4.x), existing API data, feasible client-server

---

## Concept 7: Call Metrics Parallel Coordinates

**Type**: Parallel coordinates plot (multidimensional)  
**Library**: D3.js 7.9.0 + `d3.parcoords` plugin  
**Data Needed**: Calls with `duration`, `signal_db`, `noise_db`, `freq`, `freq_error`, `error_count`, `spike_count`, `encrypted`, `emergency`

### Description

A parallel coordinates chart where each vertical axis represents a call metric, and each call is a single line traversing all axes. Unlike scatter plots that only show 2D relationships, parallel coordinates show ALL metrics simultaneously. This lets operators spot patterns across a dozen metrics at once:

- Calls with high signal + low noise → healthy reception
- Calls with high error_count + high spike_count → interference
- Long calls with low signal → degraded channel
- Encrypted calls clustering together by metric pattern

### Data Query

```sql
SELECT call_id, start_time, stop_time, duration, signal_db, noise_db,
       freq, freq_error, error_count, spike_count, encrypted, emergency, tgid
FROM calls
WHERE start_time > now() - interval '7 days'
  AND system_id IN (?)
  AND stop_time IS NOT NULL
  AND duration IS NOT NULL
LIMIT 2000;
```

### Visual Design
- **Axes**: duration, signal_db, noise_db, freq, freq_error, error_count, spike_count (8 parallel vertical axes, orderable and draggable)
- **Lines**: Each call = one line, colored by `emergency` (red vs blue) or `encrypted` (purple vs gray)
- **Brushing**: Click-drag on any axis to filter lines by value range; brushing on one axis highlights matching lines on all others
- **Bundling**: Lines bunched by group to reduce visual clutter at scale
- **Stats coloring**: Hover axis → color by Z-score for that metric
- **Progressive rendering**: Queue mode for >2000 calls

### Feasibility: Medium
- Requires external `d3.parcoords` plugin (~8KB)
- `d3.parcoords` is legacy (d3 v3) but working D3 v5+ ports exist on NPM/GitHub
- Alternatively: custom parallel coords without plugin (see https://observablehq.com/@d3/parallel-coordinates by Mike Bostock — ~200 LOC)
- ~200 lines of JS with built-in d3.parcoords approach
- ~50 lines of JS with ObservableHQ custom approach

---

## Concept 8: Signal Quality Scatterplot Matrix

**Type**: Scatterplot matrix (SPLOM) — pairwise correlations  
**Library**: D3.js 7.9.0  
**Data**: Calls with `signal_db`, `noise_db`, `freq_error`, `duration`, `start_time`, `emergency`

### Description

A grid of scatterplots showing every pairwise relationship between key signal metrics. The diagonal shows histograms; off-diagonal cells show x vs y scatterplots. Brushing in any cell highlights the same calls across ALL other cells.

This reveals hidden correlations operators can't see one at a time:
- Does `signal_db` drop correlate with `duration` increase? (interference on long calls)
- Does `freq_error` cluster with `error_count`? (reception quality)
- Do emergency calls cluster in a different region than non-emergency?

### Data (No API changes needed)

Same query as Concept 7. Each call becomes a point in every pairwise scatterplot.

### Visual Design
- **Grid**: 6×6 matrix = 36 cells (or 4×4 for 4 metrics to start)
- **Diagonal**: Histograms per metric (duration, signal_db, noise_db, freq_error)
- **Off-diagonal**: Scatterplots with semi-transparent points (brushable)
- **Highlighting**: Brush on ANY cell → same calls highlighted dim across ALL cells
- **Color**: Red for emergency, blue for normal

### Feasibility: Medium-High
- D3 scatterplot matrix exists (see https://observablehq.com/@d3/splom by D3 team)
- ~300 LOC for full matrix with brushing
- ~100 LOC for simplified 4×4 matrix
- Data volume manageable: 5000 calls × 6 metrics = 30000 points

---

## Concept 9: Daily Overview Treemap

**Type**: Treemap (area-proportional hierarchy)  
**Library**: D3.js 7.9.0 (`d3.treemap`)  
**Data**: Calls grouped by system → talkgroup → time-of-day bucket

### Description

A treemap where each rectangle's area represents call count, organized hierarchically: systems as top-level groups, talkgroups as subgroups, and time-of-day buckets as leaves. The size of each leaf rectangle is proportional to the number of calls in that TG during that hour.

Unlike the current analytics page (which uses a line chart), a treemap immediately shows which talkgroups dominate across all time — large rectangles = bus stops. Hovering shows exact call counts, emergency rates, etc.

### Data Query

```sql
SELECT system_id, tgid,
       date_trunc('hour', start_time) AS hour_bucket,
       COUNT(*) AS call_count,
       COUNT(*) FILTER (WHERE emergency) AS emergency_count,
       AVG(duration) AS avg_duration
FROM calls
WHERE start_time > now() - interval '90 days'
  AND stop_time IS NOT NULL
GROUP BY system_id, tgid, date_trunc('hour', start_time)
ORDER BY call_count DESC;
```

### Visual Design
- **Hierarchy**: Systems → Talkgroups → Hour buckets (3 levels, depth=2)
- **Colors**: Viridis sequential or custom per-system
- **Size**: Call count → rectangle area
- **Hover tooltip**: Call count, emergency rate, avg duration, alpha tag
- **Time filter**: Filter treemap to last 24h / 7 days / 30 days / 90 days

### Feasibility: Very High
- `d3.treemap` is built into D3.js core (part of `d3-hierarchy`)
- ~80 lines of JS for basic treemap
- `d3.treemapResquarify` for smooth animated transitions

---

## Concept 10: Talkgroup Activity Sunburst

**Type**: Radial sunburst (hierarchical circle chart)  
**Library**: ECharts 5.5.0 (built-in sunburst) or D3.js `d3.treemap`/`d3.sunburst`  
**Data**: Calls grouped by system → talkgroup → hour of day

### Description

A sunburst chart where the concentric rings are organized hierarchically: center ring = systems, middle ring = talkgroups, outer ring = hourly activity. Unlike a traditional pie chart, the sunburst shows THREE levels of hierarchy simultaneously. The arc width of each segment is proportional to the number of calls.

This is ideal for a "big picture" view — instantly shows which system dominates, which TGs within it are busiest, and how that activity distributes across hours.

### Data Query

```sql
SELECT system_id, tgid, tgid_alpha_tag,
       date_trunc('hour', start_time) AS hour_bucket,
       COUNT(*) AS call_count
FROM calls
WHERE start_time > now() - interval '30 days' AND stop_time IS NOT NULL
GROUP BY system_id, tgid, hour_bucket
ORDER BY call_count DESC;
```

### Visual Design
- **Center**: Systems (4-6 systems = 4-6 segments)
- **Middle**: Talkgroups within each system (arc width ∝ call count)
- **Outer**: Hourly activity distribution within each TG (24 segments per TG)
- **Interaction**: Click a segment to drill in (show just that hierarchy branch); hover to see details
- **Time control**: Filter to today / 7d / 30d / 90d

### Feasibility: Very High
- ECharts has built-in sunburst (see ECharts sunburst docs)
- D3.js has `d3.treemap` or `d3.sunburst` addon
- ~80-120 lines of JS with ECharts sunburst

---

## Concept 11: Talkgroup Correlation Matrix

**Type**: Correlation + scatter plot matrix  
**Library**: ECharts 5.5.0  
**Data**: Daily call counts per talkgroup across all days

### Description

A dual-panel visualization: left panel shows a correlation heatmap matrix (which talkgroups have correlated activity), right panel shows the corresponding scatter plot for the selected correlation cell. This reveals hidden dependencies:

- Fire dispatch ↔ fire engines (naturally correlated)
- Police dispatch ↔ patrol units
- Mutual aid: two unrelated systems that share TGs
- Time-of-day patterns: TGs that peak at the same times

### Data Query

```sql
-- For each talkgroup, count calls per day across 30 days
SELECT tgid, date(start_time) AS activity_date, COUNT(*) AS daily_count
FROM calls
WHERE start_time > now() - interval '30 days'
  AND stop_time IS NOT NULL
  AND system_id IN (?)
GROUP BY tgid, date(start_time)
ORDER BY tgid, activity_date;
```

### Visual Design
- **Left**: Correlation heatmap — blue = positive correlation, red = negative, cells clickable
- **Right**: Scatter plot showing actual data for the selected cell pair
- **Brushing**: Click a cell → right panel populates with that pair's scatter plot
- **Color**: Correlation values mapped to diverging color scale (blue → white → red)

### Feasibility: High
- ECharts matrix coordinate system for correlation heatmap
- ~150 LOC with ECharts built-in chart types
- Cross-panel interaction via `on('click')` + `setOption()` update

---

## Concept 12: Daily Call Volume Calendar Heatmap

**Type**: Calendar heatmap (GitHub-style contribution graph)  
**Library**: ECharts 5.5.0 (`calendar` + `heatmap`)  
**Data**: Calls per day (aggregated)

### Description

A GitHub-style contribution calendar showing daily call volume across a year. Each cell = one day, colored by call count. Unlike the existing ECharts heatmap (which is hour×day-of-week), this shows actual dates — useful for spotting seasonal trends, holidays, anomalies.

### Data Query

```sql
SELECT date_trunc('day', start_time)::date AS activity_date, COUNT(*) AS call_count
FROM calls
WHERE start_time > now() - interval '1 year'
  AND system_id IN (?)
GROUP BY activity_date
ORDER BY activity_date;
```

### Visual Design
- **Layout**: ECharts calendar component with vertical orientation
- **Color**: Viridis or RdYlBu diverging scale
- **Hover**: Show exact date + call count + emergency count for that day
- **Multiple years**: Up to 2 years visible simultaneously
- **Toggle**: Show total calls, emergency-only, or encryption-only

### Feasibility: Very High
- ECharts `calendar` coordinate system + `heatmap` series is built-in
- ~50 lines of JS
- Same pattern as ECharts calendar-heatmap example

---

## Concept 13: Signal Strength Time Scroller (Waterfall)

**Type**: Waterfall display (time-frequency spectrogram)  
**Library**: D3.js 7.9.0 + `d3-waterfall` (or canvas-based custom)  
**Data**: Calls with `freq`, `signal_db`, `noise_db`, `start_time`, `stop_time`

### Description

A waterfall (spectrogram) display where the x-axis is time, the y-axis is frequency, and color represents signal strength. Each call is a horizontal "blip" on the time-frequency plane. This is the classic RF visualization everyone associates with spectrum monitoring.

Unlike the existing signal quality timeline (scatter per call), this creates a continuous time-frequency canvas — operators can see the "shape" of activity across the spectrum over the entire day.

### Data (No API changes needed)

```sql
SELECT freq, signal_db, start_time, stop_time, tgid,
       EXTRACT(EPOCH FROM start_time)::bigint / 1000 AS timestamp
FROM calls
WHERE start_time > now() - interval '24 hours'
  AND freq IS NOT NULL AND signal_db IS NOT NULL
  AND system_id IN (?)
ORDER BY start_time;
```

### Visual Design
- **Canvas rendering**: X-axis = time (scrolling), Y-axis = frequency (log scale)
- **Color**: Heat map gradient (black = no signal → blue/green → yellow → white = strong)
- **Frequency bands**: Vertical lines marking known TG frequencies
- **Pan/zoom**: Horizontal pan through time, vertical zoom on frequency range
- **Tooltips**: Hover shows call details at that frequency/time
- **Time control**: Slider at top to scrub through the 24h window

### Feasibility: Medium
- `d3-waterfall` (GitHub: ddcc/d3-waterfall) handles this pattern out of the box — supports RTL-SDR CSV, canvas rendering
- Alternatively: custom canvas rendering with D3 scales (see https://rye.dev/blog/electromagnetic-spectrum-explorer/)
- ~200 LOC with d3-waterfall, ~400 LOC custom

---

## Concept 14: Emergency Frequency Breakdown

**Type**: Stacked area chart (cumulative emergency calls by type/time)  
**Library**: ECharts 5.5.0 (built-in area series)  
**Data**: Emergency calls by minute-hour, grouped by system

### Description

A stacked area chart where the y-axis shows emergency call rate (calls per hour), x-axis shows time in hours. Each layer is a different system with the area filled in different colors (using the viridis/sequential color scheme like existing stream-graph.html).

This reveals overlapping emergency situations — when multiple systems have high emergency rates simultaneously.

### Data Query

```sql
SELECT DATE_TRUNC('hour', start_time) AS hour_bucket,
       system_id, COUNT(*) AS emergency_count
FROM calls
WHERE start_time > now() - interval '48 hours'
  AND emergency = true AND stop_time IS NOT NULL
  AND system_id IN (?)
GROUP BY hour_bucket, system_id
ORDER BY hour_bucket, system_id;
```

### Visual Design
- **Stacked area**: ECharts stack area chart, one series per system
- **Colors**: Viridis or colorblind-friendly palette
- **Smooth step**: `step: 'middle'` for discrete hourly buckets
- **Hover**: Show exact count for each system + emergency rate
- **Toggle**: Show all systems or filter by specific ones

### Feasibility: Very High
- ECharts built-in area series with stacking
- ~40 lines of JS
- Same pattern as existing stream-graph.html color approach

---

## Concept 15: Transmission Duration Dot Plot

**Type**: Dot plot (horizontal) + jitter  
**Library**: Chart.js 4.x  
**Data**: Call durations per talkgroup

### Description

A dot plot where each dot represents a single call, positioned by duration on the x-axis and by time-of-day on the y-axis. Multiple TGs are overlaid with small jitter to prevent overlap. Unlike a histogram (which aggregates), this shows every individual call.

This reveals:
- **Distribution shape**: Are all calls short, or is there a long-tail?
- **Duration by time**: Are long calls more common at night (shift changes) or daytime?
- **TG comparison**: Which talkgroups tend to have very long vs very short calls?

### Data (No API changes needed)

```sql
SELECT call_id, tgid, tgid_alpha_tag, duration,
       EXTRACT(HOUR FROM start_time) AS time_bucket,
       date(start_time) AS activity_date
FROM calls
WHERE start_time > now() - interval '7 days'
  AND duration IS NOT NULL
  AND stop_time IS NOT NULL
  AND system_id IN (?)
ORDER BY start_time;
```

### Visual Design
- **X-axis**: Duration (seconds), log scale for wide range
- **Y-axis**: Time of day (0-23), with TG labels
- **Points**: Colored by TG (with jitter to prevent overlap)
- **Density**: Use opacity (alpha blending) for overlapping dots
- **Filter**: Dropdown to select top 20 TGs by volume

### Feasibility: Medium
- Chart.js scatter/horizontal scatter with custom point styling
- ~100 LOC
- Alternative: D3.js dot plot (see https://observablehq.com/@d3/dotplot)

---

## Concept 16: Transcription Word Cloud

**Type**: Word cloud (frequency-based typography)  
**Library**: D3.js 7.9.0 + `d3-cloud` (d3.layout.cloud by Jason Davies)  
**Data**: Calls with `transcription_text` populated

### Description

A dynamic word cloud where each word represents a term found in call transcriptions, sized by frequency. Unlike static word clouds, this is interactive and filterable—operators can see what words are most common in emergency vs routine calls.

This is useful for identifying recurring terminology:
- Most common words across ALL transcriptions
- Emergency-specific terms (fire, traffic, officer down, backup)
- System-specific jargon
- Word frequency by time of day

### Data Query

```sql
SELECT transcription_text
FROM calls
WHERE transcription_text IS NOT NULL
  AND transcription_text != ''
  AND start_time > now() - interval '30 days'
  AND system_id IN (?)
LIMIT 5000;
```

Client-side: tokenized, stop-word-filtered, stemmed.

### Visual Design
- **Layout**: D3 word cloud layout (https://github.com/jasondavies/d3-cloud)
- **Size**: Log-scaled by frequency across transcription corpus
- **Rotation**: Mixed orientations (default ±30°) for visual interest
- **Colors**: Category20b color scale (D3 built-in)
- **Time filter**: Last 24h, 7d, 30d, 90d
- **Emergency toggle**: Word cloud filtered to only emergency calls

### Feasibility: Medium
- `d3-cloud` is a well-established D3 addon (https://github.com/jasondavies/d3-cloud, 3KB minified)
- ~150 LOC for interactive word cloud with filters
- Stop words list needed (common English words to exclude)

---

## Concept 17: Frequency Spectrum Treemap

**Type**: Treemap (spatial frequency allocation view)  
**Library**: D3.js 7.9.0 (`d3.treemap`)  
**Data**: Calls with `freq`, `tgid`, `start_time`

### Description

A frequency-based treemap where each node represents a talkgroup (by TGD), sized by call count, positioned according to frequency allocation. Unlike the existing Treemap (talkgroup-research.html), this is organized by **frequency band** rather than by talkgroup type.

The bands (P25 trunked, analog FM, P25 Phase 2, conventional) become the parent hierarchy, with talkgroups as leaves. This shows who owns which "slice" of the spectrum.

The user can also switch to a different treemap view: Systems → TGs → Hours (activity-based).

### Data Query

```sql
SELECT system_id, 
       tgid,
       tg_alpha_tag,
       freq,
       analog,
       phase2_tdma,
       COUNT(*) AS call_count
FROM calls
WHERE start_time > now() - interval '30 days'
  AND stop_time IS NOT NULL
  AND tgid IN (SELECT tgid FROM talkgroups WHERE system_id = ?)
GROUP BY system_id, tgid, tg_alpha_tag, freq, analog, phase2_tdma;
```

### Visual Design
- **First-level** (parent): Frequency band (P25, Analog, SmartNet, Conventional)
- **Second-level** (middle): Systems within each band
- **Leaves**: Talkgroups sized by call count, colored by TG category (fire, police, etc.)
- **Hover tooltip**: TG name, total calls, emergency rate, avg duration, frequency
- **View switcher**: Toggle between "by band", "by system", "by TG group"
- **Clickable**: Click a treemap node to navigate deeper

### Feasibility: Medium-High
- D3 treemap with click-to-drill-down (~200 LOC)
- Pre-computed frequency bands from schema (P25 analog, P25 TDMA, conventional)
- `d3.treemapResquarify` for smooth transitions

---

## Concept 18: Unit Call Correlation Matrix

**Type**: Matrix heatmap (unit-to-TG correlation)  
**Library**: ECharts 5.5.0 (`matrix` coordinate system)  
**Data**: Calls with `unit_ids[]`, grouped by TG and unit

### Description

A matrix heatmap where rows = talkgroups and columns = units (or vice versa). Each cell shows how many times that unit was heard on that TG during calls. Bright cells = strong affiliation, dim cells = weak or occasional.

This reveals unit-to-TG affiliation patterns that the current API doesn't surface:
- Which units consistently affiliate to which TGs?
- Which units appear on multiple TGs (cross-system)?
- Time-varying patterns: which units only appear on TGs during certain hours?

### Data (No API changes needed)

Client-side aggregation from calls endpoint with `unit_ids[]`.

```sql
SELECT DISTINCT tgid, unit_id
FROM calls, unnest(unit_ids) AS unit_id
WHERE start_time > now() - interval '90 days'
  AND unit_ids IS NOT NULL AND array_length(unit_ids, 1) > 0
  AND system_id IN (?)
ORDER BY tgid, unit_id;
```

### Visual Design
- **Matrix grid**: Rows = TGs (sorted by call count), Columns = units
- **Color**: Sequential (viridis) with cell brightness = call count
- **Hover**: Show TG + unit names + affiliation count
- **Brushing**: Select a row/column → highlight across matrix
- **Filter**: Limit to top 50 TGs and top 100 units (configurable)

### Feasibility: Medium
- ECharts `matrix` coordinate system (see ECharts 5.5 matrix heatmap docs)
- ~100 LOC with ECharts matrix
- Data volume: 50 rows × 100 cols = 5000 cells (well within ECharts limits)

---

## Concept 19: Recorder Signal Gauge

**Type**: Animated analog + semi-circle gauge (multiple gauges)  
**Library**: ECharts 5.5.0 (linear gauges)  
**Data**: Recorder `signal_db`, `noise_db` from `recorder_snapshots`

### Description

A dashboard of circular/semi-circular gauges — one per recorder site. Each gauge shows a key metric: active TGs (center dial), signal strength (arc sweep), noise level (secondary arc), and call rate (outer ring).

This is the "cockpit dashboard" view — operators can see the health of all recorder sites at a glance, like instrument panel readings.

### Data (No API changes needed)

Existing `/recorders` endpoint + `recorder_snapshots` table.

### Visual Design
- **Per-recorder gauge**:
  - Center: active talkgroups (dial + numeric readout)
  - Left arc: average signal strength (dB scale, red→green)
  - Right arc: call rate (last hour vs last 24h)
- **Layout**: Grid of 4-8 gauges (one per recorder system), responsive
- **Color coding**: Green (> -60dB), Yellow (-60 to -80dB), Red (< -80dB)
- **Live update**: SSE-driven re-render when recorder state changes
- **Click**: Expand a gauge to show a full detail panel (with mini time-series of signal)

### Feasibility: High
- ECharts linear gauge chart built-in (https://echarts.apache.org/examples/en/editor.html?c=gauge-line)
- Or D3.js custom gauge (see https://observablehq.com/@d3/gauge)
- ~150 LOC for 4-gauge dashboard with SVG + D3

---

## Concept 20: Slope Chart of TG Activity Trends

**Type**: Slope chart (before/after trend comparison)  
**Library**: Chart.js 4.x or D3.js  
**Data**: Talkgroup activity across two time periods

### Description

A slope chart (also called a bump chart) comparing talkgroup activity between two time periods. Each TG is a line connecting its call count in Period 1 to its call count in Period 2. The vertical position of each line's endpoints represents the ranked activity level. Lines that slope upward = TGs becoming more active; downward = declining.

This is ideal for "what's changed since..." questions:
- Which TGs spiked during a specific event?
- Which TGs went quiet?
- Seasonal shifts in activity patterns

### Data (No API changes needed)

```sql
-- Period 1: Last 30 days
SELECT tgid, COUNT(*) AS call_count
FROM calls
WHERE start_time > now() - interval '60 days'
  AND start_time < now() - interval '30 days'
  AND stop_time IS NOT NULL
  AND system_id IN (?)
GROUP BY tgid

-- Period 2: Last 30 days
SELECT tgid, COUNT(*) AS call_count
FROM calls
WHERE start_time > now() - interval '30 days'
  AND stop_time IS NOT NULL
  AND system_id IN (?)
GROUP BY tgid
```

### Visual Design
- **X-axis**: Period 1 (left), Period 2 (right)
- **Y-axis**: Log-scaled call count
- **Lines**: Each TG = one connecting line, colored by system
- **Sorting**: Lines sorted by change (most increased at top)
- **Hover**: Show TG name, both periods' counts, % change

### Feasibility: Very High
- Chart.js custom plugin or D3.js simple line drawing (~60 LOC)
- Minimal data transformation required

---

## Concept 21: Call Event Ridge Plot

**Type**: Ridgeline plot (density plot with jittered overlapping curves)  
**Library**: ECharts 5.5.0 (custom) or D3.js + `d3-density` plugin  
**Data**: Call durations per talkgroup or system

### Description

A ridge plot (also called a joyplot) showing the distribution of call durations for each talkgroup. Each talkgroup gets its own density curve, stacked vertically. Unlike a bar chart, a ridgeline shows the FULL distribution shape — unimodal, bimodal, long-tailed.

This reveals distribution patterns that a simple bar chart can't:
- Some talkgroups have bimodal distributions (short + long calls)
- Some have heavy tails (rare but very long calls)
- Distribution changes over time (comparing last 30d vs previous 30d)

### Data (No API changes needed)

```sql
SELECT tgid, tgid_alpha_tag, duration, date(start_time) AS activity_day
FROM calls
WHERE start_time > now() - interval '60 days'
  AND duration IS NOT NULL
  AND stop_time IS NOT NULL
  AND system_id IN (?)
ORDER BY start_time;
```

### Visual Design
- **Rows**: Each TG = one density curve, stacked vertically (sorted by total calls)
- **Curves**: KDE-smoothed density function
- **Color**: Per-system color with gradient fill (fades to transparent on the right)
- **Hover**: Cursor follows curve → shows duration + density value
- **Time filter**: Toggle between 30d and 60d views to compare

### Feasibility: Medium
- D3.js with `d3.curve` + custom KDE computation (~200 LOC)
- D3 ridgeline examples exist in Observable (https://observablehq.com/@d3/ridgeline-plot)
- ECharts custom series could render this but requires more setup

---

## Concept 22: Talkgroup Bubble Scatter

**Type**: Bubble chart (3D scatter with size = area)  
**Library**: ECharts 5.5.0 (built-in scatter)  
**Data**: Calls per TG, with emergency rate and avg duration

### Description

A bubble scatter plot where each bubble represents a talkgroup. The bubble's position is determined by:
- **X-axis**: Total call count in time period
- **Y-axis**: Emergency rate (%)
- **Bubble size**: Average call duration (log scale)
- **Bubble color**: System affiliation

This gives an immediate "at a glance" ranking of talkgroups by all three metrics simultaneously — which TGs are busiest, which have the highest emergency rate, which have the longest calls.

### Data (No API changes needed)

```sql
SELECT tgid, tg_alpha_tag,
       COUNT(*) AS call_count,
       COUNT(*) FILTER (WHERE emergency)::float / NULLIF(COUNT(*), 0) * 100 AS emergency_rate,
       AVG(duration) AS avg_duration_s,
       system_id
FROM calls
WHERE start_time > now() - interval '30 days'
  AND stop_time IS NOT NULL
  AND system_id IN (?)
GROUP BY tgid, tg_alpha_tag, system_id
ORDER BY call_count DESC;
```

### Visual Design
- **X-axis**: Call count (log scale, 1 call to 1000+)
- **Y-axis**: Emergency rate (0% to 100%)
- **Bubble size**: Avg call duration (0–30 min), mapped to radius
- **Color**: System-based color palette
- **Hover tooltip**: TG name, alpha tag, call count, emergency %, avg duration
- **Filter**: Show top N bubbles, or filter by TG category

### Feasibility: Very High
- ECharts scatter with `visualMap` for bubble size
- ~50 lines of JS
- Same pattern as existing ECharts dashboard setup

---

## Concept 23: Emissions Timeline (Stacked Bar Over Time)

**Type**: Stacked bar chart (bar count per day/week)  
**Library**: Chart.js 4.x (built-in stacked bar)  
**Data**: Calls grouped by status — call_start, call_end, call_active

### Description

A stacked bar chart showing the number of calls, transmissions, and unit events per day/week, broken down by status (call_start, call_end, call_active, unit_event, trunking_message). Unlike the existing timeline, this emphasizes the relationship between different event types.

### Data Query

```sql
SELECT date_trunc('week', start_time) AS week_bucket,
       'calls' AS event_type, COUNT(*) AS count
FROM calls WHERE start_time > now() - interval '90 days'
GROUP BY week_bucket, 'calls'
UNION ALL
SELECT date_trunc('week', last_event_time), 'unit_events', COUNT(*)
FROM unit_events WHERE last_event_time > now() - interval '90 days'
GROUP BY week_bucket
UNION ALL
SELECT date_trunc('week', start_time), 'trunking_messages', COUNT(*)
FROM trunking_messages WHERE start_time > now() - interval '90 days'
GROUP BY week_bucket;
```

### Visual Design
- **Bars**: Weekly, stacked by event type (calls, unit_events, trunking_messages)
- **Colors**: Call events = blue, unit events = green, trunking = yellow
- **Hover**: Show exact counts per event type + total
- **Aggregation**: Toggle between daily, weekly, monthly views
- **Filter**: Per-system time windows

### Feasibility: Very High
- Chart.js stacked bar series built-in
- ~30 lines of JS
- Uses existing endpoints directly

---

## Concept 24: Unit Activity Clock Timeline

**Type**: Circular/Gantt-style timeline per unit  
**Library**: D3.js 7.9.0  
**Data**: Unit events (on/join/offer off) with timestamps

### Description

A circular timeline display showing each unit's activity throughout a 24-hour period. Think of it as a specialized circular Gantt chart — each radial sweep represents one unit, and the arc length within each sweep shows when that unit was active.

This is a visually striking "wall art" visualization showing the 24-hour rhythm of radio activity across all units.

### Data Query

```sql
SELECT unit_id, unit_alpha_tag, system_id,
       EXTRACT(HOUR FROM last_event_time)::int + EXTRACT(MINUTE FROM last_event_time)::int / 60.0 AS activity_hours
FROM unit_events
WHERE last_event_time > now() - interval '24 hours'
  AND unit_id IN (?)
  AND system_id IN (?)
ORDER BY last_event_time;
```

### Visual Design
- **Circular layout**: 24-hour wheel (outer ring), one band per unit (inner concentric rings)
- **Arcs**: Each arc segment = time period the unit was active (color = event type: on/join/off)
- **Center hub**: Shows current time (animated dial)
- **Legend**: Event type colors + unit counts
- **Hover**: Show unit name + last event time + TG it was active on

### Feasibility: Medium
- Pure D3.js custom rendering — radial layout requires manual trigonometry
- ~250 LOC for circular layout with arcs
- Inspiration: http://bl.ocks.org/mbostock/4063334

---

## Concept 25: Decibel Radar Ring (Per-Call Signal Profile)

**Type**: Radar/rose chart (per-call multi-axis profile)  
**Library**: ECharts 5.5.0 (radar series)  
**Data**: Single call's signal profile (signal_db, noise_db, freq_error, etc.)

### Description

A radar chart showing a call's complete signal profile across five axes: signal strength, noise floor, frequency offset, error count, spike count, and duration. Unlike the radar chart concept (Concept 3) which compares SYSTEMS, this compares WITHIN-A-CALL — showing the health profile of individual calls.

This is useful for debugging reception issues: a call with poor signal + high noise + high error count = bad channel, while a call with poor signal + low noise = far-from-repeater scenario.

### Data (Built from API response)

```sql
-- Returns all metrics for a single call (triggered by clicking on any stream-graph or timeline point)
SELECT signal_db, noise_db, freq_error, error_count, spike_count, duration
FROM calls WHERE call_id = ?;
```

### Visual Design
- **Axes**: 5 axes representing call metrics
- **Polygons**: Multiple calls overlaid, semi-transparent fills
- **Comparison mode**: Pick any two calls → overlay their radar profiles
- **Anomaly highlighting**: Highlight axes where values exceed thresholds
- **Real-time**: Show incoming call's profile as it's created via SSE

### Feasibility: Very High
- ECharts radar series with 5 dimensions is built-in
- ~60 lines of JS for single-call view
- ~120 lines for comparison mode

---

## Concept 26: Barycenter Activity Map (Pseudo-Spatial)

**Type**: Force-directed barycentric layout (no GPS needed, uses TG co-occurrence topology)  
**Library**: D3.js 7.9.0 (`d3.forceSimulation`)  
**Data**: TG co-activation pairs from `unit_ids[]` and call overlap analysis

### Description

A force-directed spatial layout where talkgroups are positioned based on their co-occurrence in the same calls — TGs that are frequently heard together cluster spatially, even without knowing actual site locations. This creates a "topological map" of the radio network.

Unlike Concept 1 (which shows edges explicitly), this focuses on the SPATIAL TOPOLOGY — no lines, just positions. The clustering reveals natural network regions (fire sector, police sector, etc.).

### Data (No API changes needed)

Same adjacency data as Concept 1, but rendered as a force-directed layout WITHOUT edges (positions only). The positions are computed by the force simulation.

### Visual Design
- **Nodes**: TGs as colored circles (color by system or TG group)
- **Layout**: Force simulation based on TG co-occurrence (same adjacency matrix as Concept 1)
- **NO edges**: Pure spatial clustering — no lines connecting nodes
- **Interactive**: Drag nodes to adjust; double-click to center on that TG
- **Labels**: On-hover alpha tags (or always show for top 20 by volume)
- **Time filter**: Live view (last hour) or historical (last 7d/30d)
- **Cluster markers**: Semi-transparent background bubbles per cluster group

### Feasibility: Medium-High
- Same adjacency data as Concept 1
- D3 force simulation built-in (no plugin needed)
- ~150 LOC for pure force layout (no edges)
- Conceptually simpler than Concept 1 (no edge rendering, no edge interactions)

---

## Concept 27: Encryption Timeline (Semi-transparent Band Chart)

**Type**: Band chart (area with gradient fill)  
**Library**: ECharts 5.5.0 (built-in band/splitArea)  
**Data**: Calls with `encrypted` flag, grouped by minute

### Description

A semi-transparent band chart showing the proportion of encrypted calls over time. The band height represents the total call volume, and the upper portion of each time bucket is colored differently to show the encryption ratio.

This reveals when encryption usage spikes — typically during sensitive operations, raids, or controlled substances investigations.

### Data Query

```sql
SELECT date_trunc('minute', start_time) AS minute_bucket,
       COUNT(*) AS total_calls,
       SUM(CASE WHEN encrypted THEN 1 ELSE 0 END) AS encrypted_calls
FROM calls
WHERE start_time > now() - interval '24 hours'
  AND system_id IN (?)
GROUP BY minute_bucket
ORDER BY minute_bucket;
```

### Visual Design
- **Background band**: All calls (transparent fill, thin outline)
- **Foreground band**: Encrypted calls (red/solid fill up to encrypted ratio)
- **Ratio bar**: Thin line showing encrypted % at the top
- **Hover**: Show exact counts + encryption % for that bucket
- **Scrollable**: Horizontal scroll through the 24h window
- **Toggle**: Show encrypted-only, non-encrypted-only, or both

### Feasibility: Very High
- ECharts band/split-area pattern using two series (filled area)
- ~60 lines of JS
- Data shape is flat time series, easy to render

---

## Concept 28: Frequency Proximity Network

**Type**: Proximity graph (force-directed layout constrained by frequency)  
**Library**: D3.js 7.9.0 (`d3.forceSimulation`)  
**Data**: TG frequencies from `/talkgroups` endpoint

### Description

A force-directed layout where talkgroups are constrained to positions within ±5 MHz of their actual frequency, while attractive forces pull co-occurring TGs together. This creates a hybrid layout that is BOTH frequency-accurate AND shows co-occurrence clustering.

This is the best of both worlds: RF engineers can identify frequency-adjacent TGs, while operations can see functional clusters. The frequency constraint prevents the layout from collapsing everything into meaningless clusters.

### Data (Built from APIs)

```sql
-- Frequencies + call counts for top TGs
SELECT t.system_id, t.tgid, t.tgid_alpha_tag, c.freq,
       COUNT(c.call_id) AS call_count
FROM talkgroups t
JOIN calls c ON c.tgid = t.tgid AND c.system_id = t.system_id
WHERE c.start_time > now() - interval '30 days'
  AND c.freq IS NOT NULL
  AND t.system_id IN (?)
GROUP BY t.system_id, t.tgid, t.tgid_alpha_tag, c.freq;
```

### Visual Design
- **Nodes**: TGs colored by system, sized by call count
- **Layout constraint**: Nodes constrained to Y-axis position proportional to frequency (log scale)
- **Attractive force**: Co-occurring TGs pulled together (horizontal force only, Y = frequency)
- **No edge rendering**: Pure spatial clustering
- **X-axis**: Functional clusters (no explicit meaning, just topology)
- **Y-axis**: Frequency (MHz), labeled with tick marks

### Feasibility: Medium-High
- D3 force simulation with custom `forceX`/`forceY` constraints
- ~200 LOC for frequency-constrained layout
- Requires combining co-occurrence data with frequency spectrum data

---

## Concept 29: Transcription Sentiment Timeline

**Type**: Line chart with sentiment bands (positive/negative sentiment visualization)  
**Library**: ECharts 5.5.0 (built-in line + annotation) or Chart.js  
**Data**: Calls with `transcription_text` + client-side sentiment scoring

### Description

A timeline of call transcriptions with sentiment analysis, showing positive and negative "waves" of communication. Each call is colored by its sentiment score (positive = green, negative = red, neutral = gray). Over time, this creates a visual narrative of tense vs calm operations.

This is novel because it adds a text-analysis dimension to the radio monitoring data — operators can see when situations escalated emotionally.

### Data (No API changes needed)

```sql
-- Get transcriptions for sentiment scoring
SELECT call_id, start_time, stop_time, duration,
       transcription_text, emergency, tgid
FROM calls
WHERE transcription_text IS NOT NULL AND transcription_text != ''
  AND start_time > now() - interval '7 days'
  AND system_id IN (?)
LIMIT 2000;
```

Client-side: sentiment scoring via simple keyword dictionary (no ML needed for basic version)

### Visual Design
- **X-axis**: Time (calls in chronological order)
- **Y-axis**: Sentiment score (-1 to +1 from keyword dictionary)
- **Bands**: Semi-transparent fill above/below zero for positive/negative
- **Lines**: Sentiment score over time with smooth interpolation
- **Annotation**: Mark significant points (highly positive or negative calls)
- **Toggle**: Show raw sentiment or smoothed moving average

### Feasibility: Medium
- Keyword dictionary scoring client-side (simple word list)
- ECharts line chart with gradient fill
- ~150 LOC (50 sentiment scoring + 100 ECharts rendering)
- Dictionary approach is simple but effective for radio jargon

---

## Concept 30: Talkgroup Affiliation Heatmap

**Type**: Matrix heatmap (unit ↔ TG over time)  
**Library**: ECharts 5.5.0 (`matrix` coordinate system)  
**Data**: Calls with `unit_ids[]`

### Description

A time-evolving matrix heatmap showing unit-to-TG affiliations throughout a day. Each column = one hour, each row = one TG. Cells are colored by which units affiliated with that TG during that hour. Multiple units per cell are shown as a compact multi-color patch or count indicator.

This reveals shifting unit-to-TG affiliations over time — e.g., units moving from fire dispatch to fire engines to command during an evolving incident.

### Data Query

```sql
SELECT tgid, date_trunc('hour', start_time) AS activity_hour,
       unit_ids
FROM calls
WHERE start_time > now() - interval '24 hours'
  AND unit_ids IS NOT NULL AND array_length(unit_ids, 1) > 0
  AND system_id IN (?)
GROUP BY tgid, activity_hour, unit_ids
ORDER BY activity_hour, tgid;
```

### Visual Design
- **Columns**: Hours (0-23), rows = talkgroups (top 30 by activity)
- **Cells**: Colored patch per unit + count indicator (number of units)
- **Time slider**: Horizontal scrubber at top to view any 6/12/24h window
- **Color scheme**: Per-unit distinct colors (Category20b) or uniform color with count overlay
- **Hover**: Show TG name, hour, list of units + their roles

### Feasibility: Medium
- ECharts matrix coordinate system with custom rendering per cell
- ~200 LOC for matrix heatmap with cell coloring
- Data volume: 30 cells (rows) × 24 cells (cols) = 720 cells

---

## Summary Matrix

| # | Name | Technique | Library | API Changes | Feasibility |
|---|------|-----------|---------|-------------|-------------|
| **7** | Call Metrics Parallel Coordinates | Parallel coords (multivariate) | D3 + d3.parcoords | None | Medium |
| **8** | Signal Quality Scatterplot Matrix | SPLOM (pairwise scatter) | D3.js | None | High |
| **9** | Daily Overview Treemap | Treemap (area-proportional) | D3.js built-in | None | Very High |
| **10** | Talkgroup Activity Sunburst | Radial sunburst | ECharts/D3 | None | Very High |
| **11** | TG Correlation Matrix | Correlation + scatter | ECharts | None | High |
| **12** | Daily Volume Calendar Heatmap | Calendar heatmap | ECharts built-in | None | Very High |
| **13** | Signal Waterfall Display | Waterfall (time-freq) | D3 + d3-waterfall | None | Medium |
| **14** | Emergency Frequency Breakdown | Stacked area chart | ECharts built-in | None | Very High |
| **15** | Transmission Duration Dot Plot | Dot plot (horizontal jitter) | Chart.js | None | Medium |
| **16** | Transcription Word Cloud | Word cloud (typography) | D3 + d3-cloud | None | Medium |
| **17** | Frequency Spectrum Treemap | Treemap (freq-banded) | D3.js built-in | None | High |
| **18** | Unit Call Correlation Matrix | Matrix heatmap | ECharts matrix | None | Medium |
| **19** | Recorder Signal Gauge | Analog gauges (cockpit) | ECharts gauge / D3 | None | High |
| **20** | TG Activity Slope Chart | Slope/bump chart | Chart.js/D3 | None | Very High |
| **21** | Call Event Ridge Plot | Ridgeline/Density plot | D3 | None | Medium |
| **22** | Talkgroup Bubble Scatter | Bubble chart (3D scatter) | ECharts built-in | None | Very High |
| **23** | Emissions Timeline | Stacked bar chart | Chart.js built-in | None | Very High |
| **24** | Unit Activity Clock | Circular Gantt | D3.js | None | Medium |
| **25** | Decibel Radar Ring | Radar (per-call profile) | ECharts radar | None | Very High |
| **26** | Barycenter Activity Map | Force-layout (spatial) | D3 force | None | High |
| **27** | Encryption Timeline | Band chart (ratio) | ECharts built-in | None | Very High |
| **28** | Frequency Proximity Network | Frequency-constrained force | D3 force | None | High |
| **29** | Sentiment Timeline | Sentiment line chart | ECharts/Chart.js | None | Medium |
| **30** | TG Affiliation Heatmap | Matrix heatmap (time-evolving) | ECharts | None | Medium |

---

## Recommendations (Top 5 to Implement First)

1. **#22 — Talkgroup Bubble Scatter**: Fastest to build (~50 LOC), ECharts built-in, highly visual
2. **#9 — Daily Overview Treemap**: Built-in D3, ~80 LOC, immediately useful
3. **#19 — Recorder Signal Gauge**: Cockpit dashboard feel, ~150 LOC, builds on existing recorder data
4. **#10 — Talkgroup Activity Sunburst**: ECharts built-in, ~100 LOC, visually striking
5. **#12 — Daily Volume Calendar Heatmap**: ECharts built-in, ~50 LOC, shows seasonal patterns
