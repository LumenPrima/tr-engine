# Starred Meta-Channel Design

## Goal

Add a virtual `★ Starred` channel to IRC Radio Live that aggregates all activity from favorited talkgroups into one chronological feed — so users don't have to switch between channels to see their starred activity.

## Background

IRC Radio Live already has a full favorites system: star icons on channels, persistent localStorage storage (`irc_favorites` as `Set<"system_id:tgid">`), and a dedicated Favorites section in the sidebar. What's missing is an aggregated view that shows all favorited activity in one place.

Feature request from J-Man on Discord:
> "if I star a channel, give me a channel where all of that stuff shows up instead of going channel to channel to see it"

## Architecture

**Frontend-only change.** No backend, API, or database modifications needed. The SSE event stream already delivers all events globally — client-side filtering is what routes events to channels. The starred channel simply routes events from any favorited talkgroup into an additional virtual channel.

### Single file modified

`web/irc-radio-live.html` — ~150-200 lines of JS additions.

## Design

### Virtual Channel

- **Name**: `★ Starred` (constant, not a real talkgroup)
- **Location**: Top of the Favorites sidebar section, always first when favorites exist
- **Behavior**: Identical to any real channel — click to switch, see messages, scroll up for history
- **Internal key**: `STARRED_CHANNEL = '★ Starred'` (like the existing `STATUS_CHANNEL = '#status'`)

### Message Routing

When an SSE event (call_start, call_end, unit_event, transcription) arrives for a talkgroup that is favorited:

1. Route to the talkgroup's own channel (existing behavior, unchanged)
2. Also route a copy to the `★ Starred` channel with a **channel origin tag**

The channel origin tag is a clickable prefix like `[#Fire-Dispatch]` that lets the user jump to the source channel.

### History Loading

When the user switches to `★ Starred`:

1. Collect all favorited talkgroup IDs
2. For each, call the existing `GET /api/v1/calls?system_id=X&tgid=Y&limit=10` endpoint
3. Merge results chronologically
4. Render in the message area

This reuses the existing `lazyLoadChannel()` / `callsToMessages()` pattern. Infinite scroll works the same way — load older calls from all favorites on scroll-up.

### Channel State

```javascript
channels.set(STARRED_CHANNEL, {
  tgid: null,           // no single talkgroup
  system_id: null,      // no single system
  alpha_tag: '★ Starred',
  description: 'All activity from your starred channels',
  tag: '',
  group: 'Favorites',
  mode: '+nt',
  users: new Map(),     // union of users across favorited channels
  messages: [],
  historyExhausted: false,
});
```

### Dynamic Membership

- Starring a channel: events from that talkgroup start appearing in `★ Starred` immediately
- Unstarring: future events stop appearing (existing messages remain in the feed)
- If all stars removed: `★ Starred` channel is removed from sidebar, switches to `#status` if active

### UI Details

- **Topic bar**: "Aggregating N starred channels"
- **Nicklist**: Union of online users from all favorited channels
- **Origin tag on messages**: `[#channel-name]` prefix, styled with `opacity: 0.7`, clickable to switch channel
- **No star icon** on `★ Starred` itself (can't star the meta-channel)
- **No scan icon** on `★ Starred` (scanner operates on individual channels)
- **Empty state**: If switched to with no favorites, show a notice: "Star channels with ★ to see their activity here"
- **Unread badge**: Increments when events arrive for any favorited talkgroup while `★ Starred` is not the active channel

### What It Does NOT Do

- No scanner/auto-play integration (scanner already serves that purpose independently)
- No notifications beyond the existing unread badge system
- No backend changes or new API endpoints
- No separate "aggregate starred history" API call — reuses per-talkgroup calls endpoint
