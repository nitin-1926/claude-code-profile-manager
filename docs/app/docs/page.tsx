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
          <CodeBlock code="go install github.com/nitin-1926/ccpm@latest" lang="bash" />

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

          <H2 id="profiles">Profile management</H2>

          <H3 id="profiles-add">ccpm add &lt;name&gt;</H3>
          <p>
            Create a new profile. You&apos;ll choose between OAuth (browser
            login) or API key authentication.
          </p>
          <CodeBlock
            code={`$ ccpm add personal
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
            vault backup. Use <code>--force</code> to skip confirmation.
          </p>

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

          <H3 id="running-use">ccpm use &lt;name&gt;</H3>
          <p>
            Set the active profile for your entire shell session. Requires the{" "}
            <a href="#shell">shell hook</a>. After running this, any{" "}
            <code>claude</code> command in that terminal uses the selected
            profile.
          </p>

          <H2 id="auth">Authentication</H2>

          <H3 id="auth-status">ccpm auth status</H3>
          <p>
            Check credential validity for all profiles. Shows email for OAuth
            profiles, masked key for API key profiles, and vault backup status.
          </p>

          <H3 id="auth-refresh">ccpm auth refresh &lt;name&gt;</H3>
          <p>
            Re-authenticate a profile. For OAuth: launches Claude for{" "}
            <code>/login</code>. For API key: prompts for a new key.
          </p>

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

          <H2 id="shell">Shell integration</H2>
          <p>
            The shell hook wraps <code>ccpm use</code> so it can set environment
            variables in your current shell. Without it, <code>ccpm use</code>{" "}
            cannot modify your shell environment.
          </p>

          <H3 id="shell-setup">Setup</H3>
          <CodeBlock
            code={`# add to ~/.zshrc or ~/.bashrc
eval "$(ccpm shell-init)"

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
          </p>
          <CodeBlock
            code={`# set which profile VS Code uses
ccpm set-default work
✓ profile "work" is now the default

# clear the default
ccpm unset-default`}
            lang="bash"
          />

          <H2 id="privacy">Privacy &amp; security</H2>

          <Callout type="tip" title="100% local">
            ccpm is fully local. <strong>Your data never leaves your machine.</strong>{" "}
            No telemetry, analytics, or tracking of any kind.
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
                  <td className="py-3 px-4 text-fg">OAuth</td>
                  <td className="py-3 px-4">Keychain (per profile)</td>
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
                <tr>
                  <td className="py-3 px-4 text-fg">Shell hook</td>
                  <td className="py-3 px-4">zsh, bash, fish</td>
                  <td className="py-3 px-4">zsh, bash, fish</td>
                  <td className="py-3 px-4">PowerShell</td>
                </tr>
              </tbody>
            </table>
          </div>

          <H2 id="limitations">Known limitations</H2>

          <Callout type="warn" title="VS Code extension ignores CLAUDE_CONFIG_DIR">
            The VS Code Claude extension always reads from{" "}
            <code>~/.claude</code>. Use <code>ccpm set-default</code> to set
            which account VS Code uses.
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
