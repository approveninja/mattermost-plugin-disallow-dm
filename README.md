# Mattermost Plugin: Disallow DM

A Mattermost **server plugin** that blocks users from sending each other
**direct messages (DMs)** and **group messages (GMs)**, so conversations happen
in regular channels instead.

The block is enforced **server-side** and cannot be bypassed by any client
(web, desktop, mobile, or API). Bots, integrations, and system notifications
keep working normally.

> **Status:** in development. See the design spec in
> [`docs/superpowers/specs/2026-06-18-disallow-dm-design.md`](docs/superpowers/specs/2026-06-18-disallow-dm-design.md).

## How it works

The plugin uses the `MessageWillBePosted` server hook to reject any post made in
a DM or GM channel, unless an exception applies.

### Exceptions (allowed through)

- **Bots and integrations** — so notifications and other plugins still work.
- **System messages** (`system_*` post types).
- **System administrators** — configurable.
- **Messages to yourself** (self-notes) — configurable.

## Configuration

Configured in the System Console under **Plugins → Disallow DM**:

| Setting | Default | Description |
|---|---|---|
| Rejection message | "Direct messages are disabled by your administrator." | Text shown when a DM/GM is blocked. |
| Block group messages | on | Also block group messages (GMs). |
| Allow admins | on | Let system administrators send DMs/GMs. |
| Allow self-messages | on | Allow messages to yourself (notes). |

## Building

```sh
make dist
```

This produces a plugin bundle under `dist/` that can be uploaded in the System
Console (**Plugins → Plugin Management**).

## Development

Scaffolded from the official
[mattermost-plugin-starter-template](https://github.com/mattermost/mattermost-plugin-starter-template).
This is a server-only plugin; the `webapp/` directory is kept as a stub for a
possible future UI layer.

## License

See [LICENSE](LICENSE).
