import type { Metadata } from "next";
import { Hash } from "lucide-react";
import { Nav } from "../components/nav";
import { Footer } from "../components/footer";
import { CodeBlock } from "../components/code-block";
import { Callout } from "../components/callout";
import { DocsSidebar } from "../components/docs-sidebar";
import { DocsToc } from "../components/docs-toc";

export const metadata: Metadata = {
  title: "Documentation — ccpm",
  description: "Complete documentation for ccpm (Claude Code Profile Manager)",
};

function H2({ id, children }: { id: string; children: React.ReactNode }) {
  return (
    <h2 id={id}>
      {children}
      <a
        href={`#${id}`}
        aria-label={`Link to ${typeof children === "string" ? children : id}`}
        className="heading-anchor inline-flex"
      >
        <Hash size={18} strokeWidth={1.75} className="inline" />
      </a>
    </h2>
  );
}

function H3({ id, children }: { id: string; children: React.ReactNode }) {
  return (
    <h3 id={id}>
      {children}
      <a
        href={`#${id}`}
        aria-label={`Link to ${typeof children === "string" ? children : id}`}
        className="heading-anchor inline-flex"
      >
        <Hash size={15} strokeWidth={1.75} className="inline" />
      </a>
    </h3>
  );
}

export default async function DocsPage() {
  return (
    <>
      <Nav />
      <div className="max-w-7xl mx-auto px-6 py-12 flex gap-10">
        <DocsSidebar />

        <main className="flex-1 min-w-0 max-w-3xl prose-doc">
          <div className="mb-12 not-prose">
            <p className="font-mono text-[0.7rem] font-semibold uppercase tracking-[0.12em] text-accent mb-2">
              {"// documentation"}
            </p>
            <h1
              className="font-semibold tracking-tight text-fg leading-[1.1]"
              style={{ fontSize: "var(--t-h1)" }}
            >
              Everything you need to manage multiple Claude Code accounts.
            </h1>
            <p className="mt-4 text-fg-muted leading-relaxed text-[1.0625rem]">
              ccpm is a single static binary that creates fully isolated Claude
              Code profiles, each with their own credentials, settings, and
              memory.
            </p>
          </div>

          <H2 id="installation">Installation</H2>

          <H3 id="install-npm">npm</H3>
          <CodeBlock code="npm install -g @ngcodes/ccpm" lang="bash" />

          <H3 id="install-curl">curl (macOS / Linux)</H3>
          <CodeBlock
            code="curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh"
            lang="bash"
          />

          <H3 id="install-go">Go</H3>
          <CodeBlock
            code="go install github.com/nitin-1926/ccpm@latest"
            lang="bash"
          />

          <H3 id="install-source">From source</H3>
          <CodeBlock
            code={`git clone https://github.com/nitin-1926/claude-code-profile-manager.git
cd claude-code-profile-manager
make build
# binary at ./bin/ccpm`}
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
          <CodeBlock
            code={`# run with profile
ccpm run personal

# pass arguments to claude
ccpm run work -- --model sonnet`}
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

          <div className="not-prose my-6 overflow-x-auto rounded-xl border border-border bg-surface">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left py-3 px-4 font-mono text-[0.7rem] font-semibold uppercase tracking-wider text-fg-subtle">
                    Asset
                  </th>
                  <th className="text-left py-3 px-4 font-mono text-[0.7rem] font-semibold uppercase tracking-wider text-fg-subtle">
                    Shared store
                  </th>
                  <th className="text-left py-3 px-4 font-mono text-[0.7rem] font-semibold uppercase tracking-wider text-fg-subtle">
                    In profile
                  </th>
                  <th className="text-left py-3 px-4 font-mono text-[0.7rem] font-semibold uppercase tracking-wider text-fg-subtle">
                    Mechanism
                  </th>
                </tr>
              </thead>
              <tbody className="text-fg-muted">
                <tr className="border-b border-border">
                  <td className="py-3 px-4 text-fg">
                    Skills / agents / commands
                  </td>
                  <td className="py-3 px-4">
                    ~/.ccpm/share/&lt;kind&gt;/&lt;name&gt;
                  </td>
                  <td className="py-3 px-4">
                    &lt;profile&gt;/&lt;kind&gt;/&lt;name&gt;
                  </td>
                  <td className="py-3 px-4">Symlink</td>
                </tr>
                <tr className="border-b border-border">
                  <td className="py-3 px-4 text-fg">MCP servers</td>
                  <td className="py-3 px-4">
                    ~/.ccpm/share/mcp/&#123;global,&lt;profile&gt;&#125;.json
                  </td>
                  <td className="py-3 px-4">
                    &lt;profile&gt;/settings.json#mcpServers
                  </td>
                  <td className="py-3 px-4">Merge at launch</td>
                </tr>
                <tr>
                  <td className="py-3 px-4 text-fg">Settings</td>
                  <td className="py-3 px-4">
                    ~/.ccpm/share/settings/&#123;global,&lt;profile&gt;&#125;.json
                  </td>
                  <td className="py-3 px-4">&lt;profile&gt;/settings.json</td>
                  <td className="py-3 px-4">
                    Deep merge + owned-keys override
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

# global setting
ccpm settings set model claude-opus-4 --global`}
            lang="bash"
          />

          <p className="text-sm text-fg-muted">
            Run any install or remove command without <code>--global</code> or{" "}
            <code>--profile</code> in a TTY and ccpm asks which scope you want.
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

          <H3 id="mcp-commands">MCP commands</H3>
          <p>
            <code>ccpm mcp</code> writes entries into the shared MCP fragments
            at <code>~/.ccpm/share/mcp/global.json</code> or{" "}
            <code>~/.ccpm/share/mcp/&lt;profile&gt;.json</code>. Entries are
            merged into each profile&apos;s <code>settings.json</code> at
            launch.
          </p>
          <CodeBlock
            code={`# add a server with a command, arguments, and env vars
ccpm mcp add github \\
  --command "npx" \\
  --args "-y,@modelcontextprotocol/server-github" \\
  --env GITHUB_TOKEN=ghp_... \\
  --profile work

# globally-shared server
ccpm mcp add linear \\
  --command "npx -y @linear/mcp" \\
  --env LINEAR_API_KEY=lin_... --global

# list servers (alias: mcp ls)
ccpm mcp list

# remove a server (alias: mcp rm)
ccpm mcp remove github --profile work

# bulk import from a JSON file (keys become server names)
ccpm mcp import ./mcp-servers.json --global`}
            lang="bash"
          />
          <p className="text-sm text-fg-muted">
            <code>--args</code> takes a comma-separated list; <code>--env</code>{" "}
            takes comma-separated <code>KEY=VALUE</code> pairs and may be
            repeated.
          </p>

          <H3 id="settings-commands">Settings commands</H3>
          <p>
            Settings fragments live beside MCP fragments at{" "}
            <code>~/.ccpm/share/settings/</code> and are deep-merged into the
            profile&apos;s <code>settings.json</code>. Keys you set through
            ccpm are tracked in a <code>.owned.json</code> sidecar so Claude
            Code cannot silently overwrite them (see{" "}
            <a href="#settings-precedence">Settings precedence</a>).
          </p>
          <CodeBlock
            code={`# set a simple scalar globally
ccpm settings set model claude-opus-4 --global

# dot-notation nested key, per profile
ccpm settings set permissions.allow.Bash true --profile work

# JSON values (objects, arrays) are parsed automatically
ccpm settings set env.FOO '{"a":1,"b":2}' --global

# read the effective value for a profile
ccpm settings get model --profile work

# dump the fully merged settings for a profile
ccpm settings show --profile work

# apply a JSON fragment file (deep-merged into the target)
ccpm settings apply ./team-defaults.json --global`}
            lang="bash"
          />
          <p className="text-sm text-fg-muted">
            <code>ccpm settings get</code> and <code>show</code> require{" "}
            <code>--profile</code>; <code>set</code> and <code>apply</code>{" "}
            require one of <code>--global</code> / <code>--profile</code>.
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
            own OAuth session automatically.
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
            profile by merging fragments in this order:
          </p>
          <ol>
            <li>
              <code>~/.ccpm/share/settings/global.json</code>
            </li>
            <li>
              <code>~/.ccpm/share/settings/&lt;profile&gt;.json</code>{" "}
              (deep-merged over global)
            </li>
            <li>
              The profile&apos;s existing <code>settings.json</code>{" "}
              (deep-merged on top)
            </li>
            <li>
              <strong>Owned-keys override.</strong> Any leaf key you set via{" "}
              <code>ccpm settings set</code> or <code>ccpm settings apply</code>{" "}
              is recorded in a <code>.owned.json</code> sidecar and re-applied
              from the fragment after step 3. This guarantees that values you
              explicitly set through ccpm are never silently overwritten by
              Claude Code rewriting its own config.
            </li>
          </ol>
          <p>
            Objects merge key-by-key; arrays and scalars from a
            higher-precedence source replace the lower one.
          </p>

          <H2 id="doctor">Doctor</H2>
          <p>
            <code>ccpm doctor</code> is your one-stop health check. It never
            fails builds — warnings are informational — but it will tell you
            when something is actually broken so you don&apos;t chase ghosts.
          </p>
          <p>It reports on, in order:</p>
          <ul>
            <li>
              <strong>Environment</strong> — ccpm version, platform, Claude Code
              binary path, and <code>claude --version</code> (with a warning on
              macOS if you&apos;re below v2.1.56, which is required for
              per-profile OAuth keychain isolation).
            </li>
            <li>
              <strong>ccpm base directory</strong> — confirms{" "}
              <code>~/.ccpm/</code> exists and is readable.
            </li>
            <li>
              <strong>Per-profile auth health</strong> — OAuth token validity
              and expiry for each profile. On macOS OAuth profiles, the
              namespaced keychain service name is printed so you can inspect the
              entry manually with Keychain Access.
            </li>
            <li>
              <strong>Root vs. profile diff</strong> — anything in{" "}
              <code>~/.claude</code> that no profile has adopted yet, and
              vice-versa. Prints a one-line hint pointing at the right{" "}
              <code>ccpm import</code> command.
            </li>
            <li>
              <strong>Symlink integrity</strong> — flags broken symlinks and
              copies under a profile that have drifted from the shared store.
            </li>
            <li>
              <strong>Shared asset manifest</strong> — how many skills, MCP
              servers, and settings keys are tracked in{" "}
              <code>~/.ccpm/installs.json</code>.
            </li>
            <li>
              <strong>Drift fingerprint</strong> — detects when{" "}
              <code>~/.claude</code> has changed since the last{" "}
              <code>ccpm import default</code> snapshot.
            </li>
            <li>
              <strong>Drift notifications</strong> — whether the{" "}
              <code>check_default_drift</code> config flag is on (see{" "}
              <a href="#drift">Drift detection</a>).
            </li>
            <li>
              <strong>Platform notes</strong> — platform-specific caveats such
              as the Windows symlink fallback marker and global-cache MCP
              isolation limits.
            </li>
          </ul>
          <p className="text-sm text-fg-muted">
            Exit code is 0 on success or when only warnings are present, and 1
            when real issues are detected.
          </p>
          <CodeBlock
            code={`$ ccpm doctor
Environment
  ccpm       0.3.0
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
            your profiles were built from — so a skill you tweaked in{" "}
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
            suggests the right <code>ccpm import default --profile &lt;name&gt;</code>{" "}
            to sync changes into a profile. Run <code>update</code> to accept
            the current state without importing.
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

          <H2 id="uninstall">Uninstall</H2>
          <p>
            <code>ccpm uninstall</code> removes every profile, deletes API keys
            from the OS keychain, wipes vault backups, and deletes{" "}
            <code>~/.ccpm/</code>. It does <strong>not</strong> remove the{" "}
            <code>ccpm</code> binary itself or the shell hook you added to{" "}
            <code>~/.zshrc</code> / <code>~/.bashrc</code> — the command prints
            those cleanup steps so you can run them by hand.
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

          <H2 id="ide">IDE / VS Code</H2>
          <p>
            The VS Code Claude extension ignores <code>CLAUDE_CONFIG_DIR</code>{" "}
            and always reads from <code>~/.claude</code>. Use{" "}
            <code>set-default</code> to control which account VS Code uses.
            Call it without an argument in a TTY for a profile picker. Restart
            the extension after switching so it re-reads credentials.
          </p>
          <CodeBlock
            code={`# set which profile VS Code uses
ccpm set-default work
✓ profile "work" is now the default

# pick interactively
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
            Keychain, Linux Secret Service, Windows Credential Manager) — never
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
          <div className="not-prose my-6 overflow-x-auto rounded-xl border border-border bg-surface">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left py-3 px-4 font-mono text-[0.7rem] font-semibold uppercase tracking-wider text-fg-subtle">
                    Feature
                  </th>
                  <th className="text-left py-3 px-4 font-mono text-[0.7rem] font-semibold uppercase tracking-wider text-fg-subtle">
                    macOS
                  </th>
                  <th className="text-left py-3 px-4 font-mono text-[0.7rem] font-semibold uppercase tracking-wider text-fg-subtle">
                    Linux
                  </th>
                  <th className="text-left py-3 px-4 font-mono text-[0.7rem] font-semibold uppercase tracking-wider text-fg-subtle">
                    Windows
                  </th>
                </tr>
              </thead>
              <tbody className="text-fg-muted">
                <tr className="border-b border-border">
                  <td className="py-3 px-4 text-fg">OAuth per-profile</td>
                  <td className="py-3 px-4">
                    Keychain entry namespaced by profile dir
                  </td>
                  <td className="py-3 px-4">.credentials.json</td>
                  <td className="py-3 px-4">.credentials.json</td>
                </tr>
                <tr className="border-b border-border">
                  <td className="py-3 px-4 text-fg">API key storage</td>
                  <td className="py-3 px-4">Keychain</td>
                  <td className="py-3 px-4">Secret Service</td>
                  <td className="py-3 px-4">Credential Manager</td>
                </tr>
                <tr className="border-b border-border">
                  <td className="py-3 px-4 text-fg">Parallel sessions</td>
                  <td className="py-3 px-4">Yes</td>
                  <td className="py-3 px-4">Yes</td>
                  <td className="py-3 px-4">Yes</td>
                </tr>
                <tr className="border-b border-border">
                  <td className="py-3 px-4 text-fg">Shared skill dedup</td>
                  <td className="py-3 px-4">Symlinks</td>
                  <td className="py-3 px-4">Symlinks</td>
                  <td className="py-3 px-4">
                    Symlinks (Developer Mode) or copy fallback
                  </td>
                </tr>
                <tr>
                  <td className="py-3 px-4 text-fg">Shell hook</td>
                  <td className="py-3 px-4">zsh, bash, fish</td>
                  <td className="py-3 px-4">zsh, bash, fish</td>
                  <td className="py-3 px-4">PowerShell</td>
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
            specific ccpm profile. On macOS, this copies the profile&apos;s
            namespaced keychain entry into the default slot; on Linux and
            Windows it copies <code>.credentials.json</code>.
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
