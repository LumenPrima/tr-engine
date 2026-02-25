# Page Visibility — Inline Nav Edit Mode

## Summary

Add per-browser page hiding to the nav dropdown in `theme-engine.js`. Users can toggle pages on/off via an inline edit mode. No server changes needed.

## Storage

- **Key:** `eh-hidden-pages` in `localStorage`
- **Value:** JSON array of page paths, e.g. `["/scanner.html", "/docs.html"]`

## UI Changes (all in `theme-engine.js`)

### Normal Mode (edit off)
- Hidden pages are filtered out of the nav dropdown
- A subtle "Manage pages" link at the bottom of the dropdown (pencil icon + text)
- If all pages are hidden, show "no pages (manage)" placeholder

### Edit Mode (edit on)
- All pages show, including hidden ones
- Hidden pages are dimmed
- Each page link gets an eye icon (right side): open-eye = visible, closed-eye = hidden
- Clicking the eye toggles that page in `eh-hidden-pages`
- "Done" button replaces the "Manage pages" link

## Behavior
- Hidden pages are still accessible by direct URL (cosmetic filtering only)
- Current page remains filtered out in both modes
- Filtering is client-side in `loadNavPages()`

## Files Modified
- `web/theme-engine.js` — CSS for edit mode + eye icons, `loadNavPages()` filtering logic, manage/done toggle
