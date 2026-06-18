# Design: mattermost-plugin-disallow-dm

**Date:** 2026-06-18
**Status:** Approved (v1)
**Repository:** https://github.com/approveninja/mattermost-plugin-disallow-dm

## 1. Purpose

Prevent users on a Mattermost server from sending each other **direct messages
(DMs, 1-to-1)** and **group messages (GMs, 3-7 people)**. All communication
should happen in regular channels instead.

The block must be **impossible to bypass** from any client (web, desktop,
mobile, API). System notifications and integrations must keep working.

## 2. Scope

### In scope (v1)
- Server-side rejection of human-authored posts in DM and GM channels.
- Configurable exceptions: bots/integrations, system administrators,
  self-messages.
- Configurable rejection message and a toggle for blocking group messages.

### Out of scope (v1)
- **Webapp UI hiding** (hiding the "+" next to Direct Messages, "Message"
  buttons on profiles, etc.). This is cosmetic only and version-fragile; it is
  deferred to a possible v2. The repo keeps the template's `webapp/` directory
  as a disabled stub so v2 can be added without restructuring.
- **Per-team configuration.** DM/GM channels in Mattermost are global and not
  attached to any team, so the block is server-wide. (Considered and explicitly
  rejected.)
- Deleting or hiding existing DM/GM channels and their history.

## 3. Approach

Enforcement is **server-only**. The single source of truth is the
`MessageWillBePosted` plugin hook, which runs on the server for every post and
can reject it with an error message. Because it runs server-side, no client can
bypass it.

UI hiding was evaluated and rejected for v1: it adds no security (purely
cosmetic), requires a webapp build, and relies on version-fragile DOM/CSS hacks
because Mattermost offers no official API to remove DM entry points.

Trade-off accepted: a user can still open a DM channel and type a message; they
only see the rejection when they press send. This is acceptable for v1.

## 4. Architecture

Based on the official
[`mattermost-plugin-starter-template`](https://github.com/mattermost/mattermost-plugin-starter-template).

```
plugin.json                  manifest; webapp bundle disabled; settings_schema
server/
  plugin.go                  Plugin struct, lifecycle, OnConfigurationChange
  configuration.go           configuration struct + thread-safe accessor
  message_hooks.go           MessageWillBePosted hook + decision logic
  message_hooks_test.go      unit tests against plugintest mock API
webapp/                      kept as disabled stub for a future v2
```

The plugin is server-only; `webapp.bundle_path` is removed from `plugin.json`
so no JS bundle is required to load the plugin.

## 5. Core logic — `MessageWillBePosted`

Signature:
```go
func (p *Plugin) MessageWillBePosted(c *plugin.Context, post *model.Post) (*model.Post, string)
```
Returning a non-empty string rejects the post and shows that string to the user.
Returning `(post, "")` allows it.

Decision order for each post:

1. **Look up the channel** via `p.API.GetChannel(post.ChannelId)`.
   - On error: **fail open** (allow + log a warning) so a transient lookup
     failure never blocks posting platform-wide.
2. If channel type is **not** `Direct` and **not** `Group` → allow.
3. If channel type is `Group` and `BlockGroupMessages` is **false** → allow.
4. **Exceptions** (any match → allow):
   - **Bot / integration author:** `p.API.GetUser(post.UserId).IsBot == true`.
     This naturally lets system bots, webhooks, and other plugins keep DMing
     users (notifications stay intact).
   - **System post:** `post.Type` starts with `system_` (e.g. join/leave
     messages, system bot messages).
   - **System administrator author** (only if `AllowAdmins` is true):
     user roles contain `system_admin`.
   - **Self-message** (only if `AllowSelfMessages` is true and channel is
     `Direct`): `channel.GetOtherUserIdForDM(post.UserId) == ""` identifies a
     DM channel with oneself.
5. Otherwise → **reject** with `RejectionMessage`.

## 6. Configuration (System Console)

`plugin.json` `settings_schema.settings`, mapped to a Go `configuration` struct:

| Key                  | Type     | Default | Meaning |
|----------------------|----------|---------|---------|
| `RejectionMessage`   | text     | "Direct messages are disabled by your administrator." | Error shown when a DM/GM is blocked. |
| `BlockGroupMessages` | bool     | `true`  | Also block group messages (GM). |
| `AllowAdmins`        | bool     | `true`  | Let system admins send DMs/GMs. |
| `AllowSelfMessages`  | bool     | `true`  | Allow messages to oneself (notes). |

Configuration is read through a thread-safe accessor and refreshed in
`OnConfigurationChange`, following the starter-template pattern. An empty
`RejectionMessage` falls back to the built-in default.

## 7. Error handling

- **Channel/user lookup failures:** fail open (allow) and log at warn level.
  Rationale: a blocking plugin must never become a server-wide outage of posting
  due to a transient API error.
- **Unknown/edge channel types:** only `Direct` and `Group` are ever blocked;
  everything else is allowed by default.

## 8. Testing

Unit tests in `message_hooks_test.go` using
`github.com/mattermost/mattermost/server/public/plugin/plugintest` mocks for
`p.API`. Each test mocks `GetChannel`/`GetUser` and asserts the returned
rejection string (empty = allowed). Matrix:

| Case | Channel | Author | Expect |
|------|---------|--------|--------|
| DM between two humans | Direct | human | rejected |
| GM, blocking on | Group | human | rejected |
| GM, `BlockGroupMessages=false` | Group | human | allowed |
| Public/private channel | Open/Private | human | allowed |
| Bot author in DM | Direct | bot | allowed |
| System post in DM | Direct | system_* | allowed |
| Admin in DM, `AllowAdmins=true` | Direct | admin | allowed |
| Admin in DM, `AllowAdmins=false` | Direct | admin | rejected |
| Self-DM, `AllowSelfMessages=true` | Direct (self) | human | allowed |
| Self-DM, `AllowSelfMessages=false` | Direct (self) | human | rejected |
| `GetChannel` error | — | — | allowed (fail open) |

CI: the starter template's GitHub Actions workflow runs build + lint
(`golangci-lint`) + tests on push/PR.

## 9. Implementation outline

1. Re-identify the plugin: `plugin.json` id `com.approveninja.disallow-dm`,
   name/description/urls; rename the `go.mod` module path and update imports;
   update `README.md`.
2. Remove `webapp.bundle_path` from `plugin.json`; leave `webapp/` as a stub.
3. Add the four settings to `settings_schema` and the `configuration` struct.
4. Implement `MessageWillBePosted` per section 5.
5. Write unit tests per section 8.
6. Build the bundle with `make dist`; manually verify on a local Mattermost
   server (DM blocked, bot DM allowed, admin allowed, self-note allowed).

## 10. Open questions

None. Design approved 2026-06-18.
