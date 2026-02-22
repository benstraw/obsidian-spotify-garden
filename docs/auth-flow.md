# OAuth 2.0 Auth Flow

spotify-garden uses the Spotify OAuth 2.0 Authorization Code flow to obtain
access and refresh tokens.

## Flow Overview — localhost redirect (default)

```
User                  CLI                   Spotify API          Browser
 |                     |                        |                   |
 |  spotify-garden auth|                        |                   |
 |-------------------->|                        |                   |
 |                     | build auth URL         |                   |
 |                     |--------------------------------- open ----->|
 |                     |                        |                   |
 |                     | start HTTP server :8888|       user logs in |
 |                     |                        |<------------------|
 |                     |<-- GET /callback?code=X                    |
 |                     |                        |                   |
 |                     | POST /api/token        |                   |
 |                     |----------------------->|                   |
 |                     |<-- access_token,       |                   |
 |                     |    refresh_token       |                   |
 |                     |                        |                   |
 |                     | save tokens.json (0600)|                   |
 |                     |                        |                   |
```

## Flow Overview — external redirect URI

When `SPOTIFY_REDIRECT_URI` is not a localhost URL, no local server is started.
Instead, the CLI prompts you to paste the redirect URL from your browser:

```
User                  CLI                   Spotify API          Browser
 |                     |                        |                   |
 |  spotify-garden auth|                        |                   |
 |-------------------->|                        |                   |
 |                     | build auth URL         |                   |
 |                     |--------------------------------- open ----->|
 |                     |                        |    user logs in   |
 |                     |                        |<------------------|
 |                     |                        | redirect to URI?code=X
 |  paste redirect URL |                        |                   |
 |<--------------------|                        |                   |
 |-------------------->|                        |                   |
 |                     | POST /api/token        |                   |
 |                     |----------------------->|                   |
 |                     |<-- access_token, ...   |                   |
 |                     | save tokens.json (0600)|                   |
```

## Required Environment Variables

| Variable | Description |
|---|---|
| `SPOTIFY_CLIENT_ID` | OAuth app client ID from developer.spotify.com |
| `SPOTIFY_CLIENT_SECRET` | OAuth app client secret |
| `SPOTIFY_REDIRECT_URI` | Must be registered in your Spotify app settings |

`SPOTIFY_REDIRECT_URI` defaults to `http://localhost:8888/callback` if not set.

## Scopes Requested

| Scope | Used by |
|---|---|
| `user-read-recently-played` | `collect` |
| `user-top-read` | `weekly`, `persona` |

## Token Exchange

Spotify requires **HTTP Basic authentication** for the token endpoint — client
credentials are sent as `Authorization: Basic base64(client_id:client_secret)`,
not as form fields. This differs from some other OAuth providers.

## Token Storage

Tokens are written to `tokens.json` in the current working directory with
`0600` permissions (owner read/write only). The file is git-ignored.

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "expires_at": "2026-02-21T15:00:00Z"
}
```

`expires_at` is computed at save time as `now + expires_in` so subsequent
runs can check expiry without calling the API.

## Automatic Token Refresh

Every command calls `auth.RefreshIfNeeded()` before making API calls:

1. Load `tokens.json`
2. Check if `expires_at` is within 5 minutes of now
3. If expiring soon: POST to token endpoint with `grant_type=refresh_token`
4. Save new tokens back to `tokens.json`
5. Return the valid access token

You should only need to run `spotify-garden auth` once. The refresh token is
long-lived — Spotify does not expire it as long as it is used periodically.

## Troubleshooting

**"INVALID_CLIENT: Invalid redirect URI"** — The redirect URI in your `.env`
does not match any URI registered in the Spotify Developer Dashboard. Add the
exact URI in the app's Edit Settings → Redirect URIs and save.

**"SPOTIFY_CLIENT_ID not set"** — The `.env` file was not found or is missing
the variable. Ensure `.env` exists in the directory where you run the binary.

**"no code found in pasted URL"** — When using an external redirect URI, the
pasted URL must include `?code=...`. If Spotify showed an error page, check
for `?error=` in the URL and address the cause.

**"state mismatch"** — The pasted URL doesn't match the current auth session.
This can happen if you paste a URL from a previous auth attempt. Run
`spotify-garden auth` again and paste the URL from the new browser session.

**"token endpoint returned 401"** — Client ID or secret is wrong, or the
authorization code has already been used. Authorization codes are single-use
and short-lived — complete the flow without navigating away or reloading.

**"token endpoint returned 400"** — The redirect URI sent during token exchange
must exactly match the one used during authorization. Ensure `SPOTIFY_REDIRECT_URI`
is consistent.

**Port 8888 already in use** — Stop whatever is using port 8888 and retry, or
switch to an external redirect URI.

**tokens.json not found** — Run `./spotify-garden auth` first. The file must
exist in the working directory where you run commands (the project root when
using the shell wrappers).
