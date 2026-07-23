import type { Metadata } from "next";
import { Nav } from "../components/nav";
import { Footer } from "../components/footer";
import { CodeBlock } from "../components/code-block";
import { Callout } from "../components/callout";
import { DocsSidebar } from "../components/docs-sidebar";
import { DocsToc } from "../components/docs-toc";
import { InstallTabs } from "../components/install-tabs";
import { H2, H3 } from "../components/docs/section-headings";
import { DocsHero } from "../components/docs/docs-hero";
import {
  VERSION,
  DESKTOP_DMG,
  DESKTOP_RELEASES_URL,
  DESKTOP_VERSION,
} from "@/lib/version";

export const metadata: Metadata = {
  title: "ccpm Documentation",
  description: "Complete documentation for ccpm (Claude Code Profile Manager)",
};

export default async function DocsPage() {
  return (
    <>
      <Nav />
      <div className="max-w-7xl mx-auto px-6 pt-10 pb-16 flex gap-10">
        <DocsSidebar />

        <main id="main" className="flex-1 min-w-0 max-w-3xl prose-doc">
          <DocsHero />

          <H2 id="installation">Installation</H2>
          <p>
            Pick a package manager. ccpm ships as a single static binary, so any
            of these paths gets you to the same place.
          </p>

          <div className="not-prose my-5">
            <InstallTabs />
          </div>

          <H2 id="desktop">Desktop app</H2>
          <p>
            <strong>CCPM Desktop</strong> is a native macOS GUI over the CLI —
            built with <a href="https://wails.io" target="_blank" rel="noopener noreferrer">Wails</a> (Go + native webview) in the same module as{" "}
            <code>ccpm</code>, so it reuses ccpm&apos;s own engine for reads and
            shells out to the <code>ccpm</code> CLI for writes (same locking,
            keychain, and validation). A left sidebar of profiles, and per
            profile: <strong>Overview</strong>, <strong>Cascade</strong> (the
            effective host→global→profile config with provenance badges),{" "}
            <strong>Assets</strong>, <strong>MCP &amp; Plugins</strong>,{" "}
            <strong>Permissions</strong>, <strong>Usage</strong>, and{" "}
            <strong>Health</strong> (<code>ccpm doctor</code>). Clone, rename,
            delete, open, and run from the toolbar; the view auto-refreshes when
            the CLI changes things underneath it.
          </p>

          <H3 id="desktop-download">Download (macOS)</H3>
          <p>
            Grab the <code>.dmg</code> for your Mac (~3–4 MB each):{" "}
            <a href={DESKTOP_DMG.appleSilicon}>Apple Silicon</a> (
            <code>arm64</code>) or <a href={DESKTOP_DMG.intel}>Intel</a> (
            <code>amd64</code>). Older versions and checksums live on the{" "}
            <a href={DESKTOP_RELEASES_URL} target="_blank" rel="noopener noreferrer">all desktop releases</a> page. Open the{" "}
            <code>.dmg</code> and drag <strong>CCPM</strong> into{" "}
            <strong>Applications</strong>.
          </p>
          <Callout type="warn" title="First launch: Gatekeeper">
            The app is distributed unsigned (no Apple Developer account), so
            macOS asks you to approve it once on first launch.{" "}
            <strong>Right-click the app → Open</strong> (or, on Sequoia+,{" "}
            <strong>System Settings → Privacy &amp; Security → Open Anyway</strong>)
            — just once. It opens normally after that, and in-app updates install
            themselves without repeating this step.
          </Callout>
          <Callout type="info" title="Requires the ccpm CLI">
            The desktop app uses the <code>ccpm</code> CLI for all write
            actions — <a href="#installation">install it first</a> if you
            haven&apos;t. Reads work through the shared engine; writes shell out
            to the CLI.
          </Callout>
          <p>
            <strong>Updates are automatic.</strong> When a new build ships, the
            app shows an in-app <strong>Update now</strong> prompt, downloads it,
            verifies the SHA-256, and swaps itself in place — no re-downloading,
            no re-dragging, and no repeat of the Gatekeeper step. The desktop app
            versions independently of the CLI (on <code>desktop-v*</code> tags;
            current: <code>desktop-v{DESKTOP_VERSION}</code>).
          </p>

          <H3 id="desktop-build">Build from source</H3>
          <CodeBlock
            code={`go install github.com/wailsapp/wails/v2/cmd/wails@latest   # one-time: the Wails CLI

cd ccpm/desktop
wails build          # or from ccpm/: make desktop
make desktop-dev     # hot-reload dev window`}
            lang="bash"
          />

          <H2 id="quick-start">Quick start</H2>
          <p>
            Three commands and you have two completely separate Claude Code
            sessions running side by side.
          </p>
          <CodeBlock
            code={`# create your first profile
ccpm add personal

# create a work profile
ccpm add work

# run them in parallel
ccpm run personal   # terminal 1
ccpm run work       # terminal 2`}
            lang="bash"
          />

          <H2 id="profiles">Profile management</H2>

          <H3 id="profiles-add">ccpm add &lt;name&gt;</H3>
          <p>
            Create a new profile. If <code>~/.claude</code> exists or you
            already have at least one ccpm profile, an{" "}
            <strong>import wizard</strong> runs first so the new profile can
            start from your default Claude config or be cloned from an existing
            profile. Then you choose between OAuth (browser login) or API key
            authentication.
          </p>
          <CodeBlock
            code={`$ ccpm add personal
How do you want to seed this profile?
  1) Start empty
  2) Import from ~/.claude (skills, commands, hooks, agents, settings)
  3) Clone from another profile
Enter choice [1/2/3]: 2

Choose authentication method:
  1) OAuth (browser login via claude /login)
  2) API key (enter your Anthropic API key)
Enter choice [1/2]: 1

✓ profile "personal" authenticated via OAuth`}
            lang="bash"
          />

          <H3 id="profiles-list">ccpm list</H3>
          <p>
            List all profiles with their authentication status. Also available
            as <code>ccpm ls</code>. Pass <code>--json</code> for
            machine-readable output with stable snake_case fields (
            <code>name</code>, <code>auth_method</code>, <code>valid</code>,{" "}
            <code>status</code>, <code>default</code>, <code>last_used</code>,{" "}
            <code>expire_at</code>, <code>dir</code>) you can pipe into{" "}
            <code>jq</code> or another tool.
          </p>
          <CodeBlock
            code={`# human-readable table
ccpm list

# machine-readable JSON
ccpm list --json

# e.g. the default profile's name
ccpm list --json | jq -r '.[] | select(.default) | .name'`}
            lang="bash"
          />

          <H3 id="profiles-clone">ccpm clone &lt;source&gt; &lt;new&gt;</H3>
          <p>
            Duplicate a profile: its skills, agents, commands, rules, hooks, MCP
            servers, plugins, and settings are copied, and (unless{" "}
            <code>--no-auth</code>) its credentials are copied too. Handy for a
            throwaway/scratch copy of a profile you don&apos;t want to disturb.
          </p>
          <CodeBlock
            code={`# duplicate "work" into "work-scratch" (assets + settings + auth)
ccpm clone work work-scratch

# copy assets/settings only; leave the clone unauthenticated
ccpm clone work work-scratch --no-auth
ccpm auth refresh work-scratch`}
            lang="bash"
          />
          <Callout type="warn" title="OAuth clones share the source tokens">
            A cloned OAuth profile shares the source account&apos;s tokens, so
            when Claude rotates the refresh token in one, the other goes stale.
            For a clone you intend to use long-term against the same account,
            prefer <code>--no-auth</code> and run{" "}
            <code>ccpm auth refresh &lt;new&gt;</code> to give it its own login.
            API-key clones have no such caveat.
          </Callout>

          <H3 id="profiles-remove">ccpm remove &lt;name&gt;</H3>
          <p>
            Delete a profile including its directory, keychain entries, and
            vault backup. Use <code>--force</code> (<code>-f</code>) to skip
            confirmation. Also available as <code>ccpm rm</code>.
          </p>
          <CodeBlock
            code={`# with confirmation prompt
ccpm remove work

# skip the prompt
ccpm rm work --force`}
            lang="bash"
          />

          <H3 id="profiles-status">ccpm status</H3>
          <p>
            Show system overview: ccpm version, Claude binary location, all
            profiles and their auth health.
          </p>

          <H2 id="running">Running Claude</H2>

          <H3 id="running-run">ccpm run &lt;name&gt;</H3>
          <p>
            <strong>Recommended.</strong> Launch Claude Code with the given
            profile. Sets <code>CLAUDE_CONFIG_DIR</code> and{" "}
            <code>ANTHROPIC_API_KEY</code> (for API key profiles), then replaces
            the process with Claude. Works without any shell setup.
          </p>
          <p>
            Unknown flags after the profile name flow through to{" "}
            <code>claude</code> directly, with no <code>--</code> separator
            needed for the common cases. Four flags are intercepted by ccpm:{" "}
            <code>--ccpm-env KEY=VALUE</code> (repeatable, one-shot env
            override), <code>--no-auto-adopt</code> (skip the host-asset
            cascade scan for this launch), <code>--help</code>, and{" "}
            <code>--version</code>. Use <code>--</code> to forward{" "}
            <code>--help</code> or <code>--version</code> to claude.
          </p>
          <CodeBlock
            code={`# flags forward to claude without a separator
ccpm run work --dangerously-skip-permissions
ccpm run work --model claude-sonnet-4-6

# one-shot env override (persists nothing)
ccpm run work --ccpm-env ANTHROPIC_BASE_URL=https://proxy.example

# forward --help or --version to claude with the -- separator
ccpm run work -- --help
ccpm run work -- --version`}
            lang="bash"
          />

          <H3 id="running-use">ccpm use [name]</H3>
          <p>
            Set the active profile for your entire shell session. Requires the{" "}
            <a href="#shell">shell hook</a>. After running this, any{" "}
            <code>claude</code> command in that terminal uses the selected
            profile.
          </p>
          <p>
            Called without a name in an interactive terminal,{" "}
            <code>ccpm use</code> opens a profile picker. In non-TTY contexts
            (scripts, CI) the name argument is required.
          </p>

          <H2 id="auth">Authentication</H2>

          <H3 id="auth-status">ccpm auth status [name]</H3>
          <p>
            Check credential validity across profiles. Shows email for OAuth
            profiles, masked key for API key profiles, and vault backup status.
            Pass a profile name to inspect just that one; omit it to see every
            profile. Entries flagged as <code>⚠</code> expire within seven days.
          </p>

          <H3 id="auth-refresh">ccpm auth refresh &lt;name&gt;</H3>
          <p>
            Re-authenticate a profile. For OAuth: launches Claude for{" "}
            <code>/login</code>. For API key: prompts for a new key (hidden
            input in a TTY, or reads from stdin when piped).
          </p>

          <H3 id="auth-backup">ccpm auth backup / restore</H3>
          <p>
            Save an encrypted credential backup to <code>~/.ccpm/vault/</code>{" "}
            (AES-256-GCM, master key in the OS keychain) or restore one after a
            machine migration. See <a href="#vault">Vault backup</a> for the
            full story.
          </p>

          <H2 id="import">Import & wizard</H2>
          <p>
            ccpm has three ways to bring existing Claude assets into a profile:
            the interactive wizard that runs during <code>ccpm add</code>,{" "}
            <code>ccpm import default</code> for pulling from{" "}
            <code>~/.claude</code>, and <code>ccpm import from-profile</code>{" "}
            for cloning between ccpm profiles.
          </p>

          <H3 id="import-default">ccpm import default</H3>
          <p>
            Import skills, commands, hooks, agents, rules, settings, MCP
            servers, and plugins from <code>~/.claude</code> into one or all
            profiles. Dedupable targets (skills, agents, commands, hooks, rules)
            are routed through the shared store at <code>~/.ccpm/share/</code>{" "}
            and symlinked into the profile so the same asset is not copied
            twice.
          </p>
          <CodeBlock
            code={`# import everything into one profile
ccpm import default --profile work

# import only skills into every profile
ccpm import default --all --only skills

# preview what would happen without writing
ccpm import default --profile work --dry-run

# overwrite existing profile files
ccpm import default --profile work --force

# copy directly instead of symlinking (opts out of dedup)
ccpm import default --profile work --no-share

# keep symlink-to-dir entries as live symlinks into the share store
ccpm import default --profile work --live-symlinks

# skip every per-item prompt and import all discovered assets
ccpm import default --profile work --select-all

# decide whether imported MCP servers live in the global or per-profile fragment
ccpm import default --profile work --mcp-scope profile`}
            lang="bash"
          />
          <p className="text-sm text-fg-muted">
            Valid <code>--only</code> values: <code>skills</code>,{" "}
            <code>commands</code>, <code>rules</code>, <code>hooks</code>,{" "}
            <code>agents</code>, <code>settings</code>, <code>mcp</code>,{" "}
            <code>plugins</code>. Pass them comma-separated.
          </p>

          <p>
            <strong>Interactive wizard.</strong> In a TTY,{" "}
            <code>ccpm import default</code> opens a guided flow: pick the
            target profile (or all), choose which asset types to import, select
            individual items within each type, decide whether symlink-to-
            directory sources stay live or are snapshotted, and pick MCP scope
            (global vs. per-profile). Use <code>--select-all</code>,{" "}
            <code>--no-live-symlinks</code>, and <code>--mcp-scope</code> to
            skip prompts in scripts.
          </p>

          <H3 id="import-from-profile">ccpm import from-profile</H3>
          <p>
            Clone assets from one ccpm profile into another. Useful for
            bootstrapping a new profile from a known-good one, or for copying a
            subset of tools between personal and work setups. In a TTY both
            source and target are picker-driven; otherwise <code>--src</code>{" "}
            and <code>--profile</code> are required.
          </p>
          <CodeBlock
            code={`# clone everything from "work" into new profile "work-staging"
ccpm add work-staging
ccpm import from-profile --src work --profile work-staging

# clone only skills and commands
ccpm import from-profile --src work --profile work-staging --only skills,commands

# overwrite existing files in the target profile
ccpm import from-profile --src work --profile work-staging --force`}
            lang="bash"
          />
          <p className="text-sm text-fg-muted">
            Settings merge: existing keys in the target profile win; new keys
            from the source are added. MCP servers are not cloned via this
            command. Use <a href="#mcp-commands">MCP commands</a> directly to share MCP
            fragments.
          </p>

          <H3 id="import-sync">ccpm sync</H3>
          <p>
            Re-apply every global install (skills, MCP fragments, settings) to
            one or all profiles. Useful after editing{" "}
            <code>~/.ccpm/share/</code> directly, or to heal a profile whose
            symlinks or settings have drifted. Sync also runs automatically on{" "}
            <code>ccpm add</code> and <code>ccpm run</code>.
          </p>
          <CodeBlock
            code={`# sync every profile
ccpm sync --all

# sync just one
ccpm sync --profile work

# TTY: omit flags to pick profiles interactively
ccpm sync`}
            lang="bash"
          />
          <p className="text-sm text-fg-muted">
            In a TTY with no flags, <code>ccpm sync</code> opens a multi-select
            picker. In non-TTY contexts the default is to sync all profiles.
          </p>

          <H2 id="skills">Skills, MCP, and settings</H2>
          <p>
            These three asset types are the heart of ccpm&apos;s sharing model.
            Install something with <code>--global</code> and every profile picks
            it up; install with <code>--profile &lt;name&gt;</code> and only
            that profile sees it. Global installs automatically propagate to new
            profiles created afterward.
          </p>

          <div className="not-prose my-6 overflow-x-auto rounded-lg border border-border bg-surface shadow-[var(--shadow-card)]">
            <table className="w-full text-[0.875rem]">
              <thead>
                <tr className="border-b border-border bg-bg-subtle">
                  <th
                    scope="col"
                    className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle"
                  >
                    Asset
                  </th>
                  <th
                    scope="col"
                    className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle"
                  >
                    Shared store
                  </th>
                  <th
                    scope="col"
                    className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle"
                  >
                    In profile
                  </th>
                  <th
                    scope="col"
                    className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle"
                  >
                    Mechanism
                  </th>
                </tr>
              </thead>
              <tbody className="text-fg-muted">
                <tr className="border-b border-border">
                  <td className="py-2.5 px-4 text-fg">
                    Skills / agents / commands
                  </td>
                  <td className="py-2.5 px-4">
                    ~/.ccpm/share/&lt;kind&gt;/&lt;name&gt;
                  </td>
                  <td className="py-2.5 px-4">
                    &lt;profile&gt;/&lt;kind&gt;/&lt;name&gt;
                  </td>
                  <td className="py-2.5 px-4">Symlink</td>
                </tr>
                <tr className="border-b border-border">
                  <td className="py-2.5 px-4 text-fg">MCP servers</td>
                  <td className="py-2.5 px-4">
                    ~/.ccpm/share/mcp/&#123;global,&lt;profile&gt;&#125;.json
                  </td>
                  <td className="py-2.5 px-4">
                    &lt;profile&gt;/settings.json#mcpServers
                  </td>
                  <td className="py-2.5 px-4">Merge at launch</td>
                </tr>
                <tr>
                  <td className="py-2.5 px-4 text-fg">Settings</td>
                  <td className="py-2.5 px-4">
                    ~/.claude/settings.json (shared baseline) +
                    ~/.ccpm/share/settings/&lt;profile&gt;.json (per-profile)
                  </td>
                  <td className="py-2.5 px-4">&lt;profile&gt;/settings.json</td>
                  <td className="py-2.5 px-4">
                    Deep merge + owned-keys override + project layer
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <CodeBlock
            code={`# global skill (installed into every profile)
ccpm skill add ~/code-review --global

# per-profile MCP with an auth token
ccpm mcp add github --command "npx -y @modelcontextprotocol/server-github" \\
  --env GITHUB_TOKEN=ghp_... --profile work

# profile-specific setting
ccpm settings set model claude-opus-4 --profile work

# shared-across-profiles setting → edit the host file directly
#   (no ccpm command. This is native Claude's settings layer)
# ~/.claude/settings.json is the cross-profile baseline`}
            lang="bash"
          />

          <p className="text-sm text-fg-muted">
            <code>ccpm skill</code> / <code>ccpm mcp</code> accept{" "}
            <code>--global</code> or <code>--profile</code> (they prompt if you
            omit both in a TTY). <code>ccpm settings</code> only accepts{" "}
            <code>--profile</code>: shared defaults live in{" "}
            <code>~/.claude/settings.json</code> directly.
          </p>

          <H3 id="skills-commands">Skill commands</H3>
          <p>
            <code>ccpm skill</code> installs a directory that contains a{" "}
            <code>SKILL.md</code> into the shared store, then links it into one
            or all profiles. Live symlinks keep the profile copy pointing at the
            original source; the default is to snapshot the directory into{" "}
            <code>~/.ccpm/share/skills/</code>.
          </p>
          <CodeBlock
            code={`# install a local skill globally
ccpm skill add ~/code-review --global

# install only into "work"
ccpm skill add ~/code-review --profile work

# keep a symlink-to-dir source live (updates in-place)
ccpm skill add ~/code-review --global --live-symlink

# always snapshot (disable the live-symlink prompt)
ccpm skill add ~/code-review --global --copy

# list all installed skills (alias: skill ls)
ccpm skill list

# remove a skill from all profiles (alias: skill rm)
ccpm skill remove code-review --global

# remove from one profile only
ccpm skill rm code-review --profile work

# link a shared skill into a specific profile
ccpm skill link code-review --profile work`}
            lang="bash"
          />

          <H3 id="agents-commands-rules">Agents, commands, and rules</H3>
          <p>
            <code>ccpm agent</code>, <code>ccpm command</code>, and{" "}
            <code>ccpm rule</code> share the exact subcommand shape as{" "}
            <code>ccpm skill</code>: <code>add/remove/list/link</code> with{" "}
            <code>--global</code>, <code>--profile</code>,{" "}
            <code>--live-symlink</code>, and <code>--copy</code> flags. Each
            kind has its own shared store (
            <code>~/.ccpm/share/&#123;agents,commands,rules&#125;/</code>) and
            its own symlink subdirectory under every profile. Unlike skills
            (directories with a <code>SKILL.md</code> marker), the source for
            agents/commands/rules can be a single file (typically a{" "}
            <code>.md</code> file).
          </p>
          <CodeBlock
            code={`# install a custom agent for all profiles
ccpm agent add ~/my-agent.md --global

# install a slash command for one profile
ccpm command add ~/commands/ship.md --profile work

# install a rule into the shared store
ccpm rule add ~/rules/house-style.md --global

# list / remove / link work the same as skills
ccpm agent list
ccpm command rm ship --profile work
ccpm rule link house-style --profile staging`}
            lang="bash"
          />

          <H3 id="plugin-commands">Plugin commands</H3>
          <p>
            <code>ccpm plugin</code> installs plugins end-to-end without entering
            a Claude Code session. Marketplaces clone into a shared store, plugin
            files cache once and symlink into each profile, and per-profile
            activation lives in the same settings fragment ccpm uses for
            everything else. ccpm also reads installs created by Claude Code&apos;s{" "}
            <code>/plugin install</code>, so running both side by side is
            supported.
          </p>
          <p>
            First register a marketplace, then install a plugin from it. Use{" "}
            <code>--global</code> to install into every profile or{" "}
            <code>--profile &lt;name&gt;</code> for one. Pass{" "}
            <code>--install-only</code> to install without enabling, and{" "}
            <code>--ssh</code> to clone via <code>git@</code> instead of HTTPS.
          </p>
          <CodeBlock
            code={`# register a marketplace (HTTPS by default; --ssh for git@)
ccpm plugin marketplace add anthropics/claude-plugins-official

# list / remove registered marketplaces
ccpm plugin marketplace list
ccpm plugin marketplace remove claude-plugins-official

# install a plugin into every profile and enable it
ccpm plugin install vercel@claude-plugins-official --global

# install into one profile only
ccpm plugin install vercel@claude-plugins-official --profile work

# install without enabling
ccpm plugin install vercel@claude-plugins-official --profile work --install-only`}
            lang="bash"
          />
          <p>
            Once installed, toggle activation per profile, inspect state, remove
            a plugin, or reclaim disk space from removed plugins with the
            garbage collector (also run as part of <code>ccpm sync</code>).
          </p>
          <CodeBlock
            code={`# show installed plugins + enabled state across every profile
ccpm plugin list

# limit to one profile
ccpm plugin list --profile work

# enable / disable a plugin for one profile
ccpm plugin enable vercel@claude-plugins-official --profile work
ccpm plugin disable vercel@claude-plugins-official --profile personal

# remove a plugin from one profile or every profile
ccpm plugin remove vercel@claude-plugins-official --profile work
ccpm plugin remove vercel@claude-plugins-official --global

# garbage-collect unreferenced cache entries
ccpm plugin gc`}
            lang="bash"
          />
          <p className="text-sm text-fg-muted">
            Disabling a plugin in a profile overrides global activation, so a
            plugin enabled everywhere can be turned off in just one profile.
          </p>

          <H3 id="hooks-commands">Hook commands</H3>
          <p>
            <code>ccpm hooks</code> manages entries under the <code>hooks</code>{" "}
            key in a profile&apos;s settings fragment. Each entry has an
            optional matcher (tool-name pattern. Empty matches all) and a
            command. Hook scripts on disk (files in{" "}
            <code>~/.claude/hooks/</code>) are managed separately via{" "}
            <code>ccpm import default --only hooks</code>.
          </p>
          <CodeBlock
            code={`# run a shell command before every tool use
ccpm hooks add PreToolUse "echo firing" --profile work

# restrict to Edit / Write tools
ccpm hooks add PostToolUse "make lint" --matcher "Edit|Write" --profile work

# show the merged hook view (baseline + profile overrides)
ccpm hooks list --profile work

# remove the last entry (or use --index N for a specific position)
ccpm hooks remove PreToolUse --profile work`}
            lang="bash"
          />
          <p className="text-sm text-fg-muted">
            Known events: <code>PreToolUse</code>, <code>PostToolUse</code>,{" "}
            <code>UserPromptSubmit</code>, <code>SessionStart</code>,{" "}
            <code>SessionEnd</code>, <code>Notification</code>,{" "}
            <code>Stop</code>, <code>SubagentStop</code>,{" "}
            <code>PreCompact</code>.
          </p>

          <H3 id="mcp-commands">MCP commands</H3>
          <p>
            <code>ccpm mcp</code> supports three scopes and three transports.
            Scope controls *where* the server definition is written: the shared
            fragment (<code>--scope global</code>), a single profile (
            <code>--scope profile --profile &lt;name&gt;</code>), or the current
            project&apos;s <code>.mcp.json</code>(<code>--scope project</code>).
            Transport controls the wire format: <code>stdio</code> (default; use{" "}
            <code>--command</code>), <code>http</code>, or <code>sse</code> (use{" "}
            <code>--url</code> and optional <code>--header KEY=VALUE</code>).
          </p>
          <CodeBlock
            code={`# stdio MCP for one profile, with env vars
ccpm mcp add github \\
  --scope profile --profile work \\
  --command "npx" \\
  --args "-y,@modelcontextprotocol/server-github" \\
  --env GITHUB_TOKEN=ghp_...

# remote HTTP MCP with a bearer token header
ccpm mcp add supabase \\
  --scope profile --profile work \\
  --transport http \\
  --url https://mcp.supabase.com/mcp \\
  --header "Authorization=Bearer \$SUPABASE_TOKEN"

# globally-shared server (all profiles, now and future)
ccpm mcp add linear \\
  --scope global \\
  --command "npx -y @linear/mcp" \\
  --env LINEAR_API_KEY=lin_...

# project-scoped MCP. Writes to <repo>/.mcp.json
ccpm mcp add repo-tools --scope project --command node --args "./mcp/index.js"

# OAuth for a remote MCP. Spawns native claude scoped to the profile
ccpm mcp auth supabase --profile work

# list MCPs with their source (ccpm-global | ccpm-profile | host | project)
ccpm mcp list

# remove (alias: mcp rm)
ccpm mcp remove github --scope profile --profile work

# bulk import
ccpm mcp import ./mcp-servers.json --scope global`}
            lang="bash"
          />
          <p className="text-sm text-fg-muted">
            <code>--args</code> takes a comma-separated list; <code>--env</code>{" "}
            and <code>--header</code> take <code>KEY=VALUE</code> pairs and may
            be repeated. <code>--global</code> and{" "}
            <code>--profile &lt;name&gt;</code> are still accepted as aliases
            for <code>--scope global</code> and{" "}
            <code>--scope profile --profile &lt;name&gt;</code>.
          </p>

          <H3 id="env-commands">Env var commands</H3>
          <p>
            Persist environment variables on a profile; they&apos;re layered
            into the process env at every <code>ccpm run</code>, sitting below
            the parent process env and above <code>ccpm run --ccpm-env</code>{" "}
            overrides. <code>CLAUDE_CONFIG_DIR</code> and{" "}
            <code>ANTHROPIC_API_KEY</code> are reserved. ccpm always computes
            them.
          </p>
          <CodeBlock
            code={`# persist env vars on a profile
ccpm env set ANTHROPIC_BASE_URL=https://proxy.example CLAUDE_CODE_MAX_OUTPUT_TOKENS=32768 --profile work

# remove
ccpm env unset ANTHROPIC_BASE_URL --profile work

# list
ccpm env list --profile work`}
            lang="bash"
          />

          <H3 id="permissions-commands">Permission commands</H3>
          <p>
            <code>ccpm permissions</code> manages{" "}
            <code>permissions.&#123;allow,ask,deny,defaultMode&#125;</code>{" "}
            directly. No JSON surgery. Adding a rule to one bucket removes it
            from the other two so the lists stay disjoint. Use{" "}
            <code>--global</code> to write to{" "}
            <code>~/.claude/settings.json</code> (the cross-profile baseline)
            instead of a profile fragment.
          </p>
          <CodeBlock
            code={`# allow, ask, or deny a tool-pattern rule (syntax matches native claude)
ccpm permissions allow "Bash(git status:*)" --profile work
ccpm permissions ask   "Edit(**/*.md)" --profile work
ccpm permissions deny  "Bash(rm:*)" --profile work

# strip a rule from all three buckets
ccpm permissions remove "Bash(git status:*)" --profile work

# set the default mode (native enum)
ccpm permissions mode plan --profile work
# valid values: default, acceptEdits, plan, auto, dontAsk, bypassPermissions

# list all rules and the current default mode
ccpm permissions list --profile work`}
            lang="bash"
          />

          <H3 id="trust-commands">Trusted project directories</H3>
          <p>
            <code>ccpm trust</code> manages which project directories are
            allowed to contribute hooks, permissions, and MCP servers via{" "}
            <code>./.claude/settings.json</code> and{" "}
            <code>./.claude/settings.local.json</code>. A new repo&apos;s
            project settings are ignored until you grant trust. Aliases:{" "}
            <code>add</code> / <code>grant</code>, <code>remove</code> /{" "}
            <code>rm</code> / <code>forget</code> / <code>revoke</code>,{" "}
            <code>list</code> / <code>ls</code>.
          </p>
          <CodeBlock
            code={`# trust the current project (or pass a path)
ccpm trust add
ccpm trust add ~/code/internal-tooling

# list trusted dirs
ccpm trust list

# revoke
ccpm trust remove ~/code/internal-tooling`}
            lang="bash"
          />

          <H3 id="sessions-commands">Sessions</H3>
          <p>
            <code>ccpm sessions list &lt;profile&gt;</code> reads the JSONL
            session files Claude Code stores inside a profile at{" "}
            <code>&lt;profileDir&gt;/projects/&lt;encoded-cwd&gt;/*.jsonl</code>
            . By default it scopes to the current working directory (matching{" "}
            <code>claude --resume</code>); pass <code>--all</code> to list every
            project.
          </p>
          <CodeBlock
            code={`# sessions for the current project in profile "work"
ccpm sessions list work

# every session the profile has ever recorded
ccpm sessions list work --all`}
            lang="bash"
          />

          <H3 id="settings-commands">Settings commands</H3>
          <p>
            Per-profile settings fragments live at{" "}
            <code>~/.ccpm/share/settings/&lt;profile&gt;.json</code> and are
            deep-merged into the profile&apos;s <code>settings.json</code>. Keys
            you set through ccpm are tracked in a <code>.owned.json</code>{" "}
            sidecar so Claude Code cannot silently overwrite them (see{" "}
            <a href="#settings-precedence">Settings precedence</a>).
          </p>
          <p>
            <strong>ccpm does not manage shared settings.</strong> The
            cross-profile baseline is <code>~/.claude/settings.json</code>. The
            same file native Claude Code reads, and ccpm merges it into every
            profile at launch. Edit it with a text editor, or run{" "}
            <code>claude /config</code> natively, to change defaults for every
            profile. There is no <code>--global</code> flag on{" "}
            <code>ccpm settings</code>.
          </p>
          <CodeBlock
            code={`# set a per-profile scalar
ccpm settings set model claude-opus-4 --profile work

# dot-notation nested key
ccpm settings set permissions.allow.Bash true --profile work

# JSON values (objects, arrays) are parsed automatically
ccpm settings set env.FOO '{"a":1,"b":2}' --profile work

# read the effective value for a profile
ccpm settings get model --profile work

# dump the fully merged settings for a profile
ccpm settings show --profile work

# apply a JSON fragment file (deep-merged into the profile)
ccpm settings apply ./team-defaults.json --profile work

# set the native statusLine block (empty string removes it)
ccpm settings statusline "~/.claude/statusline.sh" --profile work

# set the outputStyle (Build | Explanatory | Learning | Direct | default)
ccpm settings outputstyle Explanatory --profile work`}
            lang="bash"
          />
          <p className="text-sm text-fg-muted">
            All subcommands (<code>set</code>, <code>get</code>,{" "}
            <code>apply</code>, <code>show</code>, <code>statusline</code>,{" "}
            <code>outputstyle</code>) require <code>--profile</code>. The
            statusline wrapper writes the native{" "}
            <code>&#123;type: &quot;command&quot;, command: ...&#125;</code>{" "}
            shape so it stays loadable by native claude.
          </p>

          <H2 id="mcp-auth">MCP auth model</H2>
          <p>
            How an MCP server authenticates determines whether ccpm can isolate
            it per profile. There are three categories:
          </p>

          <Callout type="info" title="1. Env-var based (fully isolated)">
            Servers that take credentials via environment variables like{" "}
            <code>GITHUB_TOKEN</code> or <code>LINEAR_API_KEY</code>. ccpm
            stores the value inside the per-profile MCP fragment at{" "}
            <code>~/.ccpm/share/mcp/&lt;profile&gt;.json</code>, so every
            profile can carry a different account. Use{" "}
            <code>--env KEY=VALUE</code> with <code>ccpm mcp add</code>.
          </Callout>

          <Callout type="info" title="2. MCP OAuth (fully isolated)">
            Servers that open a browser and cache the token inside{" "}
            <code>.claude.json</code> under <code>mcpOAuth</code>. Because{" "}
            <code>CLAUDE_CONFIG_DIR</code> is per-profile, each profile gets its
            own OAuth session automatically. To trigger an OAuth dance from ccpm
            without launching a full claude session, run{" "}
            <code>ccpm mcp auth &lt;server&gt; --profile &lt;name&gt;</code>.
            It spawns native claude with <code>CLAUDE_CONFIG_DIR</code> pinned
            to the profile so tokens land in the right scope.
          </Callout>

          <Callout type="warn" title="3. Global-cache MCPs (shared)">
            Servers that write to a fixed-name location like{" "}
            <code>~/.config/&lt;service&gt;/</code> or a non-namespaced OS
            keychain entry. These are{" "}
            <strong>shared across all profiles</strong> and ccpm cannot isolate
            them without cooperation from the MCP server. Treat them as
            &quot;one account for all profiles&quot; and plan accordingly.
          </Callout>

          <H2 id="settings-precedence">Settings precedence</H2>
          <p>
            At launch, ccpm materializes <code>settings.json</code> for a
            profile by merging in this order (lowest → highest, higher wins):
          </p>
          <ol>
            <li>
              The profile&apos;s existing{" "}
              <code>&lt;profile&gt;/settings.json</code>. Preserves keys Claude
              Code auto-wrote that nothing else redefines.
            </li>
            <li>
              <code>~/.claude/settings.json</code>. The host file native Claude
              Code uses. Edit it to change defaults for every profile.
            </li>
            <li>
              <code>~/.ccpm/share/settings/&lt;profile&gt;.json</code>. The
              per-profile ccpm fragment. Beats the shared baseline for this
              profile.
            </li>
            <li>
              <strong>Owned-keys override.</strong> Any leaf key you set via{" "}
              <code>ccpm settings set --profile</code> or{" "}
              <code>ccpm settings apply --profile</code> is recorded in a{" "}
              <code>.owned.json</code> sidecar and re-applied from the profile
              fragment. This guarantees values you explicitly set through ccpm
              are never silently overwritten by Claude Code rewriting its own
              config.
            </li>
            <li>
              <code>./.claude/settings.json</code> at the project root
              (discovered by walking up from CWD). Per-repo overrides beat
              profile defaults.
            </li>
            <li>
              <code>./.claude/settings.local.json</code> at the project root.
              Gitignored per-machine overrides for the same project.
            </li>
            <li>
              <strong>Enterprise managed-settings.</strong>{" "}
              <code>
                /Library/Application Support/ClaudeCode/managed-settings.json
              </code>{" "}
              on macOS, <code>/etc/claude-code/managed-settings.json</code> on
              Linux, and{" "}
              <code>C:\ProgramData\ClaudeCode\managed-settings.json</code> on
              Windows, plus sibling <code>managed-settings.d/*.json</code>{" "}
              drop-ins merged in alphabetical order. Highest precedence so
              org-level policy always wins, matching native Claude Code.
            </li>
          </ol>
          <p>
            Objects merge key-by-key; arrays and scalars from a
            higher-precedence source replace the lower one.
          </p>

          <H2 id="doctor">Doctor</H2>
          <p>
            <code>ccpm doctor</code> is your one-stop health check. It never
            fails builds (warnings are informational), but it will tell you
            when something is actually broken so you don&apos;t chase ghosts.
          </p>
          <p>It reports on, in order:</p>
          <ul>
            <li>
              <strong>Environment</strong>. ccpm version, platform, Claude Code
              binary path, and <code>claude --version</code> (with a warning on
              macOS if you&apos;re below v2.1.56, which is required for
              per-profile OAuth keychain isolation).
            </li>
            <li>
              <strong>ccpm base directory</strong>. Confirms{" "}
              <code>~/.ccpm/</code> exists and is readable.
            </li>
            <li>
              <strong>Per-profile auth health</strong>. OAuth token validity
              and expiry for each profile. On macOS OAuth profiles, the
              namespaced keychain service name is printed so you can inspect the
              entry manually with Keychain Access.
            </li>
            <li>
              <strong>Root vs. profile diff</strong>. Anything in{" "}
              <code>~/.claude</code> that no profile has adopted yet, and
              vice-versa. Prints a one-line hint pointing at the right{" "}
              <code>ccpm import</code> command.
            </li>
            <li>
              <strong>Symlink integrity</strong>. Flags broken symlinks and
              copies under a profile that have drifted from the shared store.
            </li>
            <li>
              <strong>Shared asset manifest</strong>. How many skills, MCP
              servers, and settings keys are tracked in{" "}
              <code>~/.ccpm/installs.json</code>.
            </li>
            <li>
              <strong>Drift fingerprint</strong>. Detects when{" "}
              <code>~/.claude</code> has changed since the last{" "}
              <code>ccpm import default</code> snapshot.
            </li>
            <li>
              <strong>Drift notifications</strong>. Whether the{" "}
              <code>check_default_drift</code> config flag is on (see{" "}
              <a href="#drift">Drift detection</a>).
            </li>
            <li>
              <strong>Platform notes</strong>. Platform-specific caveats such
              as the Windows symlink fallback marker and global-cache MCP
              isolation limits.
            </li>
          </ul>
          <p className="text-sm text-fg-muted">
            Exit code is 0 on success or when only warnings are present, and 1
            when real issues are detected.
          </p>
          <p>
            <code>ccpm doctor</code> is read-only by default. Pass{" "}
            <code>--fix</code> to prune dangling shared-asset symlinks found
            during the check. For deeper repair across host, profile, and project
            scopes (duplicates, ghost manifest entries, budget overflow), run{" "}
            <code>ccpm consolidate --fix</code>.
          </p>
          <CodeBlock
            code={`$ ccpm doctor
Environment
  ccpm       ${VERSION}
  platform   darwin/arm64
  claude     2.1.61 (/usr/local/bin/claude)

Profiles
  personal   oauth   ✓ valid   keychain: Claude Code-credentials-7b3a4f19
  work       apikey  ✓ valid

Root vs profiles
  ~/.claude has "python-review" skill; no profile adopted it
    ↳ ccpm import default --only skills --all

No symlink issues. No drift detected.`}
            lang="bash"
          />

          <H2 id="drift">Drift detection</H2>
          <p>
            Every <code>ccpm import default</code> snapshots the files under{" "}
            <code>~/.claude</code> (skills, commands, rules, hooks, agents,
            settings, MCP fragments) into a fingerprint. Later, ccpm can tell
            you whether your default Claude config has drifted away from what
            your profiles were built from, so a skill you tweaked in{" "}
            <code>~/.claude</code> does not get stale in your profiles.
          </p>

          <H3 id="drift-fingerprint">ccpm default fingerprint</H3>
          <CodeBlock
            code={`# record the current ~/.claude state as the drift baseline
ccpm default fingerprint update

# compare ~/.claude against the last fingerprint
ccpm default fingerprint check`}
            lang="bash"
          />
          <p>
            <code>check</code> prints added, modified, and removed paths and
            suggests the right{" "}
            <code>ccpm import default --profile &lt;name&gt;</code> to sync
            changes into a profile. Run <code>update</code> to accept the
            current state without importing.
          </p>

          <H3 id="drift-config">ccpm config</H3>
          <p>
            Drift nudges on <code>ccpm run</code> and <code>ccpm use</code> are
            controlled by a single config key.
          </p>
          <CodeBlock
            code={`# turn drift warnings on (default is off)
ccpm config set check_default_drift true

# turn them off
ccpm config set check_default_drift false

# read the current value
ccpm config get check_default_drift`}
            lang="bash"
          />

          <H2 id="vault">Vault backup</H2>
          <p>
            ccpm can create encrypted backups of your credentials for disaster
            recovery and machine migration. Uses AES-256-GCM encryption with a
            master key stored in your OS keychain.
          </p>
          <CodeBlock
            code={`# backup credentials
ccpm auth backup personal

# restore after machine migration
ccpm auth restore personal`}
            lang="bash"
          />

          <H2 id="backup-migrate">Backup &amp; migrate profiles</H2>
          <p>
            <code>ccpm export</code> bundles a profile&apos;s directory — skills,
            agents, commands, rules, hooks, MCP fragments, plugin metadata, and
            settings — into a single <code>.tar.gz</code> you can copy to another
            machine and restore with <code>ccpm import-bundle</code>. Use{" "}
            <a href="#profiles-clone">ccpm clone</a> instead when you just want a
            second copy on the same machine.
          </p>
          <Callout type="warn" title="Credentials are excluded by default">
            OS-keychain tokens are machine-bound, and{" "}
            <code>.credentials.json</code> / <code>.claude.json</code> hold
            secrets you usually don&apos;t want in a shareable file, so{" "}
            <code>ccpm export</code> leaves them out. Pass{" "}
            <code>--include-credentials</code> only for a trusted same-user move
            (e.g. a Linux machine migration), and treat the resulting bundle as
            sensitive. When a restored bundle has no credentials, authenticate
            the profile afterward with <code>ccpm auth refresh &lt;name&gt;</code>
            .
          </Callout>
          <CodeBlock
            code={`# export a profile (credentials excluded by default)
ccpm export work
ccpm export work -o ~/backups/work.ccpm.tar.gz

# include credentials for a trusted same-user move
ccpm export work --include-credentials

# restore on another machine (path-traversal-safe)
ccpm import-bundle ~/backups/work.ccpm.tar.gz

# restore under a different name
ccpm import-bundle ~/backups/work.ccpm.tar.gz --profile work-restored
ccpm auth refresh work-restored`}
            lang="bash"
          />

          <H2 id="uninstall">Uninstall</H2>
          <p>
            <code>ccpm uninstall</code> removes every profile, deletes API keys
            from the OS keychain, wipes vault backups, and deletes{" "}
            <code>~/.ccpm/</code>. It does <strong>not</strong> remove the{" "}
            <code>ccpm</code> binary itself or the shell hook you added to{" "}
            <code>~/.zshrc</code> / <code>~/.bashrc</code>. The command prints
            those cleanup steps so you can run them by hand. If you installed the{" "}
            <a href="#desktop">desktop app</a>, also delete{" "}
            <code>/Applications/CCPM.app</code>.
          </p>
          <CodeBlock
            code={`# with confirmation prompt
ccpm uninstall

# skip the confirmation
ccpm uninstall --force`}
            lang="bash"
          />

          <H2 id="shell">Shell integration</H2>
          <p>
            The shell hook wraps <code>ccpm use</code> so it can set environment
            variables in your current shell. Without it, <code>ccpm use</code>{" "}
            cannot modify your shell environment.
          </p>

          <H3 id="shell-setup">Setup</H3>
          <CodeBlock
            code={`# add to ~/.zshrc or ~/.bashrc (shell auto-detected)
eval "$(ccpm shell-init)"

# force a specific shell (bash | zsh | fish | powershell)
eval "$(ccpm shell-init --shell zsh)"

# reload
source ~/.zshrc`}
            lang="bash"
          />

          <H3 id="shell-usage">Usage</H3>
          <CodeBlock
            code={`# set profile for this terminal session
ccpm use personal

# now any 'claude' command uses the personal profile
claude`}
            lang="bash"
          />

          <p className="text-sm">
            Supported shells: zsh, bash, fish, PowerShell.
          </p>

          <H2 id="completion">Shell completion</H2>
          <p>
            <code>ccpm completion &lt;shell&gt;</code> prints a completion script
            for <code>bash</code>, <code>zsh</code>, <code>fish</code>, or{" "}
            <code>powershell</code>. Completion includes profile names on the
            commands that take one (<code>run</code>, <code>use</code>,{" "}
            <code>remove</code>, <code>rename</code>, <code>set-default</code>,{" "}
            <code>clone</code>, <code>export</code>). This is separate from the{" "}
            <a href="#shell">shell hook</a> — completion only adds tab
            suggestions and never changes your environment.
          </p>
          <CodeBlock
            code={`# zsh: load on every new shell
echo 'source <(ccpm completion zsh)' >> ~/.zshrc

# bash
echo 'source <(ccpm completion bash)' >> ~/.bashrc

# fish
ccpm completion fish > ~/.config/fish/completions/ccpm.fish

# per-shell install details
ccpm completion zsh --help`}
            lang="bash"
          />

          <H2 id="prompt">Shell prompt</H2>
          <p>
            <code>ccpm prompt</code> prints the profile the current
            shell/session is using, so you can show it in your prompt (PS1,
            starship, powerlevel10k) and always know which Claude Code account a
            terminal is bound to. Resolution order:{" "}
            <code>$CCPM_ACTIVE_PROFILE</code> (set by <code>ccpm use</code>),
            then <code>$CLAUDE_CONFIG_DIR</code> matched back to a known profile
            dir, then the configured default — but only with{" "}
            <code>--show-default</code>. It prints nothing (exit 0) when no
            profile is active, so it stays quiet in non-ccpm shells. Use{" "}
            <code>--format</code> to wrap the name (it must contain a single{" "}
            <code>%s</code>).
          </p>
          <CodeBlock
            code={`# bash/zsh PS1
PS1='$(ccpm prompt --format "[ccpm:%s] ")'"$PS1"

# starship custom command (~/.config/starship.toml)
[custom.ccpm]
command = "ccpm prompt"
when = true

# fall back to the configured default when nothing is active
ccpm prompt --show-default`}
            lang="bash"
          />

          <H2 id="ide">IDE default profile</H2>
          <p>
            IDE extensions (VS Code, Cursor, Antigravity) launch the{" "}
            <code>claude</code> binary directly without going through a shell,
            so they ignore the <code>ccpm use</code> shell hook. Use{" "}
            <code>ccpm set-default</code> to pin a profile as the default for
            every direct <code>claude</code> launch on this machine. Call it
            without an argument in a TTY for a profile picker.
          </p>
          <p>
            On macOS, <code>set-default</code> does three things: copies the
            profile&apos;s namespaced keychain entry into the default slot
            (folding the previous default back into its original profile first
            so refresh-token rotation does not strand it), syncs identity
            fields into <code>~/.claude.json</code>, and writes a user-level
            LaunchAgent that runs{" "}
            <code>launchctl setenv CLAUDE_CONFIG_DIR</code> at every login.
            Already-running IDE windows need one restart to pick up the new
            env; future launches and future logins are automatic. On Linux and
            Windows the keychain and identity sync still apply; the launchctl
            mechanism is macOS-only for now.
          </p>
          <CodeBlock
            code={`# set the default profile (pick interactively if no name passed)
ccpm set-default work
ccpm set-default

# clear the default
ccpm unset-default`}
            lang="bash"
          />

          <H2 id="privacy">Privacy &amp; security</H2>

          <Callout type="tip" title="100% local">
            ccpm is fully local.{" "}
            <strong>Your data never leaves your machine.</strong> No telemetry,
            analytics, or tracking of any kind.
          </Callout>

          <H3 id="privacy-credentials">Credential storage</H3>
          <p>
            API keys are stored in your <strong>OS keychain</strong> (macOS
            Keychain, Linux Secret Service, Windows Credential Manager). Never
            in plaintext files. OAuth tokens are managed by Claude Code itself
            within the isolated profile directory.
          </p>

          <H3 id="privacy-vault">Encrypted vault</H3>
          <p>
            Vault backups use <strong>AES-256-GCM encryption</strong> with a
            master key stored in your OS keychain. The encrypted files live
            locally in <code>~/.ccpm/vault/</code>.
          </p>

          <H3 id="privacy-local">Local config only</H3>
          <p>
            All configuration, profiles, and data live in <code>~/.ccpm/</code>{" "}
            on your filesystem. No cloud storage, no sync services, no external
            dependencies.
          </p>

          <H3 id="privacy-source">Open source</H3>
          <p>
            ccpm is fully open source under the MIT license.{" "}
            <a
              href="https://github.com/nitin-1926/claude-code-profile-manager"
              target="_blank"
              rel="noopener noreferrer"
            >
              Audit the code yourself
            </a>
            .
          </p>

          <H2 id="platforms">Platform support</H2>
          <Callout
            type="warn"
            title="macOS is the only verified platform today"
          >
            Linux and Windows builds compile, install, and run, but the
            OAuth-isolation paths (<code>set-default</code>,{" "}
            <code>auth backup/restore</code>, keychain-based <code>status</code>
            ) are <strong>experimental</strong>. They have not been exercised
            against a real Linux Secret Service or Windows Credential Manager
            install. <strong>macOS now; Linux + Windows coming soon.</strong>
          </Callout>
          <div className="not-prose my-6 overflow-x-auto rounded-lg border border-border bg-surface shadow-[var(--shadow-card)]">
            <table className="w-full text-[0.875rem]">
              <thead>
                <tr className="border-b border-border bg-bg-subtle">
                  <th
                    scope="col"
                    className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle"
                  >
                    Feature
                  </th>
                  <th
                    scope="col"
                    className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle"
                  >
                    macOS ✓
                  </th>
                  <th
                    scope="col"
                    className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle"
                  >
                    Windows ⚠
                  </th>
                  <th
                    scope="col"
                    className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle"
                  >
                    Linux ⚠
                  </th>
                </tr>
              </thead>
              <tbody className="text-fg-muted">
                <tr className="border-b border-border">
                  <td className="py-2.5 px-4 text-fg">OAuth per-profile</td>
                  <td className="py-2.5 px-4">
                    Keychain entry namespaced by profile dir
                  </td>
                  <td className="py-2.5 px-4">
                    wincred entry, same namespacing (theoretical)
                  </td>
                  <td className="py-2.5 px-4">.credentials.json (legacy)</td>
                </tr>
                <tr className="border-b border-border">
                  <td className="py-2.5 px-4 text-fg">API key storage</td>
                  <td className="py-2.5 px-4">Keychain</td>
                  <td className="py-2.5 px-4">Credential Manager</td>
                  <td className="py-2.5 px-4">Secret Service</td>
                </tr>
                <tr className="border-b border-border">
                  <td className="py-2.5 px-4 text-fg">Parallel sessions</td>
                  <td className="py-2.5 px-4">Yes</td>
                  <td className="py-2.5 px-4">Yes</td>
                  <td className="py-2.5 px-4">Yes</td>
                </tr>
                <tr className="border-b border-border">
                  <td className="py-2.5 px-4 text-fg">Shared skill dedup</td>
                  <td className="py-2.5 px-4">Symlinks</td>
                  <td className="py-2.5 px-4">
                    Symlinks (Developer Mode) or copy fallback
                  </td>
                  <td className="py-2.5 px-4">Symlinks</td>
                </tr>
                <tr className="border-b border-border">
                  <td className="py-2.5 px-4 text-fg">Shell hook</td>
                  <td className="py-2.5 px-4">zsh, bash, fish</td>
                  <td className="py-2.5 px-4">PowerShell</td>
                  <td className="py-2.5 px-4">zsh, bash, fish</td>
                </tr>
                <tr>
                  <td className="py-2.5 px-4 text-fg">Desktop app</td>
                  <td className="py-2.5 px-4">Native GUI (.dmg)</td>
                  <td className="py-2.5 px-4">—</td>
                  <td className="py-2.5 px-4">—</td>
                </tr>
              </tbody>
            </table>
          </div>

          <Callout type="warn" title="Claude Code v2.1.56+ required on macOS">
            Per-profile OAuth isolation on macOS depends on Claude Code&apos;s
            namespaced keychain service (introduced in v2.1.56). Older builds
            share a single <code>Claude Code-credentials</code> entry across all
            profiles, so multiple OAuth profiles cannot stay authenticated
            simultaneously. <code>ccpm doctor</code> warns when your installed
            Claude Code is too old.
          </Callout>

          <H2 id="limitations">Known limitations</H2>

          <Callout
            type="warn"
            title="VS Code extension ignores CLAUDE_CONFIG_DIR"
          >
            The VS Code Claude extension always reads from{" "}
            <code>~/.claude</code>. Use{" "}
            <code>ccpm set-default &lt;profile&gt;</code> to point it at a
            specific ccpm profile. On macOS (verified) and Windows
            (experimental) this copies the profile&apos;s namespaced
            credential-store entry into the default slot under the OS-user
            account; on Linux it falls back to copying{" "}
            <code>.credentials.json</code> until a libsecret-backed handler
            ships.
          </Callout>

          <Callout type="warn" title="Windows symlink fallback">
            Without Developer Mode or admin rights, Windows cannot create
            symlinks. ccpm falls back to copying assets from the shared store
            into the profile and writes a marker at{" "}
            <code>~/.ccpm/.windows-copy-fallback</code>. Turn on Developer Mode
            for true deduplication.
          </Callout>

          <Callout
            type="warn"
            title="Globally-cached MCP servers cannot be isolated"
          >
            MCP servers that cache credentials in a fixed-name location (e.g.{" "}
            <code>~/.config/&lt;service&gt;/</code> or a non-namespaced OS
            keychain entry) are shared across every profile. See{" "}
            <a href="#mcp-auth">MCP auth model</a> for details.
          </Callout>

          <Callout type="info" title="CLAUDE_CONFIG_DIR path with ~/">
            Claude has a bug resolving <code>~/</code> paths on Linux. ccpm
            always uses absolute paths, so this is handled automatically.
          </Callout>

          <Callout type="info" title="Headless Linux keychain">
            <code>go-keyring</code> requires D-Bus and a secret service
            (gnome-keyring or kwallet). On headless servers, API key profiles
            need a running secret service.
          </Callout>
        </main>

        <DocsToc />
      </div>
      <Footer />
    </>
  );
}
