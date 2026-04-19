# go-sequence devcontainer

Sandboxed development environment. Go 1.25, Claude Code, firewall-limited network.
Intended use: run `claude --dangerously-skip-permissions` (auto mode) inside
the container without risk to the host filesystem.

## Setup (first time)

1. Install Docker Desktop and VS Code.
2. Install the **Dev Containers** extension in VS Code (`ms-vscode-remote.remote-containers`).
3. Open this repo in VS Code.
4. Command palette → **Dev Containers: Reopen in Container**. First build takes ~5 min.
5. Inside the container terminal, authenticate Claude once — persists via a volume
   (`go-sequence-claude-config-*`), so you won't re-auth after rebuilds.
6. Verify: `go build ./...` should succeed.

## What's inside

- Node 20 (for Claude Code)
- Go 1.25.4 with full C/C++ toolchain for gomidi's `rtmididrv`
  (ALSA + JACK dev headers)
- `gh`, `git`, `delta`, `zsh`, `fzf`, standard CLI tools
- Persistent volumes: bash history, Claude config, Go build cache,
  Go module cache

## What's limited

- **No USB MIDI passthrough.** Docker on macOS can't expose USB MIDI to the
  container. Code builds and headless tests run fine; real MIDI hardware
  testing must happen outside the container.
- **Firewall default-deny.** Only a short whitelist of domains is reachable:
  GitHub, npm, Anthropic, GitLab (for gomidi), Go proxy/sum/dev, VS Code
  marketplace. Everything else is rejected. See `init-firewall.sh` to add
  domains.

## Auto mode

Once inside the container, run:

```
claude --dangerously-skip-permissions
```

The sandbox makes this reasonable — worst case, a rogue action can only
modify files in `/workspace` (bind-mounted to this repo on your host) and
write things to volumes. It cannot touch the rest of your Mac. It can still
push to any GitHub repo your credentials reach, so consider scoping a
dedicated token for the container, or just review commits before pushing.

## Files

- `devcontainer.json` — VS Code + container wiring, mounts, env
- `Dockerfile` — image build, based on Anthropic's reference + Go toolchain
- `init-firewall.sh` — iptables/ipset default-deny with whitelist, runs at
  container start
