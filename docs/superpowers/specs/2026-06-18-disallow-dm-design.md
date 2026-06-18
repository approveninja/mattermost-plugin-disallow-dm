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
- Server-side rejection of human-to-human posts in DM and GM channels.
- Always-on exceptions: bots/integrations (either side) and system messages.
- Configurable exceptions: system administrators (either side) and
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
- **Editing existing messages.** The block applies to newly posted messages
  (`MessageWillBePosted`), not to edits (`MessageWillBeUpdated`). Editing a
  pre-existing DM/GM message is therefore not blocked. For users who never
  exchanged DMs before the plugin was installed there is no such history to
  edit, so this is an accepted v1 limitation (revisit in v2 if needed).

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

Decision order for each post. Cheap, pure checks (no API call) run first; the
API-backed exception checks run last so failures can be handled with
**scoped fail-closed** (see section 7).

1. **Look up the channel** via `p.API.GetChannel(post.ChannelId)`.
   - On error: **allow** (+ log a warning). We cannot confirm the channel is a
     DM/GM, so we must not block — otherwise a transient lookup error would
     block posting in *every* channel. Fail-closed does **not** apply here.
2. If channel type is **not** `Direct` and **not** `Group` → allow.
3. If channel type is `Group` and `BlockGroupMessages` is **false** → allow.
4. **Pure exceptions** (no API call; any match → allow):
   - **System post:** `post.Type` starts with `system_` (e.g. join/leave
     messages, system bot messages).
   - **Self-message** (only if `AllowSelfMessages` is true and channel is
     `Direct`): `channel.GetOtherUserIdForDM(post.UserId) == ""` identifies a
     DM channel with oneself.
   - **Incoming-webhook post** (only if `AllowWebhookMessages` is true):
     `post.GetProp(model.PostPropsFromWebhook) == "true"`. **Security caveat:**
     the `from_webhook` prop is set by the server's webhook handler, but it is
     only spoof-proof when the server runs hardened mode
     (`ExperimentalEnableHardenedMode`, default off). When hardened mode is off,
     any user can set this prop via the API, so enabling this toggle weakens the
     "cannot be bypassed via API" guarantee. Off by default. Bot-owned webhooks
     and integrations are exempt regardless of this toggle (via `IsBot`).
5. **Author lookup** via `p.API.GetUser(post.UserId)`.
   - On error: **reject** (fail closed) — we already know this is a DM/GM and
     cannot confirm an exemption.
   - **Bot / integration author** (`user.IsBot == true`) → allow. This lets
     system bots, webhooks, and other plugins keep DMing users (notifications
     stay intact).
   - **Author is a system administrator** (only if `AllowAdmins`) → allow.
6. **Other participants on the receiving side.** Resolve the other user(s) and
   allow if any of them qualifies:
   - **Direct:** `other := channel.GetOtherUserIdForDM(post.UserId)`; if
     `other != ""`, `p.API.GetUser(other)` — on error **reject** (fail closed).
   - **Group:** `p.API.GetUsersInChannel(...)` — on error **reject** (fail
     closed).
   - Allow if **any** resolved participant is a **bot** (`IsBot`) — a human↔bot
     conversation is not a user-to-user one.
   - Allow if `AllowAdmins` and **any** resolved participant is a **system
     administrator** — admins must be reachable by anyone.
7. Otherwise → **reject** with `RejectionMessage`.

Two helpers centralize the participant rules: `isSystemAdmin(user)` (checks
`user.Roles` contains `system_admin`) and the `IsBot` flag. The rules are
**bidirectional**: a DM/GM is allowed whenever a bot or (when `AllowAdmins`) an
admin sits on *either* end of it. Human↔human conversations are always blocked.

## 6. Configuration (System Console)

`plugin.json` `settings_schema.settings`, mapped to a Go `configuration` struct:

| Key                  | Type     | Default | Meaning |
|----------------------|----------|---------|---------|
| `RejectionMessage`   | text     | "Direct messages are disabled by your administrator." | Error shown when a DM/GM is blocked. |
| `BlockGroupMessages` | bool     | `true`  | Also block group messages (GM). |
| `AllowAdmins`        | bool     | `true`  | Allow DMs/GMs whenever a system admin is involved — admins can message anyone, and anyone can message an admin (bidirectional). |
| `AllowSelfMessages`  | bool     | `true`  | Allow messages to oneself (notes). |
| `AllowWebhookMessages` | bool   | `false` | Allow incoming-webhook-authored posts (`from_webhook`) in DMs/GMs. Off by default; spoofable via API unless the server runs hardened mode. Bot-owned integrations are exempt regardless. |

Configuration is read through a thread-safe accessor and refreshed in
`OnConfigurationChange`, following the starter-template pattern. An empty
`RejectionMessage` falls back to the built-in default.

## 7. Error handling — scoped fail-closed

The plugin fails **closed** (blocks on error), but only once it is certain the
message is a private one. This keeps strictness where it matters without risking
a server-wide posting outage.

- **`GetChannel` fails (channel type unknown):** **allow** + warn-log. We cannot
  tell whether the message is a DM/GM; blocking here would block *all* channels
  during a transient error. This is the one deliberate fail-open point.
- **Any error after the channel is confirmed to be DM/GM** (author `GetUser`,
  recipient `GetUser`, group `GetUsersInChannel`): **reject** (fail closed) and
  warn-log. We know it is a private message and cannot confirm an exemption, so
  we err toward blocking.
- **Unknown/edge channel types:** only `Direct` and `Group` are ever blocked;
  everything else is allowed by default.

## 8. Testing

Unit tests in `message_hooks_test.go` using
`github.com/mattermost/mattermost/server/public/plugin/plugintest` mocks for
`p.API`. Each test mocks `GetChannel`/`GetUser` and asserts the returned
rejection string (empty = allowed). Matrix:

| Case | Channel | Participants | Expect |
|------|---------|--------------|--------|
| DM between two humans | Direct | human → human | rejected |
| GM of humans, blocking on | Group | humans | rejected |
| GM, `BlockGroupMessages=false` | Group | humans | allowed |
| Public/private channel | Open/Private | human | allowed |
| Bot author in DM | Direct | bot → human | allowed |
| **Bot recipient** in DM | Direct | human → bot | allowed |
| System post in DM | Direct | `system_*` | allowed |
| Webhook post, `AllowWebhookMessages=true` | Direct | human + `from_webhook` | allowed |
| Webhook post, `AllowWebhookMessages=false` | Direct | human + `from_webhook` | rejected |
| Admin author in DM, `AllowAdmins=true` | Direct | admin → human | allowed |
| **Admin recipient** in DM, `AllowAdmins=true` | Direct | human → admin | allowed |
| **Admin member** in GM, `AllowAdmins=true` | Group | humans + admin | allowed |
| Admin author in DM, `AllowAdmins=false` | Direct | admin → human | rejected |
| Admin recipient in DM, `AllowAdmins=false` | Direct | human → admin | rejected |
| Self-DM, `AllowSelfMessages=true` | Direct (self) | human | allowed |
| Self-DM, `AllowSelfMessages=false` | Direct (self) | human | rejected |
| `GetChannel` error | unknown | — | allowed (fail open) |
| `GetUser`/members error in confirmed DM/GM | Direct/Group | human | rejected (fail closed) |

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
