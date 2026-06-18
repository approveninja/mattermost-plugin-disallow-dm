# Disallow DM Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Mattermost server plugin that blocks human-to-human direct (DM) and group (GM) messages, with configurable exceptions for admins and self-notes, and always-on exceptions for bots and system messages.

**Architecture:** A single server-side `MessageWillBePosted` hook inspects each post's channel type and participants and rejects human-to-human DMs/GMs. The repo is the official `mattermost-plugin-starter-template` with all unused scaffolding (commands, KV store, background job, HTTP router, webapp bundle) stripped down to: manifest + `Plugin` struct + configuration + the hook.

**Tech Stack:** Go 1.25, `github.com/mattermost/mattermost/server/public` (plugin API + model), `plugintest` mocks, `testify` for assertions. No webapp/JS.

## Global Constraints

- **Language:** All code, comments, docs, and commit messages MUST be in English.
- **Go toolchain:** Go **1.25+** must be installed and on `PATH` (matches `go.mod`). Network access is required for `go mod tidy` and `go test` (module downloads).
- **Plugin ID:** `com.approveninja.disallow-dm` (set in `plugin.json`).
- **Go module path:** `github.com/approveninja/mattermost-plugin-disallow-dm`.
- **Blocking semantics:** Only **human-to-human** DM and GM posts are blocked. Bots (either side) and `system_*` posts are **always** exempt. System admins (either side) and self-DMs are exempt **when their config toggle is on**.
- **Error policy (scoped fail-closed):** If `GetChannel` fails → **allow** (we cannot confirm it is a DM/GM). Once the channel is confirmed to be a DM/GM, any later lookup error (`GetUser`, `GetUsersInChannel`) → **reject**.
- **API signatures are unverified in this environment** (Go is not installed here). Treat the TDD compile/run steps as the safety net: if a signature or constant differs in the pinned module version, adjust at first compile. Known fallback: if `model.ChannelSortByUsername` does not exist, use the string literal `"username"`.
- **Spec:** `docs/superpowers/specs/2026-06-18-disallow-dm-design.md` (approved). This plan implements it.

---

## Task 1: Strip the template to a compiling server-only baseline

Remove all unused starter scaffolding and re-identify the plugin so it compiles cleanly with only the pieces this plugin needs.

**Files:**
- Modify: `plugin.json` (identity; remove webapp section)
- Modify: `go.mod` (module path)
- Rewrite: `server/plugin.go` (minimal `Plugin` struct + lifecycle)
- Delete: `server/api.go`, `server/job.go`, `server/plugin_test.go`, `server/command/` (dir), `server/store/` (dir)
- Unchanged: `server/main.go`, `server/configuration.go`

**Interfaces:**
- Produces: `type Plugin struct{ plugin.MattermostPlugin; configurationLock sync.RWMutex; configuration *configuration }` and the existing `getConfiguration()/setConfiguration()/OnConfigurationChange()` from `configuration.go`. Later tasks attach methods (`MessageWillBePosted`, helpers) to `*Plugin`.

- [ ] **Step 1: Ensure Go 1.25+ is available**

Run: `go version`
Expected: `go version go1.25.x ...`. If `go: command not found`, install it (Linux amd64):

```bash
curl -fsSL https://go.dev/dl/go1.25.1.linux-amd64.tar.gz -o /tmp/go.tgz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tgz
export PATH=$PATH:/usr/local/go/bin
go version
```

- [ ] **Step 2: Delete unused scaffolding files**

```bash
cd /home/alexeysilver/claudecode/mattermost-plugin-disallow-dm
git rm -r server/api.go server/job.go server/plugin_test.go server/command server/store
```

- [ ] **Step 3: Rewrite `server/plugin.go` to the minimal struct**

Replace the entire file `server/plugin.go` with:

```go
package main

import (
	"sync"

	"github.com/mattermost/mattermost/server/public/plugin"
)

// Plugin implements the interface expected by the Mattermost server to
// communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration
	// and setConfiguration for usage.
	configuration *configuration
}
```

- [ ] **Step 4: Re-identify the plugin in `plugin.json`**

Replace the top identity fields and remove the `webapp` block. The file becomes:

```json
{
    "id": "com.approveninja.disallow-dm",
    "name": "Disallow DM",
    "description": "Blocks direct and group messages between users so conversations happen in channels.",
    "homepage_url": "https://github.com/approveninja/mattermost-plugin-disallow-dm",
    "support_url": "https://github.com/approveninja/mattermost-plugin-disallow-dm/issues",
    "icon_path": "assets/starter-template-icon.svg",
    "min_server_version": "6.2.1",
    "server": {
        "executables": {
            "linux-amd64": "server/dist/plugin-linux-amd64",
            "linux-arm64": "server/dist/plugin-linux-arm64",
            "darwin-amd64": "server/dist/plugin-darwin-amd64",
            "darwin-arm64": "server/dist/plugin-darwin-arm64",
            "windows-amd64": "server/dist/plugin-windows-amd64.exe"
        }
    },
    "settings_schema": {
        "header": "",
        "footer": "",
        "settings": []
    }
}
```

(Removing the `webapp` key sets `HAS_WEBAPP` empty, so the build skips all webapp/Node steps. `settings_schema.settings` is filled in Task 2.)

- [ ] **Step 5: Change the module path in `go.mod`**

Change the first line of `go.mod` from
`module github.com/mattermost/mattermost-plugin-starter-template`
to
`module github.com/approveninja/mattermost-plugin-disallow-dm`

- [ ] **Step 6: Tidy modules and verify it builds**

Run:
```bash
cd /home/alexeysilver/claudecode/mattermost-plugin-disallow-dm
go mod tidy
go build ./server/...
go vet ./server/...
```
Expected: all succeed with no output (or only download logs). `go mod tidy` removes now-unused deps (gorilla/mux, pluginapi/cluster, go.uber.org/mock, etc.).

- [ ] **Step 7: Run the test suite (should be empty and pass)**

Run: `go test ./server/...`
Expected: `ok` / `no test files` — no failures.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "chore: strip starter scaffolding to server-only baseline

Remove commands, KV store, background job, HTTP router and webapp; rename
module and plugin id; reduce Plugin to config + hooks."
```

---

## Task 2: Add configuration fields, settings schema, and rejection-message default

Define the four settings, expose them in the System Console, and add a helper that supplies the default rejection message when the admin leaves it blank.

**Files:**
- Modify: `server/configuration.go` (add fields, `strings` import, helper + const)
- Modify: `plugin.json` (`settings_schema.settings`)
- Create: `server/configuration_test.go`

**Interfaces:**
- Consumes: `Plugin` from Task 1.
- Produces:
  - `type configuration struct { RejectionMessage string; BlockGroupMessages bool; AllowAdmins bool; AllowSelfMessages bool }`
  - `const defaultRejectionMessage = "Direct messages are disabled by your administrator."`
  - `func (c *configuration) rejectionMessageOrDefault() string`

- [ ] **Step 1: Write the failing test**

Create `server/configuration_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRejectionMessageOrDefault(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"empty returns default":      {"", defaultRejectionMessage},
		"whitespace returns default": {"   ", defaultRejectionMessage},
		"custom is returned as-is":   {"No DMs allowed here", "No DMs allowed here"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := &configuration{RejectionMessage: tc.in}
			assert.Equal(t, tc.want, c.rejectionMessageOrDefault())
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/ -run TestRejectionMessageOrDefault -v`
Expected: FAIL — `undefined: defaultRejectionMessage` and `c.rejectionMessageOrDefault undefined`.

- [ ] **Step 3: Add fields, const, and helper to `server/configuration.go`**

Add `"strings"` to the import block. Replace `type configuration struct{}` with:

```go
type configuration struct {
	// RejectionMessage is shown to a user when their DM/GM is blocked.
	RejectionMessage string

	// BlockGroupMessages also blocks group messages (3-7 people), not just 1-to-1 DMs.
	BlockGroupMessages bool

	// AllowAdmins allows DMs/GMs whenever a system admin is involved (sender or recipient).
	AllowAdmins bool

	// AllowSelfMessages allows a user to message their own DM channel (personal notes).
	AllowSelfMessages bool
}

const defaultRejectionMessage = "Direct messages are disabled by your administrator."

// rejectionMessageOrDefault returns the configured rejection message, or the
// built-in default when the admin left it blank.
func (c *configuration) rejectionMessageOrDefault() string {
	if strings.TrimSpace(c.RejectionMessage) == "" {
		return defaultRejectionMessage
	}
	return c.RejectionMessage
}
```

(The existing `Clone()` stays a shallow copy — all fields are value types.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/ -run TestRejectionMessageOrDefault -v`
Expected: PASS (all three subtests).

- [ ] **Step 5: Fill in `settings_schema.settings` in `plugin.json`**

Replace the `"settings": []` from Task 1 with:

```json
        "settings": [
            {
                "key": "RejectionMessage",
                "display_name": "Rejection message",
                "type": "text",
                "help_text": "Message shown to a user when their direct or group message is blocked. Leave blank to use the default.",
                "default": "Direct messages are disabled by your administrator."
            },
            {
                "key": "BlockGroupMessages",
                "display_name": "Block group messages",
                "type": "bool",
                "help_text": "Also block group messages (3-7 people), not just 1-to-1 direct messages.",
                "default": true
            },
            {
                "key": "AllowAdmins",
                "display_name": "Allow system administrators",
                "type": "bool",
                "help_text": "Allow direct/group messages whenever a system administrator is involved (sender or recipient).",
                "default": true
            },
            {
                "key": "AllowSelfMessages",
                "display_name": "Allow messages to yourself",
                "type": "bool",
                "help_text": "Allow a user to send messages to their own direct message channel (personal notes).",
                "default": true
            }
        ]
```

The `key` values map case-sensitively to the Go struct field names via `LoadPluginConfiguration`. The `default` values are applied by the server on a fresh install; this is verified manually in Task 4.

- [ ] **Step 6: Verify manifest is still valid and builds**

Run:
```bash
go build ./server/...
./build/bin/manifest check || go run ./build/manifest check
```
Expected: build succeeds; manifest check reports no errors (it validates `plugin.json`).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: add configuration settings and rejection-message default"
```

---

## Task 3: Implement the MessageWillBePosted hook

Add the admin helper and the hook that enforces the blocking rules, fully test-driven against the spec's decision matrix.

**Files:**
- Create: `server/message_hooks.go`
- Create: `server/message_hooks_test.go`

**Interfaces:**
- Consumes: `Plugin`, `getConfiguration()`, `configuration` fields, `rejectionMessageOrDefault()` from Tasks 1-2.
- Produces:
  - `func isSystemAdmin(user *model.User) bool`
  - `func (p *Plugin) MessageWillBePosted(c *plugin.Context, post *model.Post) (*model.Post, string)`
  - `func (p *Plugin) otherParticipants(channel *model.Channel, authorID string) ([]*model.User, *model.AppError)`

- [ ] **Step 1: Write the failing tests**

Create `server/message_hooks_test.go`:

```go
package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	humanID  = "humanaaaaaaaaaaaaaaaaaaaaaa"
	human2ID = "humanbbbbbbbbbbbbbbbbbbbbbb"
	botID    = "botccccccccccccccccccccccccc"
	adminID  = "adminddddddddddddddddddddddd"
)

func human(id string) *model.User  { return &model.User{Id: id, Roles: "system_user"} }
func bot(id string) *model.User    { return &model.User{Id: id, Roles: "system_user", IsBot: true} }
func admin(id string) *model.User  { return &model.User{Id: id, Roles: "system_user system_admin"} }

func dmChannel(a, b string) *model.Channel {
	return &model.Channel{Id: "channel_dm", Type: model.ChannelTypeDirect, Name: model.GetDMNameFromIds(a, b)}
}
func gmChannel() *model.Channel {
	return &model.Channel{Id: "channel_gm", Type: model.ChannelTypeGroup, Name: "groupchannel"}
}
func openChannel() *model.Channel {
	return &model.Channel{Id: "channel_open", Type: model.ChannelTypeOpen, Name: "town-square"}
}

func defaultConfig() *configuration {
	return &configuration{BlockGroupMessages: true, AllowAdmins: true, AllowSelfMessages: true}
}

// registerUsers wires GetUser for all canonical users as optional expectations.
func registerUsers(api *plugintest.API) {
	api.On("GetUser", humanID).Return(human(humanID), nil).Maybe()
	api.On("GetUser", human2ID).Return(human(human2ID), nil).Maybe()
	api.On("GetUser", botID).Return(bot(botID), nil).Maybe()
	api.On("GetUser", adminID).Return(admin(adminID), nil).Maybe()
}

func newAPI() *plugintest.API {
	api := &plugintest.API{}
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	return api
}

func runHook(cfg *configuration, api *plugintest.API, post *model.Post) string {
	p := &Plugin{configuration: cfg}
	p.API = api
	_, msg := p.MessageWillBePosted(&plugin.Context{}, post)
	return msg
}

func TestMessageWillBePosted(t *testing.T) {
	cases := []struct {
		name         string
		cfg          *configuration
		setup        func(api *plugintest.API)
		post         *model.Post
		wantRejected bool
	}{
		{
			name: "DM human to human is rejected",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, human2ID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: true,
		},
		{
			name: "GM of humans is rejected when blocking on",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_gm").Return(gmChannel(), nil)
				api.On("GetUsersInChannel", "channel_gm", mock.Anything, mock.Anything, mock.Anything).
					Return([]*model.User{human(humanID), human(human2ID)}, nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_gm"},
			wantRejected: true,
		},
		{
			name: "GM allowed when BlockGroupMessages is false",
			cfg:  &configuration{BlockGroupMessages: false, AllowAdmins: true, AllowSelfMessages: true},
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_gm").Return(gmChannel(), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_gm"},
			wantRejected: false,
		},
		{
			name: "public channel is allowed",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				api.On("GetChannel", "channel_open").Return(openChannel(), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_open"},
			wantRejected: false,
		},
		{
			name: "bot author in DM is allowed",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(botID, humanID), nil)
			},
			post:         &model.Post{UserId: botID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "bot recipient in DM is allowed",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, botID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "system post in DM is allowed",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, human2ID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm", Type: "system_join_channel"},
			wantRejected: false,
		},
		{
			name: "admin author allowed when AllowAdmins on",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(adminID, humanID), nil)
			},
			post:         &model.Post{UserId: adminID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "admin recipient allowed when AllowAdmins on",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, adminID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "admin member in GM allowed when AllowAdmins on",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_gm").Return(gmChannel(), nil)
				api.On("GetUsersInChannel", "channel_gm", mock.Anything, mock.Anything, mock.Anything).
					Return([]*model.User{human(humanID), human(human2ID), admin(adminID)}, nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_gm"},
			wantRejected: false,
		},
		{
			name: "admin author rejected when AllowAdmins off",
			cfg:  &configuration{BlockGroupMessages: true, AllowAdmins: false, AllowSelfMessages: true},
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(adminID, humanID), nil)
			},
			post:         &model.Post{UserId: adminID, ChannelId: "channel_dm"},
			wantRejected: true,
		},
		{
			name: "admin recipient rejected when AllowAdmins off",
			cfg:  &configuration{BlockGroupMessages: true, AllowAdmins: false, AllowSelfMessages: true},
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, adminID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: true,
		},
		{
			name: "self-DM allowed when AllowSelfMessages on",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, humanID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "self-DM rejected when AllowSelfMessages off",
			cfg:  &configuration{BlockGroupMessages: true, AllowAdmins: true, AllowSelfMessages: false},
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, humanID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: true,
		},
		{
			name: "GetChannel error fails open (allowed)",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				api.On("GetChannel", "channel_dm").
					Return(nil, model.NewAppError("GetChannel", "boom", nil, "boom", 500))
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "author lookup error in confirmed DM fails closed (rejected)",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, human2ID), nil)
				api.On("GetUser", humanID).
					Return(nil, model.NewAppError("GetUser", "boom", nil, "boom", 500))
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newAPI()
			tc.setup(api)
			msg := runHook(tc.cfg, api, tc.post)
			if tc.wantRejected {
				assert.NotEmpty(t, msg, "expected the post to be rejected")
			} else {
				assert.Empty(t, msg, "expected the post to be allowed")
			}
		})
	}
}

func TestIsSystemAdmin(t *testing.T) {
	assert.True(t, isSystemAdmin(&model.User{Roles: "system_user system_admin"}))
	assert.False(t, isSystemAdmin(&model.User{Roles: "system_user"}))
	assert.False(t, isSystemAdmin(nil))
}
```

Note: the "author lookup error" case deliberately does NOT call `registerUsers`, so its error expectation for `GetUser(humanID)` is the only one registered.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/ -run 'TestMessageWillBePosted|TestIsSystemAdmin' -v`
Expected: FAIL — `MessageWillBePosted`, `isSystemAdmin`, `otherParticipants` undefined (compile error).

- [ ] **Step 3: Implement `server/message_hooks.go`**

```go
package main

import (
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const systemAdminRole = "system_admin"

// isSystemAdmin reports whether the user holds the system admin role.
func isSystemAdmin(user *model.User) bool {
	if user == nil {
		return false
	}
	for _, role := range strings.Fields(user.Roles) {
		if role == systemAdminRole {
			return true
		}
	}
	return false
}

// MessageWillBePosted blocks human-to-human direct and group messages.
// A non-empty returned string rejects the post and is shown to the user.
func (p *Plugin) MessageWillBePosted(c *plugin.Context, post *model.Post) (*model.Post, string) {
	cfg := p.getConfiguration()

	// 1. Look up the channel. If the type is unknown, fail open: blocking here
	//    would block posting in every channel during a transient error.
	channel, appErr := p.API.GetChannel(post.ChannelId)
	if appErr != nil {
		p.API.LogWarn("disallow-dm: get channel failed, allowing post", "channel_id", post.ChannelId, "error", appErr.Error())
		return post, ""
	}

	// 2. Only direct and group messages are ever blocked.
	if channel.Type != model.ChannelTypeDirect && channel.Type != model.ChannelTypeGroup {
		return post, ""
	}

	// 3. Group messages are blocked only when enabled.
	if channel.Type == model.ChannelTypeGroup && !cfg.BlockGroupMessages {
		return post, ""
	}

	// 4. Pure exceptions (no API call).
	if strings.HasPrefix(post.Type, "system_") {
		return post, ""
	}
	if cfg.AllowSelfMessages && channel.Type == model.ChannelTypeDirect &&
		channel.GetOtherUserIdForDM(post.UserId) == "" {
		return post, ""
	}

	// From here the channel is a confirmed DM/GM: lookup errors fail closed.

	// 5. Author exceptions.
	author, appErr := p.API.GetUser(post.UserId)
	if appErr != nil {
		p.API.LogWarn("disallow-dm: get author failed, blocking post", "user_id", post.UserId, "error", appErr.Error())
		return nil, cfg.rejectionMessageOrDefault()
	}
	if author.IsBot {
		return post, ""
	}
	if cfg.AllowAdmins && isSystemAdmin(author) {
		return post, ""
	}

	// 6. Receiving-side exceptions: a bot anywhere, or (when enabled) an admin.
	participants, appErr := p.otherParticipants(channel, post.UserId)
	if appErr != nil {
		p.API.LogWarn("disallow-dm: get participants failed, blocking post", "channel_id", channel.Id, "error", appErr.Error())
		return nil, cfg.rejectionMessageOrDefault()
	}
	for _, u := range participants {
		if u.IsBot {
			return post, ""
		}
		if cfg.AllowAdmins && isSystemAdmin(u) {
			return post, ""
		}
	}

	// 7. Human-to-human DM/GM: reject.
	return nil, cfg.rejectionMessageOrDefault()
}

// otherParticipants returns the users on the other side of a DM/GM, excluding
// the author. For a self-DM it returns no users.
func (p *Plugin) otherParticipants(channel *model.Channel, authorID string) ([]*model.User, *model.AppError) {
	if channel.Type == model.ChannelTypeDirect {
		otherID := channel.GetOtherUserIdForDM(authorID)
		if otherID == "" {
			return nil, nil
		}
		user, appErr := p.API.GetUser(otherID)
		if appErr != nil {
			return nil, appErr
		}
		return []*model.User{user}, nil
	}

	members, appErr := p.API.GetUsersInChannel(channel.Id, model.ChannelSortByUsername, 0, 100)
	if appErr != nil {
		return nil, appErr
	}
	others := make([]*model.User, 0, len(members))
	for _, u := range members {
		if u.Id != authorID {
			others = append(others, u)
		}
	}
	return others, nil
}
```

If the module version lacks `model.ChannelSortByUsername`, replace it with the literal `"username"`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/ -run 'TestMessageWillBePosted|TestIsSystemAdmin' -v`
Expected: PASS — all 16 `TestMessageWillBePosted` subtests and `TestIsSystemAdmin`.

- [ ] **Step 5: Run the full server suite and vet**

Run:
```bash
go test ./server/...
go vet ./server/...
```
Expected: `ok` and no vet warnings.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: block human-to-human DMs and GMs via MessageWillBePosted"
```

---

## Task 4: Lint, build the bundle, fix CI, and verify manually

Make the repo green end-to-end and confirm the runtime behavior — especially the settings defaults, which cannot be verified by unit tests.

**Files:**
- Modify: `.github/workflows/ci.yml` (branch `master` → `main`)

- [ ] **Step 1: Fix the CI trigger branch**

In `.github/workflows/ci.yml`, under `on.push.branches`, change `- master` to `- main` so CI runs on this repo's default branch.

- [ ] **Step 2: Run lint exactly as CI will**

Run:
```bash
cd /home/alexeysilver/claudecode/mattermost-plugin-disallow-dm
make check-style
```
Expected: passes. If `golangci-lint` flags issues in the new files, fix them and re-run. (`make check-style` installs the pinned Go tools; needs network.)

- [ ] **Step 3: Run the full test target**

Run: `make test`
Expected: server tests pass; no webapp step runs (`HAS_WEBAPP` is empty after Task 1).

- [ ] **Step 4: Build the distributable bundle**

Run: `make dist`
Expected: prints `plugin built at: dist/com.approveninja.disallow-dm-<version>.tar.gz` and no default-plugin-ID warning. Confirm the tarball exists:
```bash
ls dist/*.tar.gz
```

- [ ] **Step 5: Manual verification on a live server (REQUIRED — covers the settings-defaults unknown)**

Upload `dist/*.tar.gz` in **System Console → Plugins → Plugin Management**, enable it, then **without opening or saving the plugin's settings page** (this is what tests the schema defaults on a fresh install):

- [ ] Two non-admin users: opening a DM and sending a message → **blocked**, with the rejection message.
- [ ] A non-admin opens a group message (3+ people) and sends → **blocked** (confirms `BlockGroupMessages` default `true` was applied with no manual save).
- [ ] A non-admin DMs an **admin**, and the admin DMs a non-admin → **both allowed** (confirms `AllowAdmins` default `true`).
- [ ] A user sends a message to **themselves** (self-DM / notes) → **allowed** (confirms `AllowSelfMessages` default `true`).
- [ ] A user DMs a **bot** (e.g. a bot account or another plugin's bot) → **allowed**.
- [ ] Posting in a normal public/private channel → **allowed**.

If any default-driven case (GM/admin/self) behaves opposite to the table, the server did not apply `settings_schema` defaults on this version: open the plugin settings page once and save to persist them, and note this in the README's configuration section.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "ci: run on main branch; finalize build"
```

- [ ] **Step 7: Push**

```bash
git push origin main
```

---

## Self-Review

**Spec coverage:**
- §3 server-only enforcement → Task 3 hook. ✓
- §5 decision order (channel gate, GM toggle, system/self pure exceptions, author bot/admin, recipient bot/admin, reject) → Task 3 implementation + tests. ✓
- §6 four settings + default fallback → Task 2. ✓
- §7 scoped fail-closed (GetChannel allow; later lookups reject) → Task 3 steps + tests "GetChannel error" and "author lookup error". ✓
- §8 test matrix (all 15 rows) → Task 3 `TestMessageWillBePosted` (16 subtests incl. bot recipient). ✓
- §4 architecture (slim server-only, webapp stub) → Task 1. ✓
- §2 out-of-scope (no webapp, no per-team, edit limitation) → not implemented by design; edit limitation documented in spec + README. ✓

**Placeholder scan:** No TBD/TODO; every code and test step contains complete code; every command has expected output. ✓

**Type consistency:** `configuration` fields (`RejectionMessage`, `BlockGroupMessages`, `AllowAdmins`, `AllowSelfMessages`) and `rejectionMessageOrDefault()` defined in Task 2 are used identically in Task 3. `isSystemAdmin`, `otherParticipants`, and `MessageWillBePosted` signatures match between definition and tests. Channel/user constructors in the test file are self-consistent. ✓
