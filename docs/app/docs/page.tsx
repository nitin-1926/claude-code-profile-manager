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
import { VERSION } from "@/lib/version";

export const metadata: Metadata = {
  title: "Documentation — ccpm",
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
            Pick a package manager. ccpm ships as a single static binary, so
            any of these paths gets you to the same place.
          </p>

          <div className="not-prose my-5">
            <InstallTabs />
          </div>

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
          <p className="text-sm text-fg-muted">
            Every command accepts a global <code>--verbose</code> /{" "}
            <code>-v</code> flag that prints extra diagnostic output — useful
            when filing an issue.
          </p>

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
            as <code>ccpm ls</code>.
          </p>

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
            <code>claude</code> directly — no <code>--</code> separator needed
            for the common cases. Three flags are intercepted by ccpm:{" "}
            <code>--ccpm-env KEY=VALUE</code> (repeatable, one-shot env
            override), <code>--help</code>, and <code>--version</code>. Use{" "}
            <code>--</code> to forward the latter two to claude.
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
            Called without a name in an interactive terminal, <code>ccpm use</code>{" "}
            opens a profile picker. In non-TTY contexts (scripts, CI) the name
            argument is required.
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
            machine migration. See <a href="#vault">Vault backup</a> for the full
            story.
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
            Valid <code>--only</code> values:{" "}
            <code>skills</code>, <code>commands</code>, <code>rules</code>,{" "}
            <code>hooks</code>, <code>agents</code>, <code>settings</code>,{" "}
            <code>mcp</code>, <code>plugins</code>. Pass them comma-separated.
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
            command — use <a href="#mcp">MCP commands</a> directly to share MCP
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
                  <th scope="col" className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle">
                    Asset
                  </th>
                  <th scope="col" className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle">
                    Shared store
                  </th>
                  <th scope="col" className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle">
                    In profile
                  </th>
                  <th scope="col" className="text-left py-2.5 px-4 font-mono text-[0.68rem] font-semibold uppercase tracking-[0.1em] text-fg-subtle">
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
#   (no ccpm command — this is native Claude's settings layer)
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
            or all profiles. Live symlinks keep the profile copy pointing at
            the original source; the default is to snapshot the directory into{" "}
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
            kind has its own shared store
            (<code>~/.ccpm/share/&#123;agents,commands,rules&#125;/</code>)
            and its own symlink subdirectory under every profile. Unlike
            skills (directories with a <code>SKILL.md</code> marker), the
            source for agents/commands/rules can be a single file (typically a{" "}
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
            Plugin files are installed by Claude Code itself — run{" "}
            <code>/plugin install &lt;name&gt;</code> inside a ccpm session to
            add one. ccpm manages the <code>enabledPlugins</code> settings key
            per profile so you can override which plugins are active in each
            profile. There is no <code>ccpm plugin install</code> (the stub
            exists only to point users back to the in-session command).
          </p>
          <CodeBlock
            code={`# show installed plugins + enabled state across every profile
ccpm plugin list

# limit to one profile
ccpm plugin list --profile work

# enable a plugin for one profile
ccpm plugin enable vercel@claude-plugins-official --profile work

# disable a globally-enabled plugin in one profile
ccpm plugin disable vercel@claude-plugins-official --profile personal`}
            lang="bash"
          />
          <p className="text-sm text-fg-muted">
            Global activation (every profile) is whatever Claude Code wrote
            into <code>~/.claude/settings.json</code> under{" "}
            <code>enabledPlugins</code> — ccpm reads that as the baseline and
            layers profile fragments on top.
          </p>

          <H3 id="hooks-commands">Hook commands</H3>
          <p>
            <code>ccpm hooks</code> manages entries under the <code>hooks</code>{" "}
            key in a profile&apos;s settings fragment. Each entry has an
            optional matcher (tool-name pattern — empty matches all) and a
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
            fragment (<code>--scope global</code>), a single profile
            (<code>--scope profile --profile &lt;name&gt;</code>), or the
            current project&apos;s <code>.mcp.json</code>
            (<code>--scope project</code>). Transport controls the wire format:{" "}
            <code>stdio</code> (default; use <code>--command</code>),{" "}
            <code>http</code>, or <code>sse</code> (use <code>--url</code> and
            optional <code>--header KEY=VALUE</code>).
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

# project-scoped MCP — writes to <repo>/.mcp.json
ccpm mcp add repo-tools --scope project --command node --args "./mcp/index.js"

# OAuth for a remote MCP — spawns native claude scoped to the profile
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
            <code>--args</code> takes a comma-separated list;{" "}
            <code>--env</code> and <code>--header</code> take{" "}
            <code>KEY=VALUE</code> pairs and may be repeated.{" "}
            <code>--global</code> and <code>--profile &lt;name&gt;</code> are
            still accepted as aliases for <code>--scope global</code> and{" "}
            <code>--scope profile --profile &lt;name&gt;</code>.
          </p>

          <H3 id="env-commands">Env var commands</H3>
          <p>
            Persist environment variables on a profile; they&apos;re layered
