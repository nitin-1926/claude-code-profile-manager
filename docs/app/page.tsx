import {
  ArrowUpRight,
  BookOpenCheck,
  Braces,
  Code2,
  EyeOff,
  Folder,
  GitFork,
  HardDrive,
  KeyRound,
  Layers,
  Lock,
  MessageSquare,
  Plug,
  ShieldCheck,
  Sparkles,
  Star,
  UserCheck,
  Zap,
} from "lucide-react";
import { Nav } from "./components/nav";
import { Footer } from "./components/footer";
import { Eyebrow } from "./components/eyebrow";
import { Button } from "./components/button";
import { CodeBlock } from "./components/code-block";
import { BentoTile } from "./components/bento-tile";
import { DotGrid, AccentOrb } from "./components/dot-grid";
import { InstallTabs } from "./components/install-tabs";
import {
  TerminalReel,
  type ReelStep,
} from "./components/terminal-reel";
import { VERSION_TAG } from "@/lib/version";

/* ─────────────────────────────────────────────────────────────────────────
   Hero terminal windows — animated scripts inside a fixed-height frame
   ───────────────────────────────────────────────────────────────────────── */

function HeroTerminal({
  title,
  script,
  startDelay = 0,
  minLines = 8,
  className = "",
  dim = false,
  ariaLabel,
}: {
  title: string;
  script: ReelStep[];
  startDelay?: number;
  minLines?: number;
  className?: string;
  dim?: boolean;
  /** Accessible summary of what the animation demonstrates. The terminal is
   *  decorative; without this a screen reader hears disjointed fragments
   *  ("ccpm add personal", "→ OAuth · browser opened", …) with no context. */
  ariaLabel?: string;
}) {
  return (
    <div
      role="img"
      aria-label={ariaLabel ?? `Demo terminal: ${title}`}
      className={`term-window term-boot ${className} ${dim ? "opacity-70" : ""}`}
    >
      <div className="term-window__chrome" aria-hidden="true">
        <span className="term-window__dot bg-[#ff5f56]" />
        <span className="term-window__dot bg-[#ffbd2e]" />
        <span className="term-window__dot bg-[#27c93f]" />
        <span className="ml-2 text-[10px] term-text-muted font-mono truncate">
          {title}
        </span>
      </div>
      <div
        className="px-4 py-3 font-mono text-[11px] leading-[1.8]"
        aria-hidden="true"
      >
        <TerminalReel
          script={script}
          startDelay={startDelay}
          minLines={minLines}
          loop
        />
      </div>
    </div>
  );
}

/* ─── Scripts: each terminal runs a different real-world scenario ───── */

const personalScript: ReelStep[] = [
  { text: "ccpm add personal", kind: "typed", prompt: "$", afterMs: 550 },
  {
    text: "→ OAuth · browser opened",
    kind: "instant",
    afterMs: 700,
    color: "muted",
  },
  {
    text: "✓ personal authenticated",
    kind: "instant",
    afterMs: 1200,
    color: "success",
  },
];

const workScript: ReelStep[] = [
  { text: "ccpm add work", kind: "typed", prompt: "$", afterMs: 500 },
  {
    text: "Anthropic API key: ************",
    kind: "instant",
    afterMs: 700,
    color: "muted",
  },
  {
    text: "✓ work authenticated (api key)",
    kind: "instant",
    afterMs: 1200,
    color: "success",
  },
];

const stagingScript: ReelStep[] = [
  { text: "ccpm list", kind: "typed", prompt: "$", afterMs: 450 },
  {
    text: "personal · work · staging ★",
    kind: "instant",
    afterMs: 700,
    color: "muted",
  },
  { text: "ccpm run staging", kind: "typed", prompt: "$", afterMs: 500 },
  {
    text: "→ activated (mcp: aws, sentry)",
    kind: "instant",
    afterMs: 1200,
    color: "success",
  },
];

/* ─────────────────────────────────────────────────────────────────────────
   Hero
   ───────────────────────────────────────────────────────────────────────── */
function Hero() {
  return (
    <section className="relative pt-16 pb-14 px-6 overflow-hidden">
      <DotGrid />
      <AccentOrb className="top-[-80px] right-[-15%] w-[640px] h-[640px]" />

      <div className="relative max-w-6xl mx-auto grid lg:grid-cols-12 gap-10 lg:gap-8 items-center">
        {/* Left: text content */}
        <div className="lg:col-span-6">
          <div className="mb-4 flex items-center gap-2">
            <span className="pill pill--accent">
              <span className="pulse-dot" />
              <span>{VERSION_TAG}</span>
            </span>
            <span className="pill">open source · MIT</span>
          </div>

          <h1
            className="font-semibold tracking-[-0.025em] leading-[1.05] text-fg"
            style={{ fontSize: "var(--t-display)" }}
          >
            Multiple Claude Code accounts.
            <br />
            <span className="accent-gradient-text">In parallel.</span>
          </h1>

          <p
            className="mt-5 text-fg-muted leading-relaxed max-w-lg"
            style={{ fontSize: "var(--t-body-lg)" }}
          >
            Personal in one terminal, work in another. Each with its own
            credentials, MCP servers, settings, and memory. Fully isolated.
            One command to switch.
          </p>

          <div className="mt-7 max-w-md">
            <InstallTabs />
          </div>

          <div className="mt-7 flex flex-wrap items-center gap-2.5">
            <Button href="/docs" variant="primary" size="md">
              Get started
              <ArrowUpRight size={14} strokeWidth={2.25} />
            </Button>
            <Button
              href="https://github.com/nitin-1926/claude-code-profile-manager"
              external
              variant="secondary"
              size="md"
            >
              View on GitHub
            </Button>
          </div>
        </div>

        {/* Right: 3 terminals in a diagonal cascade */}
        <div className="lg:col-span-6 relative min-h-[420px] hidden lg:block">
          <div className="absolute top-0 left-0 w-[85%] z-30">
            <HeroTerminal
              title="terminal · personal"
              script={personalScript}
              startDelay={400}
              minLines={4}
              ariaLabel="Demo: ccpm add personal — authenticate via OAuth"
            />
          </div>

          <div className="absolute top-[140px] left-[15%] w-[85%] z-20">
            <HeroTerminal
              title="terminal · work"
              script={workScript}
              startDelay={1600}
              minLines={4}
              ariaLabel="Demo: ccpm add work — authenticate via API key"
            />
          </div>

          <div className="absolute top-[280px] left-[8%] w-[80%] z-10">
            <HeroTerminal
              title="terminal · staging"
              script={stagingScript}
              startDelay={2800}
              minLines={4}
              dim
              ariaLabel="Demo: ccpm list, then ccpm run staging"
            />
          </div>
        </div>

        {/* Mobile: show terminals stacked */}
        <div className="lg:hidden space-y-3">
          <HeroTerminal
            title="terminal · personal"
            script={personalScript}
            startDelay={400}
            minLines={4}
            ariaLabel="Demo: ccpm add personal — authenticate via OAuth"
          />
          <HeroTerminal
            title="terminal · work"
            script={workScript}
            startDelay={1600}
            minLines={4}
            ariaLabel="Demo: ccpm add work — authenticate via API key"
          />
        </div>
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Tiny visual payloads used inside bento tiles
   ───────────────────────────────────────────────────────────────────────── */

function ParallelCodeSnippet() {
  return (
    <pre className="font-mono text-[11px] leading-6 text-fg-muted p-3.5 rounded-md bg-bg-subtle border border-border-subtle">
      <span className="text-accent">$</span> ccpm run personal{"  "}
      <span className="opacity-50"># terminal 1</span>
      {"\n"}
      <span className="text-accent">$</span> ccpm run work{"      "}
      <span className="opacity-50"># terminal 2</span>
      {"\n"}
      <span className="text-accent">$</span> ccpm run staging{"   "}
      <span className="opacity-50"># terminal 3</span>
    </pre>
  );
}

function AuthSplit() {
  return (
    <div className="grid grid-cols-2 gap-2 font-mono text-[11px]">
      <div className="rounded-md border border-border-subtle bg-bg-subtle p-3">
        <div className="text-accent text-[10px] uppercase tracking-[0.1em] font-semibold">
          oauth
        </div>
        <div className="mt-2 text-fg-muted">browser login</div>
        <div className="text-fg-muted">token in keychain</div>
      </div>
      <div className="rounded-md border border-border-subtle bg-bg-subtle p-3">
        <div className="text-accent text-[10px] uppercase tracking-[0.1em] font-semibold">
          api key
        </div>
        <div className="mt-2 text-fg-muted">sk-ant-…7f2k</div>
        <div className="text-fg-muted">per-profile secret</div>
      </div>
    </div>
  );
}

function McpStack() {
  const items = [
    { name: "github", scope: "personal" },
    { name: "jira", scope: "work" },
    { name: "datadog", scope: "work" },
    { name: "aws", scope: "staging" },
  ];
  return (
    <div className="space-y-1.5 font-mono text-[11px]">
      {items.map((i) => (
        <div
          key={i.name}
          className="flex items-center justify-between rounded-md border border-border-subtle bg-bg-subtle px-2.5 py-1.5"
        >
          <span className="flex items-center gap-2 text-fg">
            <span className="h-1.5 w-1.5 rounded-full bg-accent" />
            {i.name}
          </span>
          <span className="text-fg-subtle">{i.scope}</span>
        </div>
      ))}
    </div>
  );
}

function VaultBadge() {
  return (
    <div className="rounded-md border border-border-subtle bg-bg-subtle p-3">
      <div className="flex items-center justify-between font-mono text-[11px]">
        <span className="text-fg-muted">~/.ccpm/vault/</span>
        <span className="pill pill--accent">AES-256-GCM</span>
      </div>
      <div className="mt-2.5 grid grid-cols-6 gap-1">
        {Array.from({ length: 18 }).map((_, i) => (
          <span
            key={i}
            className="h-1.5 rounded-sm bg-accent/80"
            style={{ opacity: 0.25 + (i % 6) * 0.12 }}
          />
        ))}
      </div>
    </div>
  );
}

function IdeCard() {
  return (
    <div className="rounded-md border border-border-subtle bg-bg-subtle p-3 font-mono text-[11px]">
      <div className="flex items-center gap-2 text-fg">
        <Code2 size={12} strokeWidth={1.75} className="text-accent" />
        VS Code
        <span className="ml-auto text-fg-subtle">default: work</span>
      </div>
      <div className="mt-2.5 text-fg-muted">ccpm set-default work</div>
    </div>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Features: bento grid
   ───────────────────────────────────────────────────────────────────────── */
function Features() {
  return (
    <section id="features" className="relative py-20 px-6">
      <div className="gradient-line max-w-6xl mx-auto mb-20" />
      <div className="max-w-6xl mx-auto">
        <div className="mb-10 max-w-2xl reveal">
          <Eyebrow>{"// features"}</Eyebrow>
          <h2
            className="mt-2.5 font-semibold tracking-tight text-fg"
            style={{ fontSize: "var(--t-h2)" }}
          >
            Built for developers who juggle accounts.
          </h2>
          <p className="mt-2 text-fg-muted leading-relaxed text-[0.9375rem]">
            Every profile is a real filesystem sandbox — its own credentials,
            its own MCPs, its own memory. Switch with a single command.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-6 gap-3.5 auto-rows-[minmax(200px,auto)]">
          <BentoTile
            className="md:col-span-4 reveal reveal-1"
            eyebrow="core"
            title="True parallel sessions"
            description="Run personal and work Claude Code instances in separate terminals simultaneously. Each session has its own config, memory, and MCP servers. No leaking, no conflicts."
            icon={Layers}
          >
            <ParallelCodeSnippet />
          </BentoTile>

          <BentoTile
            className="md:col-span-2 reveal reveal-2"
            eyebrow="auth"
            title="Both auth modes"
            description="OAuth login or API key per profile. Mix and match across profiles."
            icon={Zap}
          >
            <AuthSplit />
          </BentoTile>

          <BentoTile
            className="md:col-span-2 reveal reveal-3"
            eyebrow="mcp"
            title="Isolated MCP servers"
            description="Different MCP configurations per profile. Work Jira stays in work."
            icon={Plug}
          >
            <McpStack />
          </BentoTile>

          <BentoTile
            className="md:col-span-2 reveal reveal-4"
            eyebrow="vault"
            title="Encrypted vault"
            description="AES-256-GCM backups with master key in your OS keychain. Safe machine migrations."
            icon={KeyRound}
          >
            <VaultBadge />
          </BentoTile>

          <BentoTile
            className="md:col-span-2 reveal reveal-5"
            eyebrow="ide"
            title="IDE-aware defaults"
            description="Set the active profile for VS Code with one command. Extension picks it up."
            icon={Code2}
          >
            <IdeCard />
          </BentoTile>
        </div>
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Anatomy — what lives inside each profile directory
   ───────────────────────────────────────────────────────────────────────── */

const profileAssets = [
  { name: ".credentials.json", hint: "OAuth tokens or API key", icon: KeyRound },
  { name: "settings.json", hint: "Model, permissions, hooks", icon: Braces },
  { name: ".claude.json", hint: "Native Claude state + MCP sessions", icon: Braces },
  { name: "CLAUDE.md", hint: "Per-profile memory", icon: BookOpenCheck },
  { name: "skills/", hint: "Symlinked into shared store", icon: Sparkles },
  { name: "commands/", hint: "Custom slash commands", icon: Code2 },
  { name: "agents/", hint: "Subagent definitions", icon: UserCheck },
  { name: "mcp/", hint: "Env vars, OAuth state", icon: Plug },
];

function Anatomy() {
  return (
    <section id="anatomy" className="relative py-20 px-6">
      <div className="gradient-line max-w-6xl mx-auto mb-20" />
      <div className="max-w-6xl mx-auto grid lg:grid-cols-12 gap-10 items-center">
        <div className="lg:col-span-5 reveal">
          <Eyebrow>{"// anatomy"}</Eyebrow>
          <h2
            className="mt-2.5 font-semibold tracking-tight text-fg"
            style={{ fontSize: "var(--t-h2)" }}
          >
            Every profile is a real directory.
          </h2>
          <p className="mt-2 text-fg-muted leading-relaxed text-[0.9375rem]">
            No magic daemon. No patched binary. ccpm sets{" "}
            <code>CLAUDE_CONFIG_DIR</code> and Claude Code reads its entire
            world from there. The isolation is physical.
          </p>
          <p className="mt-3 text-fg-muted leading-relaxed text-[0.9375rem]">
            Tokens live in your OS keychain. Shared skills are symlinked into a
            de-duplicated store. Everything else is plain files you can{" "}
            <code>cat</code>, diff, and back up.
          </p>
        </div>

        <div className="lg:col-span-7 reveal reveal-2">
          <div className="term-window">
            <div className="term-window__chrome">
              <span className="term-window__dot bg-[#ff5f56]" />
              <span className="term-window__dot bg-[#ffbd2e]" />
              <span className="term-window__dot bg-[#27c93f]" />
              <span className="ml-2 text-[10px] term-text-muted font-mono truncate">
                ~/.ccpm/profiles/work/
              </span>
            </div>
            <div className="px-4 py-3 font-mono text-[12px] leading-[1.75]">
              <div className="flex items-center gap-2 term-text-accent">
                <Folder size={12} strokeWidth={2} />
                <span>work/</span>
              </div>
              <ul className="mt-1 space-y-1 pl-2">
                {profileAssets.map((a, i) => {
                  const Icon = a.icon;
                  const last = i === profileAssets.length - 1;
                  return (
                    <li
                      key={a.name}
                      className="flex items-center gap-2 anatomy-row"
                      style={{ animationDelay: `${200 + i * 80}ms` }}
                    >
                      <span className="term-text-muted w-4">
                        {last ? "└─" : "├─"}
                      </span>
                      <Icon
                        size={12}
                        strokeWidth={1.75}
                        className="term-text-accent shrink-0"
                      />
                      <span className="term-text-fg">{a.name}</span>
                      <span className="term-text-muted">
                        &nbsp;&nbsp;{a.hint}
                      </span>
                    </li>
                  );
                })}
              </ul>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   How it works
   ───────────────────────────────────────────────────────────────────────── */
const steps = [
  {
    n: "01",
    title: "Create your profiles",
    desc: "ccpm add creates an isolated directory for each account under ~/.ccpm/profiles/. Every profile has its own credentials, settings, memory, and MCP config.",
    code: `$ ccpm add personal
Choose authentication method:
  1) OAuth (browser login)
  2) API key
> 1
✓ profile "personal" authenticated

$ ccpm add work
> 2
Enter your Anthropic API key: sk-ant-...
✓ profile "work" authenticated`,
  },
  {
    n: "02",
    title: "Run them side by side",
    desc: "ccpm run sets CLAUDE_CONFIG_DIR to the right profile directory and launches Claude Code. Open two terminals, run two profiles. They never interfere.",
    code: `# Terminal 1
$ ccpm run personal
→ activated personal (oauth, mcp: github)
Welcome to Claude Code

# Terminal 2
$ ccpm run work
→ activated work (api key, mcp: jira)
Welcome to Claude Code`,
  },
  {
    n: "03",
    title: "Manage from one place",
    desc: "See all profiles, their auth status, and the IDE default. One CLI to manage credentials, switch contexts, and keep everything organized.",
    code: `$ ccpm list
NAME       AUTH      STATUS
personal   oauth     ✓ nitin@gmail.com
work       api_key   ✓ sk-ant-...7f2k   ★

$ ccpm set-default work
✓ profile "work" is now the VS Code default`,
  },
];

function HowItWorks() {
  return (
    <section className="relative py-20 px-6 bg-bg-subtle">
      <div className="gradient-line max-w-6xl mx-auto mb-20" />
      <div className="max-w-6xl mx-auto">
        <div className="mb-14 max-w-2xl">
          <Eyebrow>{"// how it works"}</Eyebrow>
          <h2
            className="mt-2.5 font-semibold tracking-tight text-fg"
            style={{ fontSize: "var(--t-h2)" }}
          >
            Three steps. No daemons, no patches.
          </h2>
          <p className="mt-2 text-fg-muted leading-relaxed text-[0.9375rem]">
            Built on a single official primitive: <code>CLAUDE_CONFIG_DIR</code>.
            That is it.
          </p>
        </div>

        <div className="space-y-16">
          {steps.map((step, i) => {
            const reverse = i % 2 === 1;
            return (
              <div
                key={step.n}
                className={`grid lg:grid-cols-12 gap-8 lg:gap-10 items-center ${
                  reverse ? "lg:[&>*:first-child]:col-start-7" : ""
                }`}
              >
                <div
                  className={`lg:col-span-6 ${reverse ? "lg:order-2" : ""}`}
                >
                  <div className="font-mono text-[2rem] font-semibold leading-none mb-3 accent-gradient-text inline-block opacity-60">
                    {step.n}
                  </div>
                  <h3 className="text-xl font-semibold tracking-tight text-fg mb-2">
                    {step.title}
                  </h3>
                  <p className="text-fg-muted leading-relaxed text-[0.9375rem]">
                    {step.desc}
                  </p>
                </div>
                <div
                  className={`lg:col-span-6 ${reverse ? "lg:order-1" : ""}`}
                >
                  <CodeBlock code={step.code} lang="bash" />
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Privacy
   ───────────────────────────────────────────────────────────────────────── */
const privacyPoints = [
  { icon: EyeOff, text: "Zero telemetry, analytics, or tracking" },
  { icon: UserCheck, text: "No data collection. We do not know you exist" },
  {
    icon: KeyRound,
    text: "API keys stored in your OS keychain, never plaintext",
  },
  { icon: ShieldCheck, text: "Vault backups encrypted with AES-256-GCM" },
  { icon: HardDrive, text: "Everything lives in ~/.ccpm/ on your machine" },
  {
    icon: BookOpenCheck,
    text: "Fully open source. MIT licensed. Audit the code",
  },
];

function Privacy() {
  return (
    <section id="privacy" className="relative py-20 px-6">
      <div className="gradient-line max-w-6xl mx-auto mb-20" />
      <div className="max-w-3xl mx-auto text-center">
        <Eyebrow>{"// privacy"}</Eyebrow>
        <h2
          className="mt-2.5 font-semibold tracking-tight text-fg"
          style={{ fontSize: "var(--t-h2)" }}
        >
          100% local. 100% private.
        </h2>
        <p className="mt-2 text-fg-muted leading-relaxed mb-10 text-[0.9375rem]">
          ccpm never makes network requests. Your credentials, config, and data
          stay on your machine. Always.
        </p>

        <div className="relative p-[1px] rounded-2xl bg-gradient-to-br from-[var(--c-accent-light)] via-[var(--c-accent)] to-[var(--c-accent-dark)] opacity-95">
          <div className="rounded-2xl bg-surface-raised p-7 sm:p-9">
            <Lock
              size={30}
              strokeWidth={1.5}
              className="text-accent mx-auto mb-6 opacity-80"
            />
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-8 gap-y-5 text-left max-w-2xl mx-auto">
              {privacyPoints.map((p, i) => (
                <div
                  key={i}
                  className="flex items-start gap-3 animate-fade-up"
                  style={{ animationDelay: `${i * 55}ms` }}
                >
                  <div className="mt-0.5 shrink-0 w-7 h-7 rounded-md bg-accent-muted border border-accent/20 flex items-center justify-center">
                    <p.icon
                      size={13}
                      strokeWidth={1.9}
                      className="text-accent"
                    />
                  </div>
                  <p className="text-[0.875rem] text-fg-muted leading-relaxed">
                    {p.text}
                  </p>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Community
   ───────────────────────────────────────────────────────────────────────── */
function Community() {
  return (
    <section className="relative py-20 px-6 bg-bg-subtle">
      <div className="gradient-line max-w-6xl mx-auto mb-20" />
      <div className="max-w-4xl mx-auto">
        <div className="text-center mb-12">
          <Eyebrow>{"// community"}</Eyebrow>
          <h2
            className="mt-2.5 font-semibold tracking-tight text-fg"
            style={{ fontSize: "var(--t-h2)" }}
          >
            Built in the open. Actively maintained.
          </h2>
          <p className="mt-2 text-fg-muted leading-relaxed max-w-xl mx-auto text-[0.9375rem]">
            ccpm is a solo project I use every day. If it saves you time too, I
            would love your support.
          </p>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3.5">
          {[
            {
              icon: Star,
              title: "Star on GitHub",
              desc: "Stars help others discover ccpm.",
              href: "https://github.com/nitin-1926/claude-code-profile-manager",
            },
            {
              icon: MessageSquare,
              title: "Report issues",
              desc: "Bug? Feature request? Open an issue.",
              href: "https://github.com/nitin-1926/claude-code-profile-manager/issues",
            },
            {
              icon: GitFork,
              title: "Contribute",
              desc: "PRs welcome. Check the contributing guide.",
              href: "https://github.com/nitin-1926/claude-code-profile-manager/blob/main/CONTRIBUTING.md",
            },
          ].map((c) => {
            const Icon = c.icon;
            return (
              <a
                key={c.title}
                href={c.href}
                target="_blank"
                rel="noopener noreferrer"
                className="surface-card group flex flex-col items-center text-center gap-2.5 p-7"
              >
                <span className="inline-flex h-10 w-10 items-center justify-center rounded-md bg-accent-muted border border-accent/20">
                  <Icon
                    size={18}
                    strokeWidth={1.75}
                    className="text-accent transition-transform group-hover:scale-110"
                  />
                </span>
                <span className="font-semibold text-fg text-[0.9375rem]">
                  {c.title}
                </span>
                <span className="text-[0.8125rem] text-fg-muted">
                  {c.desc}
                </span>
              </a>
            );
          })}
        </div>
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   CTA
   ───────────────────────────────────────────────────────────────────────── */
function CTA() {
  return (
    <section className="relative py-20 px-6 overflow-hidden">
      <div className="gradient-line max-w-6xl mx-auto mb-20" />
      <AccentOrb className="bottom-[-200px] left-1/2 -translate-x-1/2 w-[720px] h-[360px]" />
      <div className="relative max-w-3xl mx-auto text-center">
        <h2
          className="font-semibold tracking-tight text-fg"
          style={{ fontSize: "var(--t-h2)" }}
        >
          One install. Every account.
        </h2>
        <p className="mt-2 text-fg-muted leading-relaxed text-[0.9375rem]">
          Free, open source, MIT licensed. No account required. No credit card.
          No tracking.
        </p>
        <div className="mt-6 flex flex-wrap items-center justify-center gap-2.5">
          <Button href="/docs" variant="primary" size="md">
            Read the docs
            <ArrowUpRight size={14} strokeWidth={2.25} />
          </Button>
          <Button
            href="https://github.com/nitin-1926/claude-code-profile-manager"
            external
            variant="secondary"
            size="md"
          >
            Browse source
          </Button>
        </div>
      </div>
    </section>
  );
}

export default function Home() {
  return (
    <>
      <Nav />
      <main id="main" className="flex-1">
        <Hero />
        <Features />
        <Anatomy />
        <HowItWorks />
        <Privacy />
        <Community />
        <CTA />
      </main>
      <Footer />
    </>
  );
}
