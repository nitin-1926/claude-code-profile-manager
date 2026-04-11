import Link from "next/link";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Documentation — ccpm",
  description: "Complete documentation for ccpm (Claude Code Profile Manager)",
};

function Nav() {
  return (
    <nav className="sticky top-0 z-50 backdrop-blur-md bg-[var(--background)]/80 border-b border-[var(--border)]">
      <div className="max-w-6xl mx-auto px-6 h-14 flex items-center justify-between">
        <Link href="/" className="font-bold text-lg tracking-tight">
          ccpm
        </Link>
        <div className="flex items-center gap-6 text-sm">
          <Link
            href="/docs"
            className="text-[var(--foreground)] font-medium"
          >
            Docs
          </Link>
          <a
            href="https://github.com/nitin-1926/claude-code-profile-manager"
            target="_blank"
            rel="noopener"
            className="text-[var(--muted)] hover:text-[var(--foreground)] transition-colors"
          >
            GitHub
          </a>
        </div>
      </div>
    </nav>
  );
}

function Section({
  id,
  title,
  children,
}: {
  id: string;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section id={id} className="scroll-mt-20 mb-16">
      <h2 className="text-2xl font-bold mb-6 pb-2 border-b border-[var(--border)]">
        {title}
      </h2>
      {children}
    </section>
  );
}

function Code({ children }: { children: string }) {
  return (
    <pre className="my-4">
      <code>{children}</code>
    </pre>
  );
}

function Sidebar() {
  const sections = [
    { id: "installation", label: "Installation" },
    { id: "quick-start", label: "Quick Start" },
    { id: "profiles", label: "Profile Management" },
    { id: "running", label: "Running Claude" },
    { id: "auth", label: "Authentication" },
    { id: "vault", label: "Vault Backup" },
    { id: "shell", label: "Shell Integration" },
    { id: "ide", label: "IDE / VS Code" },
    { id: "privacy", label: "Privacy & Security" },
    { id: "platforms", label: "Platform Support" },
    { id: "limitations", label: "Known Limitations" },
  ];

  return (
    <aside className="hidden lg:block w-56 flex-shrink-0">
      <div className="sticky top-20">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-[var(--muted)] mb-3">
          On this page
        </h3>
        <nav className="space-y-1.5">
          {sections.map((s) => (
            <a
              key={s.id}
              href={`#${s.id}`}
              className="block text-sm text-[var(--muted)] hover:text-[var(--foreground)] transition-colors py-0.5"
            >
              {s.label}
            </a>
          ))}
        </nav>
      </div>
    </aside>
  );
}

export default function DocsPage() {
  return (
    <>
      <Nav />
      <div className="max-w-6xl mx-auto px-6 py-12 flex gap-12">
        <Sidebar />
        <main className="flex-1 min-w-0 max-w-3xl">
          <h1 className="text-4xl font-bold mb-2">Documentation</h1>
          <p className="text-[var(--muted)] mb-12">
            Everything you need to manage multiple Claude Code accounts.
          </p>

          <Section id="installation" title="Installation">
            <h3 className="font-semibold text-lg mb-2">curl (macOS / Linux)</h3>
            <Code>{`curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh`}</Code>

            <h3 className="font-semibold text-lg mb-2 mt-6">Go</h3>
            <Code>{`go install github.com/nitin-1926/ccpm@latest`}</Code>

            <h3 className="font-semibold text-lg mb-2 mt-6">npm</h3>
            <Code>{`npm install -g ccpm`}</Code>

            <h3 className="font-semibold text-lg mb-2 mt-6">From source</h3>
            <Code>{`git clone https://github.com/nitin-1926/claude-code-profile-manager.git
cd claude-code-profile-manager
make build
# Binary at ./bin/ccpm`}</Code>
          </Section>

          <Section id="quick-start" title="Quick Start">
            <Code>{`# Create your first profile
ccpm add personal

# Create a work profile
ccpm add work

# Run them in parallel
ccpm run personal   # Terminal 1
ccpm run work       # Terminal 2

# Check status
ccpm status`}</Code>
          </Section>

          <Section id="profiles" title="Profile Management">
            <div className="space-y-6">
              <div>
                <h3 className="font-semibold text-lg mb-1">
                  <code className="text-[var(--accent)]">ccpm add &lt;name&gt;</code>
                </h3>
                <p className="text-[var(--muted)] text-sm mb-2">
                  Create a new profile. You&apos;ll choose between OAuth (browser login)
                  or API key authentication.
                </p>
                <Code>{`$ ccpm add personal
Choose authentication method:
  1) OAuth (browser login via claude /login)
  2) API Key (enter your Anthropic API key)
Enter choice [1/2]: 1

✓ Profile "personal" authenticated via OAuth`}</Code>
              </div>

              <div>
                <h3 className="font-semibold text-lg mb-1">
                  <code className="text-[var(--accent)]">ccpm list</code>
                </h3>
                <p className="text-[var(--muted)] text-sm mb-2">
                  List all profiles with their authentication status. Also available
                  as <code>ccpm ls</code>.
                </p>
              </div>

              <div>
                <h3 className="font-semibold text-lg mb-1">
                  <code className="text-[var(--accent)]">ccpm remove &lt;name&gt;</code>
                </h3>
                <p className="text-[var(--muted)] text-sm mb-2">
                  Delete a profile including its directory, keychain entries, and
                  vault backup. Use <code>--force</code> to skip confirmation.
                </p>
              </div>

              <div>
                <h3 className="font-semibold text-lg mb-1">
                  <code className="text-[var(--accent)]">ccpm status</code>
                </h3>
                <p className="text-[var(--muted)] text-sm mb-2">
                  Show system overview: ccpm version, Claude binary location, all
                  profiles and their auth health.
                </p>
              </div>
            </div>
          </Section>

          <Section id="running" title="Running Claude">
            <div className="space-y-6">
              <div>
                <h3 className="font-semibold text-lg mb-1">
                  <code className="text-[var(--accent)]">ccpm run &lt;name&gt;</code>
                  <span className="ml-2 text-xs bg-[var(--accent-light)] text-[var(--accent)] px-2 py-0.5 rounded-full">
                    recommended
                  </span>
                </h3>
                <p className="text-[var(--muted)] text-sm mb-2">
                  Launch Claude Code with the given profile. Sets{" "}
                  <code>CLAUDE_CONFIG_DIR</code> and <code>ANTHROPIC_API_KEY</code>{" "}
                  (for API key profiles) then replaces the process with Claude.
                  Works without any shell setup.
                </p>
                <Code>{`# Run with profile
ccpm run personal

# Pass arguments to claude
ccpm run work -- --model sonnet`}</Code>
              </div>

              <div>
                <h3 className="font-semibold text-lg mb-1">
                  <code className="text-[var(--accent)]">ccpm use &lt;name&gt;</code>
                </h3>
                <p className="text-[var(--muted)] text-sm mb-2">
                  Set the active profile for your entire shell session. Requires the{" "}
                  <a href="#shell" className="text-[var(--accent)] underline">
                    shell hook
                  </a>
                  . After running this, any <code>claude</code> command in that
                  terminal uses the selected profile.
                </p>
              </div>
            </div>
          </Section>

          <Section id="auth" title="Authentication">
            <div className="space-y-6">
              <div>
                <h3 className="font-semibold text-lg mb-1">
                  <code className="text-[var(--accent)]">ccpm auth status</code>
                </h3>
                <p className="text-[var(--muted)] text-sm mb-2">
                  Check credential validity for all profiles. Shows email for OAuth
                  profiles, masked key for API key profiles, and vault backup status.
                </p>
              </div>

              <div>
                <h3 className="font-semibold text-lg mb-1">
                  <code className="text-[var(--accent)]">ccpm auth refresh &lt;name&gt;</code>
                </h3>
                <p className="text-[var(--muted)] text-sm mb-2">
                  Re-authenticate a profile. For OAuth: launches Claude for{" "}
                  <code>/login</code>. For API key: prompts for a new key.
                </p>
              </div>
            </div>
          </Section>

          <Section id="vault" title="Vault Backup">
            <p className="text-[var(--muted)] text-sm mb-4">
              ccpm can create encrypted backups of your credentials for disaster
              recovery and machine migration. Uses AES-256-GCM encryption with a
              master key stored in your OS keychain.
            </p>
            <Code>{`# Backup credentials
ccpm auth backup personal

# Restore after machine migration
ccpm auth restore personal`}</Code>
          </Section>

          <Section id="shell" title="Shell Integration">
            <p className="text-[var(--muted)] text-sm mb-4">
              The shell hook wraps <code>ccpm use</code> so it can set environment
              variables in your current shell. Without it, <code>ccpm use</code>{" "}
              cannot modify your shell environment.
            </p>

            <h3 className="font-semibold mb-2">Setup</h3>
            <Code>{`# Add to ~/.zshrc or ~/.bashrc
eval "$(ccpm shell-init)"

# Reload
source ~/.zshrc`}</Code>

            <h3 className="font-semibold mb-2 mt-6">Usage</h3>
            <Code>{`# Set profile for this terminal session
ccpm use personal

# Now any 'claude' command uses the personal profile
claude`}</Code>

            <p className="text-[var(--muted)] text-sm mt-4">
              Supported shells: zsh, bash, fish, PowerShell
            </p>
          </Section>

          <Section id="ide" title="IDE / VS Code">
            <p className="text-[var(--muted)] text-sm mb-4">
              The VS Code Claude extension ignores <code>CLAUDE_CONFIG_DIR</code>{" "}
              and always reads from <code>~/.claude</code>. Use{" "}
              <code>set-default</code> to control which account VS Code uses.
            </p>
            <Code>{`# Set which profile VS Code uses
ccpm set-default work
✓ Profile "work" is now the default

# Clear the default
ccpm unset-default`}</Code>
          </Section>

          <Section id="privacy" title="Privacy & Security">
            <div className="p-6 rounded-xl border border-[var(--accent)] bg-[var(--accent-light)] mb-6">
              <p className="font-semibold text-lg mb-2">
                ccpm is 100% local. Your data never leaves your machine.
              </p>
              <p className="text-[var(--muted)] text-sm">
                No telemetry, analytics, or tracking of any kind. No network
                calls. No data collection. We don&apos;t know you exist.
              </p>
            </div>
            <div className="space-y-4 text-sm">
              <div className="p-4 border border-[var(--border)] rounded-lg">
                <h4 className="font-semibold mb-1">Credential Storage</h4>
                <p className="text-[var(--muted)]">
                  API keys are stored in your <strong>OS keychain</strong> (macOS
                  Keychain, Linux Secret Service, Windows Credential Manager) —
                  never in plaintext files. OAuth tokens are managed by Claude
                  Code itself within the isolated profile directory.
                </p>
              </div>
              <div className="p-4 border border-[var(--border)] rounded-lg">
                <h4 className="font-semibold mb-1">Encrypted Vault</h4>
                <p className="text-[var(--muted)]">
                  Vault backups use <strong>AES-256-GCM encryption</strong> with
                  a master key stored in your OS keychain. The encrypted files
                  live locally in <code>~/.ccpm/vault/</code>.
                </p>
              </div>
              <div className="p-4 border border-[var(--border)] rounded-lg">
                <h4 className="font-semibold mb-1">Local Config Only</h4>
                <p className="text-[var(--muted)]">
                  All configuration, profiles, and data live in{" "}
                  <code>~/.ccpm/</code> on your filesystem. No cloud storage, no
                  sync services, no external dependencies.
                </p>
              </div>
              <div className="p-4 border border-[var(--border)] rounded-lg">
                <h4 className="font-semibold mb-1">Open Source</h4>
                <p className="text-[var(--muted)]">
                  ccpm is fully open source under the MIT license.{" "}
                  <a
                    href="https://github.com/nitin-1926/claude-code-profile-manager"
                    className="text-[var(--accent)] underline"
                    target="_blank"
                    rel="noopener"
                  >
                    Audit the code yourself
                  </a>
                  .
                </p>
              </div>
            </div>
          </Section>

          <Section id="platforms" title="Platform Support">
            <div className="overflow-x-auto">
              <table className="w-full text-sm border-collapse">
                <thead>
                  <tr className="border-b border-[var(--border)]">
                    <th className="text-left py-2 pr-4 font-semibold">Feature</th>
                    <th className="text-left py-2 pr-4 font-semibold">macOS</th>
                    <th className="text-left py-2 pr-4 font-semibold">Linux</th>
                    <th className="text-left py-2 font-semibold">Windows</th>
                  </tr>
                </thead>
                <tbody className="text-[var(--muted)]">
                  <tr className="border-b border-[var(--border)]">
                    <td className="py-2 pr-4">OAuth</td>
                    <td className="py-2 pr-4">Keychain (per-profile)</td>
                    <td className="py-2 pr-4">.credentials.json</td>
                    <td className="py-2">.credentials.json</td>
                  </tr>
                  <tr className="border-b border-[var(--border)]">
                    <td className="py-2 pr-4">API key storage</td>
                    <td className="py-2 pr-4">Keychain</td>
                    <td className="py-2 pr-4">Secret Service</td>
                    <td className="py-2">Credential Manager</td>
                  </tr>
                  <tr className="border-b border-[var(--border)]">
                    <td className="py-2 pr-4">Parallel sessions</td>
                    <td className="py-2 pr-4">Yes</td>
                    <td className="py-2 pr-4">Yes</td>
                    <td className="py-2">Yes</td>
                  </tr>
                  <tr>
                    <td className="py-2 pr-4">Shell hook</td>
                    <td className="py-2 pr-4">zsh, bash, fish</td>
                    <td className="py-2 pr-4">zsh, bash, fish</td>
                    <td className="py-2">PowerShell</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </Section>

          <Section id="limitations" title="Known Limitations">
            <div className="space-y-4 text-sm">
              <div className="p-4 border border-[var(--border)] rounded-lg">
                <h4 className="font-semibold mb-1">
                  VS Code extension ignores CLAUDE_CONFIG_DIR
                </h4>
                <p className="text-[var(--muted)]">
                  The VS Code Claude extension always reads from{" "}
                  <code>~/.claude</code>. Use <code>ccpm set-default</code> to set
                  which account VS Code uses.
                </p>
              </div>
              <div className="p-4 border border-[var(--border)] rounded-lg">
                <h4 className="font-semibold mb-1">
                  CLAUDE_CONFIG_DIR path with ~/
                </h4>
                <p className="text-[var(--muted)]">
                  Claude has a bug resolving <code>~/</code> paths on Linux. ccpm
                  always uses absolute paths, so this is handled automatically.
                </p>
              </div>
              <div className="p-4 border border-[var(--border)] rounded-lg">
                <h4 className="font-semibold mb-1">
                  Headless Linux keychain
                </h4>
                <p className="text-[var(--muted)]">
                  <code>go-keyring</code> requires D-Bus and a secret service
                  (gnome-keyring or kwallet). On headless servers, API key profiles
                  need a running secret service.
                </p>
              </div>
            </div>
          </Section>
        </main>
      </div>
    </>
  );
}
