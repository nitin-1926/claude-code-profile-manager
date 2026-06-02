## How it works

`ccpm run <profile>` materializes the profile (merges settings + MCP fragments, links host-cascaded assets), sets `CLAUDE_CONFIG_DIR` to the profile dir, and execs `claude`. Each terminal gets a completely isolated Claude Code instance. No daemons, no patches.

### Asset resolution (higher wins)

1. **Project**: `<repo>/.claude/<asset>/` and `<repo>/.claude/settings.json`. Native Claude Code's loader already gives this priority.
2. **Profile**: assets installed via `ccpm <asset> add --profile <name>`.
3. **Global**: assets installed via `ccpm <asset> add --global` (live in `~/.ccpm/share/`).
4. **Host cascade**: anything in `~/.claude/<asset>/` (a `/plugin install` inside a session, `npx skills add`, manual edits) auto-adopts into every profile at launch. Opt out with `ccpm config set cascade_auto_adopt false` or `--no-auto-adopt` on `run` / `sync`. Profile-local always wins over host; `ccpm doctor` flags shadowed names.

### Settings merge order (lowest to highest)

1. The profile's existing `<profile>/settings.json` (preserves keys Claude wrote itself).
2. `~/.claude/settings.json` (shared baseline; edit directly to change every profile).
3. `~/.ccpm/share/settings/<profile>.json` (ccpm per-profile fragment).
4. Owned-keys sidecar (values explicitly set via `ccpm settings set --profile`; protects them from being silently overwritten).
5. `./.claude/settings.json` at the project root.
6. `./.claude/settings.local.json` at the project root (gitignored local overrides).
7. Enterprise managed-settings (`/Library/Application Support/ClaudeCode/managed-settings.json` on macOS; `/etc/claude-code/managed-settings.json` on Linux; `C:\ProgramData\ClaudeCode\managed-settings.json` on Windows; plus `managed-settings.d/*.json` drop-ins merged alphabetically). Highest precedence so org policy always wins.

MCP merge order: host `~/.claude.json#mcpServers` → ccpm global fragment → ccpm profile fragment → project `.claude/settings.json#mcpServers` → project `.mcp.json` → managed `mcpServers`.

Objects merge key-by-key; arrays and scalars from a higher-precedence source replace the lower one.

### Directory layout

```
~/.ccpm/
├── config.json          # profile registry; cascade_auto_adopt, check_default_drift
├── installs.json        # manifest of installed assets (skill/agent/command/rule/hook/mcp/plugin)
│                          # entries carry scope: global | profile | host
├── share/
│   ├── skills/          # shared skills (symlinked into profiles)
│   ├── agents/ commands/ rules/ hooks/
│   ├── mcp/             # MCP fragments: global.json + <profile>.json
│   └── settings/        # per-profile settings fragments
├── profiles/
│   ├── personal/        # CLAUDE_CONFIG_DIR for "personal"
│   └── work/
└── vault/               # encrypted credential backups
```

## Privacy and security

ccpm is 100% local. It never makes network requests, collects data, or phones home.

- API keys live in the OS keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager).
- Vault backups use AES-256-GCM with a master key in your OS keychain.
- All data lives in `~/.ccpm/`. No telemetry, no analytics, no tracking.

## Platform support

> **macOS is the verified platform today.** Linux and Windows builds compile, install, and run, but the OAuth-isolation paths (`set-default`, `auth backup/restore`, keychain-based `status`) are **experimental** until exercised against a real Linux Secret Service or Windows Credential Manager install.

| Feature            | macOS ✓ verified                         | Windows ⚠ experimental                                | Linux ⚠ experimental |
| ------------------ | ---------------------------------------- | ----------------------------------------------------- | -------------------- |
| OAuth per-profile  | Keychain entry namespaced by profile dir | wincred entry namespaced by profile dir (theoretical) | `.credentials.json`  |
| API key storage    | Keychain                                 | Credential Manager                                    | Secret Service       |
| Parallel sessions  | Yes                                      | Yes                                                   | Yes                  |
| Shared asset dedup | Symlinks                                 | Symlinks (Developer Mode) or copy[^1]                 | Symlinks             |
| Shell hook         | zsh, bash, fish                          | PowerShell                                            | zsh, bash, fish      |
| `set-default` (system-wide GUI) | LaunchAgent + `launchctl setenv` | Keychain / identity only                | Keychain / identity only |

[^1]: Without Developer Mode or admin, Windows cannot create symlinks; ccpm falls back to copying and writes a marker at `~/.ccpm/.windows-copy-fallback`. Turn on Developer Mode for true deduplication.

> Requires **Claude Code v2.1.56+** on macOS for OAuth isolation. Earlier versions share a single keychain entry across profiles. `ccpm doctor` warns when your Claude Code is too old.

## MCP authentication model

How an MCP server authenticates determines whether ccpm can isolate it per profile.

1. **Env-var MCPs** (e.g. `GITHUB_TOKEN`): isolated. Stored in the per-profile fragment at `~/.ccpm/share/mcp/<profile>.json`. Configure with `ccpm mcp add <name> --env KEY=VALUE --profile <p>`.
2. **OAuth MCPs** (open a browser, cache tokens in `.claude.json#mcpOAuth`): isolated, because `CLAUDE_CONFIG_DIR` is per-profile.
3. **Globally-cached MCPs** (servers that write to `~/.config/<service>` or a non-namespaced OS keychain entry): **shared across profiles**. ccpm cannot isolate them without cooperation from the server.

## Known limitations

- **IDE extensions ignore `CLAUDE_CONFIG_DIR`**: VS Code, Cursor, and Antigravity launch claude directly without going through a shell. Use `ccpm set-default <profile>` to pin one for them. On macOS this is system-wide via a LaunchAgent; on Linux and Windows the keychain and identity sync apply but the system-wide env mechanism is not yet implemented (terminal use is covered by `ccpm shell-init`).
- **Headless Linux**: `go-keyring` requires D-Bus and a secret service (gnome-keyring or kwallet). API-key profiles on headless servers need a running secret service.
- **Globally-cached MCP servers** cannot be isolated per profile (see the MCP auth section).

## Troubleshooting / FAQ

### `/plugin install` fails with `git@github.com: Permission denied (publickey)`

Claude Code clones plugin marketplaces over SSH by default. If you authenticate over HTTPS only, force git to rewrite SSH URLs to HTTPS:

```sh
git config --global url."https://github.com/".insteadOf "git@github.com:"
```

This applies to every tool that uses git.

### "Keychain access denied" / repeated keychain prompts

ccpm stores credentials in the OS keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager). If access is denied, allow `ccpm` (and `claude`) to read the relevant keychain items when prompted. On macOS you can inspect the namespaced entry name via `ccpm doctor`.

### "Claude Code too old (needs 2.1.56+)" on macOS OAuth

Per-profile OAuth isolation on macOS needs Claude Code v2.1.56+ (older builds share a single keychain entry across profiles). Run `ccpm doctor` to see your installed version, then update Claude Code.

### Broken or dangling symlinks

Run `ccpm doctor` to detect them, then `ccpm doctor --fix` to prune dangling shared-asset symlinks. For deeper asset drift across host, share, and profile scopes, use `ccpm consolidate --fix`.

### My IDE still uses the wrong account

After `ccpm set-default <profile>`, already-running IDE windows (VS Code, Cursor, Antigravity) need **one restart** to pick up the new `CLAUDE_CONFIG_DIR`. Future launches and logins are automatic.

### Running sessions don't see my change

`ccpm set-default`, `ccpm rename`, and `ccpm remove` don't affect already-running Claude Code sessions. Restart the session (and any IDE window) to pick up the change.

## Uninstall

`ccpm uninstall` removes every profile, deletes API keys from the OS keychain, wipes vault backups, and deletes `~/.ccpm/`. It does **not** remove the `ccpm` binary itself or the shell hook line you added to `~/.zshrc` / `~/.bashrc`. The command prints those cleanup steps so you can run them by hand.

```bash
# with confirmation prompt
ccpm uninstall

# skip the confirmation
ccpm uninstall --force
```

## Build from source

```bash
git clone https://github.com/nitin-1926/claude-code-profile-manager.git
cd claude-code-profile-manager/ccpm
go build -o ccpm .
./ccpm --version
```

## Releasing

`scripts/release.sh` handles the end-to-end release (bump, verify, tag, GitHub Release, npm publish) with preflight checks (git/go/node/npm/gh on PATH, on `main`, clean tree, in sync with `origin/main`, logged in to `gh` and npm, target tag unused). Run `./scripts/release.sh --help` for the full flag list (`patch`/`minor`/`major`/explicit version, `--dry-run`, `--stash`, `--allow-dirty`, `--skip-tests`, `--skip-npm`, `-y`).

## Contributing

Contributions welcome. Open an issue first to discuss what you want to change.

1. Fork the repo.
2. Create a feature branch (`git checkout -b feature/your-feature`).
3. Make your changes and run the tests (`cd ccpm && go test ./...`).
4. Open a pull request.

## License

MIT

## Author

Built by [Nitin Gupta](https://x.com/nitingupta__7).
