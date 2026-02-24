# IRC Client Overhaul — Design Document

**Date:** 2026-02-24
**Scope:** Channel tree redesign, mobile responsive layout, scanner mode
**File:** `web/irc-radio-live.html` (2910 lines, single-file SPA)

## Context

The IRC client is a mature read-only radio monitor with 1000+ channels, 3000+ users, SSE-driven live updates, audio playback, transcriptions, and 11 themes. The current pain points:

- **Channel tree is unnavigable at scale** — 1000 channels across ~60 groups, no search, no favorites, active channels buried in the scroll.
- **No mobile support** — 3-panel desktop layout breaks on small screens.
- **No scanner mode** — users want auto-play functionality for monitored channels.

## 1. Channel Tree Overhaul

### 1.1 Search Box

- Always visible at top of the channel tree panel.
- Debounced input (150ms). Filters across channel name, group name, and alpha_tag.
- When active (non-empty): shows flat filtered results without group headers. Matches highlighted.
- Clear button (X) resets filter and returns to grouped view.

### 1.2 Favorites

- "Favorites" group pinned at top of the tree, always expanded.
- Star icon (outlined/filled) on each channel — click to toggle favorite.
  - Desktop: star appears on hover.
  - Mobile: star always visible.
- Stored in localStorage as array of `system_id:tgid` keys.
- Favorited channels appear in both the Favorites section and their normal group position (with a subtle star indicator in the group position).
- No limit on favorites count — user self-curates.

### 1.3 Smart Collapse

- **Groups with unread channels:** auto-expanded, showing only channels with unread messages. Remaining channels behind "N more..." expander.
- **Groups with zero unread:** auto-collapsed, showing only the group header.
- **Group unread counts:** displayed in group header, e.g., "Butler County (09) Law **[12]**".
- Manual expand/collapse overrides auto behavior and is sticky (persisted to localStorage per group).

### 1.4 Result

Tree scroll order becomes: Favorites (5-10 items) -> expanded groups with active channels -> collapsed quiet groups. Dramatically shorter scroll surface.

## 2. Mobile Responsive Layout

### 2.1 Breakpoint

768px. Below this, mobile layout activates.

### 2.2 Layout Changes

| Component | Desktop (>768px) | Mobile (<=768px) |
|-----------|------------------|-------------------|
| Channel tree | Fixed left panel (240px) | Slide-out drawer from left |
| Messages | Center panel | Full viewport width |
| Nicklist | Fixed right panel | Slide-out drawer from right |
| Top bar | Server + channel info + decode rate | Hamburger + channel name + user count button |
| Input bar | Channel tag + command input | Same, full width |
| Status bar | Full stats | Simplified (connection + events/min) |

### 2.3 Navigation

- Hamburger button (top-left) or swipe-right from screen edge opens channel drawer.
- User count button (top-right) or swipe-left from screen edge opens nicklist.
- Tap channel in drawer auto-switches and closes drawer.
- Drawers have backdrop overlay; tap outside to close.

### 2.4 Mobile Message Format

Simplified card-style layout instead of IRC `<nick>` format:

```
Nick Name                               09:24
  Message text here with transcription
  [Play 3.2s]
```

- Nick on top line (colored, bold), timestamp right-aligned.
- Message text below, full width.
- Audio play buttons 44px minimum touch target.
- Transcription text inline below audio controls.
- Emergency calls: colored left border (red) instead of full-row highlight.

### 2.5 Touch Interactions

- Swipe right from edge: open channel drawer.
- Swipe left from edge: open nicklist drawer.
- Pull-down on messages: lazy load older history (replaces scroll-to-top detection).

## 3. Scanner Mode

### 3.1 Scan List

- Independent from favorites. Separate concept: "channels I watch" vs "channels I listen to."
- Per-channel toggle via antenna/radio icon in channel tree.
- Stored in localStorage as `scan_channels` array of `system_id:tgid` keys.
- Scanned channels show a subtle indicator in the tree (colored dot or icon).

### 3.2 Global Toggle

- Prominent on/off button in the status bar.
- When ON: auto-plays audio from scanned channels as `call_end` events arrive with audio.
- When OFF: scan list persists, no auto-play.
- Visual indicator when active (pulsing icon or status bar color change).

### 3.3 Audio Queue

- FIFO queue. New transmissions from scanned channels appended.
- Current transmission finishes before next starts.
- Queue depth indicator: "Playing 1 of 3" in audio bar.
- Skip button: jump to next in queue.
- Clear queue button: flush all pending.
- Overflow protection: if queue depth > 10, oldest unplayed items dropped with notice ("Skipped N transmissions").

### 3.4 Auto-Channel Switching

- When scanner plays a transmission, auto-switch to that channel for context.
- Default ON, toggleable via scanner settings.

### 3.5 Emergency Override

- Emergency calls from scanned channels jump to front of queue (next-in-line, not interrupting current playback).

### 3.6 Mobile Scanner

- Scanner toggle in simplified top bar.
- Works in background — audio plays through speaker/headphones.
- Standard browser audio behavior allows screen-off playback.

## 4. What Stays the Same

- All existing functionality: SSE, audio playback, transcriptions, call grouping, /commands, themes.
- Desktop layout above 768px (3-panel, IRC message formatting).
- Theme system, CRT effects, nerd mode.
- All 11 themes work on mobile via existing CSS variables.

## 5. Technical Notes

- Single-file SPA remains (no build tools). All changes within `irc-radio-live.html`.
- CSS media queries for responsive breakpoint.
- Touch events for swipe gestures (with pointer event fallback).
- localStorage keys: `favorites`, `scan_channels`, `scanner_enabled`, `scanner_auto_switch`, `group_collapsed_*`.
- Audio queue managed in JS state alongside existing `activeCalls` Map.
- Search filtering operates on the existing `channels` Map and `tgidToChannel` Map.
