# Migrating Auth Configuration

This guide covers changes to tr-engine's authentication system introduced in v0.9.8. The auth model has been simplified — most users need to make minor changes to their `.env`.

## What Changed

Auth mode is now derived automatically from which env vars are set. The old `AUTH_ENABLED` flag and `WRITE_TOKEN` are deprecated. The new system has three modes:

| Mode | When | Behavior |
|------|------|----------|
| `open` | No `AUTH_TOKEN`, no `ADMIN_PASSWORD` | No auth required. All endpoints are public. |
| `token` | `AUTH_TOKEN` set, no `ADMIN_PASSWORD` | Single shared token. Dashboard prompts users to enter it. |
| `full` | `ADMIN_PASSWORD` set | JWT login for write access. Optional `AUTH_TOKEN` for public read. |

---

## Migration by Setup Type

### 1. You had `AUTH_ENABLED=false` (open/no-auth setup)

**Before:**
```env
AUTH_ENABLED=false
```

**After:** Remove `AUTH_ENABLED` entirely and make sure `AUTH_TOKEN` and `ADMIN_PASSWORD` are both unset (or commented out). No other changes needed.

```env
# AUTH_ENABLED is deprecated and no longer needed
# Just leave AUTH_TOKEN and ADMIN_PASSWORD unset
```

> **Note:** If you had `AUTH_ENABLED=false` alongside token vars, the old behavior cleared those tokens on startup. The new behavior is the same — unset `AUTH_TOKEN` and `ADMIN_PASSWORD` to get open mode.

---

### 2. You had `AUTH_TOKEN` only (single shared token, no user login)

**Before:**
```env
AUTH_ENABLED=true
AUTH_TOKEN=your-token-here
```

**After:** Remove `AUTH_ENABLED`. Keep `AUTH_TOKEN`. Do not set `ADMIN_PASSWORD`.

```env
AUTH_TOKEN=your-token-here
```

The dashboard now prompts users to enter the token directly instead of relying on Caddy to inject it. Once entered, the token is saved in the browser's localStorage.

---

### 3. You had `AUTH_TOKEN` + `ADMIN_PASSWORD` (public read + JWT login)

No changes needed. This maps directly to `full` mode:

- `AUTH_TOKEN` becomes the public read token (handed out by `auth-init` to unauthenticated browsers)
- `ADMIN_PASSWORD` enables JWT login for write access

Remove `AUTH_ENABLED` if present — it's no longer needed.

---

### 4. You had `WRITE_TOKEN` set

`WRITE_TOKEN` is deprecated. It still works during the transition period but will be removed in a future release.

**Upgrade path:**
- Set `ADMIN_PASSWORD` to enable JWT login
- Remove `WRITE_TOKEN`
- Any services using `WRITE_TOKEN` directly (upload plugins, scripts) should be migrated to use API keys (`tre_` prefix) or the JWT login flow

In the meantime, a deprecation warning is logged at startup if `WRITE_TOKEN` is set.

---

### 5. You use Caddy to inject the auth token

The old Caddyfile pattern for injecting the read token into requests:

```caddy
@no_auth not header Authorization *
request_header @no_auth Authorization "Bearer {$TR_AUTH_TOKEN}"
```

This is now **handled by the dashboard itself** — it fetches its token from `auth-init` directly. The Caddy injection block is harmless to leave in place but is no longer required. You can remove it as optional cleanup.

---

## Deprecated Variables

| Variable | Status | Replace With |
|----------|--------|--------------|
| `AUTH_ENABLED` | Deprecated — ignored | Remove it. Set/unset `AUTH_TOKEN` and `ADMIN_PASSWORD` instead. |
| `WRITE_TOKEN` | Deprecated — warns at startup | Use `ADMIN_PASSWORD` + JWT login, or API keys for service accounts. |

---

## Quick Reference: Auth Mode by Config

```
AUTH_TOKEN unset, ADMIN_PASSWORD unset  →  open   (no auth)
AUTH_TOKEN set,   ADMIN_PASSWORD unset  →  token  (shared token)
                  ADMIN_PASSWORD set    →  full   (JWT login + optional read token)
```
