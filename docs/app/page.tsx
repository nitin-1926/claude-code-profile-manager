import {
  ArrowUpRight,
  BookOpenCheck,
  Code2,
  EyeOff,
  GitFork,
  HardDrive,
  KeyRound,
  Layers,
  Lock,
  MessageSquare,
  Plug,
  ShieldCheck,
  Star,
  UserCheck,
  Zap,
} from "lucide-react";
import { Nav } from "./components/nav";
import { Footer } from "./components/footer";
import { Eyebrow } from "./components/eyebrow";
import { Button } from "./components/button";
import { Tabs } from "./components/tabs";
import { CodeBlock } from "./components/code-block";
import { BentoTile } from "./components/bento-tile";
import { DotGrid, AccentOrb } from "./components/dot-grid";

/* ─────────────────────────────────────────────────────────────────────────
   Mini terminal component for the hero showcase
   ───────────────────────────────────────────────────────────────────────── */
function MiniTerminal({
  title,
  children,
  className = "",
  delay = 0,
}: {
  title: string;
  children: React.ReactNode;
  className?: string;
  delay?: number;
}) {
  return (
    <div
      className={`rounded-xl overflow-hidden border border-[color:var(--c-code-border)] bg-[color:var(--c-code-bg)] shadow-2xl shadow-black/40 animate-fade-up ${className}`}
      style={{ animationDelay: `${delay}ms` }}
    >
      <div className="flex items-center gap-1.5 px-3.5 py-2 border-b border-[color:var(--c-code-border)] bg-black/40">
        <span className="w-2.5 h-2.5 rounded-full bg-[#FF5F56]" />
        <span className="w-2.5 h-2.5 rounded-full bg-[#FFBD2E]" />
        <span className="w-2.5 h-2.5 rounded-full bg-[#27C93F]" />
        <span className="ml-2 text-[10px] text-zinc-500 font-mono truncate">
          {title}
        </span>
      </div>
      <div className="px-4 py-3 font-mono text-[12px] leading-6 min-h-[120px]">
        {children}
      </div>
    </div>
  );
}

function TermLine({
  prompt,
  color = "fg",
  children,
}: {
  prompt?: boolean;
  color?: "fg" | "muted" | "accent" | "success";
  children: React.ReactNode;
}) {
  const colors = {
    fg: "text-zinc-100",
    muted: "text-zinc-400",
    accent: "text-orange-400",
    success: "text-green-400",
  };
  return (
    <div className={colors[color]}>
      {prompt && <span className="text-orange-400 select-none">$ </span>}
      {children}
    </div>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Hero
   ───────────────────────────────────────────────────────────────────────── */
function Hero() {
  return (
    <section className="relative pt-20 pb-8 px-6 overflow-hidden">
      <DotGrid className="opacity-60" />
      <AccentOrb className="top-10 right-[-10%] w-[600px] h-[600px]" />

      <div className="relative max-w-6xl mx-auto text-center">
        <div className="mb-5">
          <Eyebrow>{"// v0.1 . open source . cross-platform"}</Eyebrow>
        </div>
        <h1
          className="font-semibold tracking-[-0.025em] leading-[1.05] text-fg max-w-4xl mx-auto"
          style={{ fontSize: "var(--t-display)" }}
        >
          Run multiple Claude Code accounts.{" "}
          <span className="text-accent">In parallel.</span>
        </h1>
        <p
          className="mt-6 text-fg-muted leading-relaxed max-w-2xl mx-auto"
          style={{ fontSize: "var(--t-body-lg)" }}
        >
          Personal account in one terminal. Work account in another. Each with
          its own credentials, MCP servers, settings, and memory. Fully
          isolated. One command to switch.
        </p>

        <div className="mt-8 flex justify-center">
          <div className="max-w-md w-full">
            <Tabs
              tabs={[
                {
                  id: "npm",
                  label: "npm",
                  content: (
                    <CodeBlock code="npm i -g @ngcodes/ccpm" lang="bash" />
                  ),
                },
                {
                  id: "curl",
                  label: "curl",
                  content: (
                    <CodeBlock
                      code="curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh"
                      lang="bash"
                    />
                  ),
                },
                {
                  id: "go",
                  label: "go",
                  content: (
                    <CodeBlock
                      code="go install github.com/nitin-1926/ccpm@latest"
                      lang="bash"
                    />
                  ),
                },
              ]}
            />
          </div>
        </div>

        <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
          <Button href="/docs" variant="primary" size="md">
            Get started
            <ArrowUpRight size={15} strokeWidth={2} />
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

      {/* Multi-terminal showcase */}
      <div className="relative max-w-6xl mx-auto mt-16">
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          {/* Terminal 1: Setup */}
          <MiniTerminal title="setup" delay={100}>
            <TermLine prompt>ccpm add personal</TermLine>
            <TermLine color="muted">Auth: OAuth (browser login)</TermLine>
            <TermLine color="success">
              ✓ Profile &quot;personal&quot; created
            </TermLine>
            <TermLine prompt>ccpm add work</TermLine>
            <TermLine color="muted">Auth: API key</TermLine>
            <TermLine color="success">
              ✓ Profile &quot;work&quot; created
            </TermLine>
          </MiniTerminal>

          {/* Terminal 2: Personal session */}
          <MiniTerminal title="terminal 1 — personal" delay={300}>
            <TermLine prompt>ccpm run personal</TermLine>
            <TermLine color="accent">
              → activated personal (oauth)
            </TermLine>
            <TermLine color="fg">
              Claude Code v1.0
            </TermLine>
            <TermLine color="muted">
              nitin@gmail.com
            </TermLine>
            <TermLine color="muted">
              mcp: github, slack
            </TermLine>
            <TermLine color="fg">
              &gt; Review my PR #42...
            </TermLine>
          </MiniTerminal>

          {/* Terminal 3: Work session */}
          <MiniTerminal title="terminal 2 — work" delay={500}>
            <TermLine prompt>ccpm run work</TermLine>
            <TermLine color="accent">
              → activated work (api key)
            </TermLine>
            <TermLine color="fg">
              Claude Code v1.0
            </TermLine>
            <TermLine color="muted">
              key: sk-ant-...7f2k
            </TermLine>
            <TermLine color="muted">
              mcp: jira, datadog
            </TermLine>
            <TermLine color="fg">
              &gt; Debug the auth service...
            </TermLine>
          </MiniTerminal>
        </div>
        <p className="text-center text-xs text-fg-subtle mt-4 font-mono">
          Two accounts. Two terminals. Zero conflicts.
        </p>
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Features — bento grid with stagger animation
   ───────────────────────────────────────────────────────────────────────── */
function Features() {
  return (
    <section id="features" className="relative py-24 px-6">
      <div className="gradient-line max-w-6xl mx-auto mb-24" />
      <div className="max-w-6xl mx-auto">
        <div className="mb-12 max-w-2xl">
          <Eyebrow>{"// features"}</Eyebrow>
          <h2
            className="mt-3 font-semibold tracking-tight text-fg"
            style={{ fontSize: "var(--t-h2)" }}
          >
            Built for developers who juggle accounts.
          </h2>
          <p className="mt-3 text-fg-muted leading-relaxed">
            One binary. Zero dependencies. Every feature exists because profile
            isolation should be a first-class primitive, not a workaround.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-6 lg:grid-rows-2 gap-4">
          <BentoTile
            className="lg:col-span-3 lg:row-span-1 animate-fade-up stagger-1"
            title="True parallel sessions"
            description="Run personal and work Claude Code instances in separate terminals at the same time. Each session is completely isolated with its own config, memory, and MCP servers."
            icon={Layers}
          >
            <pre className="font-mono text-[11px] leading-6 text-fg-muted mt-2 p-3 rounded-lg bg-bg/50 border border-border">
              <span className="text-orange-400">$</span> ccpm run personal{"  "}
              <span className="opacity-50"># terminal 1</span>
              {"\n"}
              <span className="text-orange-400">$</span> ccpm run work{"      "}
              <span className="opacity-50"># terminal 2</span>
              {"\n"}
              <span className="opacity-50"># both running simultaneously, zero conflicts</span>
            </pre>
          </BentoTile>

          <BentoTile
            className="lg:col-span-2 lg:row-span-1 animate-fade-up stagger-2"
            title="Encrypted vault"
            description="AES-256-GCM encrypted backups of your credentials. Master key stored in your OS keychain. Migrate between machines without exposing secrets."
            icon={KeyRound}
          />

          <BentoTile
            className="lg:col-span-1 lg:row-span-2 animate-fade-up stagger-3"
            title="Both auth modes"
            description="OAuth login or API key per profile. Mix and match however you need."
            icon={Zap}
          >
            <div className="mt-3 space-y-2.5 font-mono text-[11px]">
              <div className="flex items-center gap-2">
                <span className="w-1.5 h-1.5 rounded-full bg-green-400" />
                <span className="text-fg-muted">oauth login</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="w-1.5 h-1.5 rounded-full bg-green-400" />
                <span className="text-fg-muted">api key</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="w-1.5 h-1.5 rounded-full bg-green-400" />
                <span className="text-fg-muted">per-profile</span>
              </div>
            </div>
          </BentoTile>

          <BentoTile
            className="lg:col-span-2 lg:row-span-1 animate-fade-up stagger-4"
            title="Isolated MCP servers"
            description="Different MCP configurations per profile. Your work Jira integration never leaks into your personal setup."
            icon={Plug}
          />

          <BentoTile
            className="lg:col-span-2 lg:row-span-1 animate-fade-up stagger-5"
            title="IDE-aware defaults"
            description="Set the active profile for VS Code with one command. The Claude extension picks up the right credentials instantly."
            icon={Code2}
          />
        </div>
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   How it works — alternating layout
   ───────────────────────────────────────────────────────────────────────── */
const steps = [
  {
    n: "01",
    title: "Create your profiles",
    desc: "ccpm add creates an isolated directory for each account under ~/.ccpm/profiles/. Every profile has its own credentials, settings, memory, and MCP config. Add as many as you need.",
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
    desc: "ccpm run sets CLAUDE_CONFIG_DIR to the right profile directory and launches Claude Code. Open two terminals, run two profiles. They never interfere with each other.",
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
    title: "Manage everything from one place",
    desc: "See all your profiles, their auth status, and which one is the IDE default. One CLI to manage credentials, switch contexts, and keep everything organized.",
    code: `$ ccpm list
NAME       AUTH      STATUS
personal   oauth     ✓ nitin@gmail.com
work       api_key   ✓ sk-ant-...7f2k   ★

$ ccpm set-default work
✓ profile "work" is now the VS Code default`,
  },
];

async function HowItWorks() {
  return (
    <section className="relative py-24 px-6 bg-bg-subtle">
      <div className="gradient-line max-w-6xl mx-auto mb-24" />
      <div className="max-w-6xl mx-auto">
        <div className="mb-16 max-w-2xl">
          <Eyebrow>{"// how it works"}</Eyebrow>
          <h2
            className="mt-3 font-semibold tracking-tight text-fg"
            style={{ fontSize: "var(--t-h2)" }}
          >
            Three steps. No daemons, no patches.
          </h2>
          <p className="mt-3 text-fg-muted leading-relaxed">
            Built on a single official primitive:{" "}
            <code>CLAUDE_CONFIG_DIR</code>. That is it.
          </p>
        </div>

        <div className="space-y-20">
          {steps.map((step, i) => {
            const reverse = i % 2 === 1;
            return (
              <div
                key={step.n}
                className={`grid lg:grid-cols-12 gap-8 lg:gap-12 items-center ${
                  reverse ? "lg:[&>*:first-child]:col-start-7" : ""
                }`}
              >
                <div
                  className={`lg:col-span-6 ${reverse ? "lg:order-2" : ""}`}
                >
                  <div className="font-mono text-[2.5rem] font-semibold text-fg-subtle/30 leading-none mb-4">
                    {step.n}
                  </div>
                  <h3 className="text-2xl font-semibold tracking-tight text-fg mb-3">
                    {step.title}
                  </h3>
                  <p className="text-fg-muted leading-relaxed">
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
   Privacy — compact, visually distinct from features
   ───────────────────────────────────────────────────────────────────────── */
const privacyPoints = [
  { icon: EyeOff, text: "Zero telemetry, analytics, or tracking of any kind" },
  { icon: UserCheck, text: "No data collection. We do not know you exist" },
  { icon: KeyRound, text: "API keys stored in your OS keychain, never in plaintext" },
  { icon: ShieldCheck, text: "Vault backups use AES-256-GCM encryption" },
  { icon: HardDrive, text: "Everything lives in ~/.ccpm/ on your machine" },
  { icon: BookOpenCheck, text: "Fully open source, MIT licensed. Audit the code yourself" },
];

function Privacy() {
  return (
    <section id="privacy" className="relative py-24 px-6">
      <div className="gradient-line max-w-6xl mx-auto mb-24" />
      <div className="max-w-3xl mx-auto text-center">
        <Eyebrow>{"// privacy"}</Eyebrow>
        <h2
          className="mt-3 font-semibold tracking-tight text-fg"
          style={{ fontSize: "var(--t-h2)" }}
        >
          100% local. 100% private.
        </h2>
        <p className="mt-3 text-fg-muted leading-relaxed mb-12">
          ccpm never makes network requests. Your credentials, config, and data
          never leave your machine.
        </p>

        <div className="relative p-8 rounded-2xl border border-border bg-surface">
          <Lock
            size={40}
            strokeWidth={1.25}
            className="text-accent mx-auto mb-6 opacity-80"
          />
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-8 gap-y-5 text-left max-w-2xl mx-auto">
            {privacyPoints.map((p, i) => (
              <div
                key={i}
                className="flex items-start gap-3 animate-fade-up"
                style={{ animationDelay: `${i * 60}ms` }}
              >
                <div className="mt-0.5 shrink-0">
                  <p.icon
                    size={16}
                    strokeWidth={1.75}
                    className="text-accent"
                  />
                </div>
                <p className="text-sm text-fg-muted leading-relaxed">
                  {p.text}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Community / support — star on github, contribute, etc.
   ───────────────────────────────────────────────────────────────────────── */
function Community() {
  return (
    <section className="relative py-24 px-6 bg-bg-subtle">
      <div className="gradient-line max-w-6xl mx-auto mb-24" />
      <div className="max-w-4xl mx-auto">
        <div className="text-center mb-14">
          <Eyebrow>{"// community"}</Eyebrow>
          <h2
            className="mt-3 font-semibold tracking-tight text-fg"
            style={{ fontSize: "var(--t-h2)" }}
          >
            Built in the open. Actively maintained.
          </h2>
          <p className="mt-3 text-fg-muted leading-relaxed max-w-xl mx-auto">
            ccpm is a solo project that I use every day. If it helps you too, I
            would love to hear about it.
          </p>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <a
            href="https://github.com/nitin-1926/claude-code-profile-manager"
            target="_blank"
            rel="noopener noreferrer"
            className="group flex flex-col items-center gap-3 p-8 rounded-xl border border-border bg-surface hover:border-accent/50 hover:bg-surface-hover transition-all duration-200"
          >
            <Star
              size={28}
              strokeWidth={1.5}
              className="text-accent group-hover:scale-110 transition-transform"
            />
            <span className="font-semibold text-fg">Star on GitHub</span>
            <span className="text-xs text-fg-muted text-center">
              Show your support. Stars help others discover ccpm.
            </span>
          </a>

          <a
            href="https://github.com/nitin-1926/claude-code-profile-manager/issues"
            target="_blank"
            rel="noopener noreferrer"
            className="group flex flex-col items-center gap-3 p-8 rounded-xl border border-border bg-surface hover:border-accent/50 hover:bg-surface-hover transition-all duration-200"
          >
            <MessageSquare
              size={28}
              strokeWidth={1.5}
              className="text-accent group-hover:scale-110 transition-transform"
            />
            <span className="font-semibold text-fg">Report issues</span>
            <span className="text-xs text-fg-muted text-center">
              Found a bug? Have a feature request? Open an issue.
            </span>
          </a>

          <a
            href="https://github.com/nitin-1926/claude-code-profile-manager/blob/main/CONTRIBUTING.md"
            target="_blank"
            rel="noopener noreferrer"
            className="group flex flex-col items-center gap-3 p-8 rounded-xl border border-border bg-surface hover:border-accent/50 hover:bg-surface-hover transition-all duration-200"
          >
            <GitFork
              size={28}
              strokeWidth={1.5}
              className="text-accent group-hover:scale-110 transition-transform"
            />
            <span className="font-semibold text-fg">Contribute</span>
            <span className="text-xs text-fg-muted text-center">
              PRs welcome. Check the contributing guide to get started.
            </span>
          </a>
        </div>
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Final CTA
   ───────────────────────────────────────────────────────────────────────── */
function CTA() {
  return (
    <section className="relative py-24 px-6 overflow-hidden">
      <div className="gradient-line max-w-6xl mx-auto mb-24" />
      <AccentOrb className="bottom-[-200px] left-1/2 -translate-x-1/2 w-[800px] h-[400px]" />
      <div className="relative max-w-3xl mx-auto text-center">
        <h2
          className="font-semibold tracking-tight text-fg"
          style={{ fontSize: "var(--t-h2)" }}
        >
          One install. Every account.
        </h2>
        <p className="mt-3 text-fg-muted leading-relaxed">
          Free, open source, MIT licensed. No account required. No credit card.
          No tracking.
        </p>
        <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
          <Button href="/docs" variant="primary" size="md">
            Read the docs
            <ArrowUpRight size={15} strokeWidth={2} />
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

export default async function Home() {
  return (
    <>
      <Nav />
      <main className="flex-1">
        <Hero />
        <Features />
        <HowItWorks />
        <Privacy />
        <Community />
        <CTA />
      </main>
      <Footer />
    </>
  );
}
