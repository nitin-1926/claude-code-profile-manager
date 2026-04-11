import Link from "next/link";

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
            className="text-[var(--muted)] hover:text-[var(--foreground)] transition-colors"
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

function Hero() {
  return (
    <section className="pt-24 pb-16 px-6">
      <div className="max-w-3xl mx-auto text-center">
        <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-[var(--accent-light)] text-[var(--accent)] text-xs font-medium mb-6">
          v0.1.0 &mdash; macOS, Linux, Windows
        </div>
        <h1 className="text-5xl sm:text-6xl font-bold tracking-tight leading-[1.1] mb-6">
          Multiple Claude accounts.
          <br />
          <span className="text-[var(--accent)]">Zero friction.</span>
        </h1>
        <p className="text-lg text-[var(--muted)] max-w-xl mx-auto mb-10 leading-relaxed">
          Run your personal and work Claude Code accounts in parallel &mdash;
          each with its own credentials, settings, MCP servers, and memory.
          Fully isolated. One command.
        </p>
        <div className="flex flex-col sm:flex-row items-center justify-center gap-3">
          <code className="bg-[var(--code-bg)] text-[var(--code-fg)] px-5 py-3 rounded-lg text-sm font-mono select-all">
            curl -fsSL
            https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh
            | sh
          </code>
        </div>
        <div className="flex items-center justify-center gap-4 mt-4 text-sm text-[var(--muted)]">
          <span>or</span>
          <code className="bg-[var(--code-bg)] text-[var(--code-fg)] px-3 py-1.5 rounded text-xs">
            go install github.com/nitin-1926/ccpm@latest
          </code>
          <span>or</span>
          <code className="bg-[var(--code-bg)] text-[var(--code-fg)] px-3 py-1.5 rounded text-xs">
            npm i -g ccpm
          </code>
        </div>
      </div>
    </section>
  );
}

function Terminal() {
  return (
    <section className="pb-20 px-6">
      <div className="max-w-2xl mx-auto">
        <div className="rounded-xl overflow-hidden shadow-2xl border border-[var(--border)]">
          <div className="bg-[#1e1e2e] px-4 py-2.5 flex items-center gap-2">
            <div className="w-3 h-3 rounded-full bg-[#f38ba8]" />
            <div className="w-3 h-3 rounded-full bg-[#f9e2af]" />
            <div className="w-3 h-3 rounded-full bg-[#a6e3a1]" />
            <span className="ml-2 text-xs text-[#6c7086]">terminal</span>
          </div>
          <pre className="!rounded-none !m-0 text-sm leading-7">
            <code>{`$ ccpm add personal
Choose authentication method:
  1) OAuth (browser login)
  2) API Key
Enter choice [1/2]: 1
✓ Profile "personal" authenticated via OAuth

$ ccpm add work
Enter choice [1/2]: 2
Enter your Anthropic API key: ****
✓ Profile "work" authenticated via API key

$ ccpm list
  NAME       AUTH     STATUS                     DEFAULT
  ──────────────────────────────────────────────────────
  personal   oauth    ✓ nitin@gmail.com
  work       api_key  ✓ key: sk-ant-...abcd      ★

$ ccpm run personal   # Terminal 1
$ ccpm run work       # Terminal 2`}</code>
          </pre>
        </div>
      </div>
    </section>
  );
}

function Features() {
  const features = [
    {
      title: "Parallel Sessions",
      description:
        "Run different Claude accounts in different terminals simultaneously. Each is fully isolated.",
      icon: "||",
    },
    {
      title: "OAuth + API Key",
      description:
        "First tool to properly support both OAuth browser login and API key authentication.",
      icon: "{}",
    },
    {
      title: "Encrypted Vault",
      description:
        "Backup and restore credentials with AES-256-GCM encryption. Master key in your OS keychain.",
      icon: "##",
    },
    {
      title: "Cross-Platform",
      description:
        "macOS Keychain, Linux Secret Service, Windows Credential Manager. Works everywhere.",
      icon: "//",
    },
    {
      title: "IDE Support",
      description:
        "Set which profile VS Code uses with ccpm set-default. Switch IDE accounts in seconds.",
      icon: "<>",
    },
    {
      title: "Shell Integration",
      description:
        "ccpm use sets the profile for your whole shell session. Works with zsh, bash, fish, PowerShell.",
      icon: "$_",
    },
  ];

  return (
    <section className="py-20 px-6 border-t border-[var(--border)]">
      <div className="max-w-5xl mx-auto">
        <h2 className="text-3xl font-bold text-center mb-12">
          Everything you need
        </h2>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {features.map((f) => (
            <div
              key={f.title}
              className="p-6 rounded-xl border border-[var(--border)] hover:border-[var(--accent)] transition-colors"
            >
              <div className="font-mono text-[var(--accent)] text-lg mb-3 font-bold">
                {f.icon}
              </div>
              <h3 className="font-semibold text-lg mb-2">{f.title}</h3>
              <p className="text-[var(--muted)] text-sm leading-relaxed">
                {f.description}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function HowItWorks() {
  return (
    <section className="py-20 px-6 border-t border-[var(--border)]">
      <div className="max-w-3xl mx-auto">
        <h2 className="text-3xl font-bold text-center mb-4">How it works</h2>
        <p className="text-center text-[var(--muted)] mb-12">
          Built on one official mechanism &mdash;{" "}
          <code className="text-xs bg-[var(--code-bg)] text-[var(--code-fg)] px-1.5 py-0.5 rounded">
            CLAUDE_CONFIG_DIR
          </code>
        </p>
        <div className="space-y-8">
          {[
            {
              step: "1",
              title: "Create isolated profiles",
              desc: "ccpm add creates a directory under ~/.ccpm/profiles/<name>/ with its own credentials, settings, and memory.",
            },
            {
              step: "2",
              title: "Launch with the right context",
              desc: "ccpm run sets CLAUDE_CONFIG_DIR to the profile directory and execs Claude. On macOS, each profile gets its own Keychain entry.",
            },
            {
              step: "3",
              title: "Fully isolated",
              desc: "Each terminal runs a completely separate Claude instance. Different accounts, different settings, different MCP servers. No conflicts.",
            },
          ].map((item) => (
            <div key={item.step} className="flex gap-5">
              <div className="flex-shrink-0 w-8 h-8 rounded-full bg-[var(--accent)] text-white flex items-center justify-center text-sm font-bold">
                {item.step}
              </div>
              <div>
                <h3 className="font-semibold mb-1">{item.title}</h3>
                <p className="text-[var(--muted)] text-sm leading-relaxed">
                  {item.desc}
                </p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function Privacy() {
  const points = [
    {
      title: "No telemetry or tracking",
      description: "Zero analytics, zero tracking, zero network calls. ccpm never contacts any server.",
      icon: "🚫",
    },
    {
      title: "No data collection",
      description: "We don't know you exist. No usage data, no error reporting, no phone-home behavior.",
      icon: "👤",
    },
    {
      title: "OS keychain storage",
      description: "API keys stored in macOS Keychain, Linux Secret Service, or Windows Credential Manager — never in plaintext.",
      icon: "🔐",
    },
    {
      title: "AES-256-GCM encryption",
      description: "Vault backups are encrypted with a master key stored in your OS keychain. Industry-standard encryption.",
      icon: "🛡️",
    },
    {
      title: "Local config only",
      description: "Everything lives in ~/.ccpm/ on your filesystem. No cloud, no sync, no external storage.",
      icon: "💾",
    },
    {
      title: "Fully open source",
      description: "Every line of code is public. Audit it yourself on GitHub. MIT licensed.",
      icon: "📖",
    },
  ];

  return (
    <section className="py-20 px-6 border-t border-[var(--border)]">
      <div className="max-w-5xl mx-auto">
        <h2 className="text-3xl font-bold text-center mb-3">
          100% local. 100% private.
        </h2>
        <p className="text-center text-[var(--muted)] mb-12 max-w-xl mx-auto">
          Your data never leaves your machine. ccpm does not collect, transmit,
          or store any data externally. Period.
        </p>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {points.map((p) => (
            <div
              key={p.title}
              className="p-6 rounded-xl border border-[var(--border)] hover:border-[var(--accent)] transition-colors"
            >
              <div className="text-2xl mb-3">{p.icon}</div>
              <h3 className="font-semibold text-lg mb-2">{p.title}</h3>
              <p className="text-[var(--muted)] text-sm leading-relaxed">
                {p.description}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function Footer() {
  return (
    <footer className="border-t border-[var(--border)] py-8 px-6 mt-auto">
      <div className="max-w-6xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-4 text-sm text-[var(--muted)]">
        <span>MIT License &mdash; built by Nitin Gupta</span>
        <div className="flex gap-6">
          <a
            href="https://github.com/nitin-1926/claude-code-profile-manager"
            target="_blank"
            rel="noopener"
            className="hover:text-[var(--foreground)] transition-colors"
          >
            GitHub
          </a>
          <a
            href="https://www.npmjs.com/package/ccpm"
            target="_blank"
            rel="noopener"
            className="hover:text-[var(--foreground)] transition-colors"
          >
            npm
          </a>
        </div>
      </div>
    </footer>
  );
}

export default function Home() {
  return (
    <>
      <Nav />
      <main className="flex-1">
        <Hero />
        <Terminal />
        <Features />
        <HowItWorks />
        <Privacy />
      </main>
      <Footer />
    </>
  );
}
