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
   Mini terminal for hero showcase
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
      className={`rounded-xl overflow-hidden border border-[color:var(--c-code-border)] bg-[color:var(--c-code-bg)] shadow-2xl shadow-black/40 animate-fade-up accent-border-glow ${className}`}
      style={{ animationDelay: `${delay}ms` }}
    >
      <div className="flex items-center gap-1.5 px-3 py-2 border-b border-[color:var(--c-code-border)] bg-black/40">
        <span className="w-2.5 h-2.5 rounded-full bg-[#FF5F56]" />
        <span className="w-2.5 h-2.5 rounded-full bg-[#FFBD2E]" />
        <span className="w-2.5 h-2.5 rounded-full bg-[#27C93F]" />
        <span className="ml-2 text-[10px] text-zinc-500 font-mono truncate">
          {title}
        </span>
      </div>
      <div className="px-4 py-3 font-mono text-[11px] leading-[1.7] min-h-0">
        {children}
      </div>
    </div>
  );
}

function T({
  prompt,
  color = "fg",
  children,
}: {
  prompt?: boolean;
  color?: "fg" | "muted" | "accent" | "success";
  children: React.ReactNode;
}) {
  const c = {
    fg: "text-zinc-100",
    muted: "text-zinc-400",
    accent: "text-[#d77757]",
    success: "text-green-400",
  };
  return (
    <div className={c[color]}>
      {prompt && <span className="text-[#d77757] select-none">$ </span>}
      {children}
    </div>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Hero: left-aligned text + 3 diagonal terminals on right
   ───────────────────────────────────────────────────────────────────────── */
function Hero() {
  return (
    <section className="relative pt-20 pb-16 px-6 overflow-hidden">
      <DotGrid className="opacity-60" />
      <AccentOrb className="top-0 right-[-15%] w-[700px] h-[700px]" />

      <div className="relative max-w-6xl mx-auto grid lg:grid-cols-12 gap-12 lg:gap-8 items-center">
        {/* Left: text content */}
        <div className="lg:col-span-6">
          <div className="mb-5">
            <Eyebrow>{"// v0.1 . open source . cross-platform"}</Eyebrow>
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
            className="mt-6 text-fg-muted leading-relaxed max-w-lg"
            style={{ fontSize: "var(--t-body-lg)" }}
          >
            Personal account in one terminal, work account in another. Each with
            its own credentials, MCP servers, settings, and memory. Fully
            isolated. One command to switch.
          </p>

          <div className="mt-8 max-w-md">
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

          <div className="mt-8 flex flex-wrap items-center gap-3">
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

        {/* Right: 3 terminals in a diagonal cascade */}
        <div className="lg:col-span-6 relative min-h-[420px] hidden lg:block">
          {/* Terminal 1: top-left of the group */}
          <div className="absolute top-0 left-0 w-[85%] z-30">
            <MiniTerminal title="terminal 1 : personal" delay={200}>
              <T prompt>ccpm run personal</T>
              <T color="accent">→ activated personal (oauth)</T>
              <T color="fg">Claude Code v1.0</T>
              <T color="muted">nitin@gmail.com | mcp: github, slack</T>
              <T color="fg">&gt; Review my PR #42...</T>
            </MiniTerminal>
          </div>

          {/* Terminal 2: center-right, offset down */}
          <div className="absolute top-[140px] left-[15%] w-[85%] z-20">
            <MiniTerminal title="terminal 2 : work" delay={450}>
              <T prompt>ccpm run work</T>
              <T color="accent">→ activated work (api key)</T>
              <T color="fg">Claude Code v1.0</T>
              <T color="muted">sk-ant-...7f2k | mcp: jira, datadog</T>
              <T color="fg">&gt; Debug the auth service...</T>
            </MiniTerminal>
          </div>

          {/* Terminal 3: bottom-right, peeking below */}
          <div className="absolute top-[280px] left-[8%] w-[80%] z-10 opacity-60">
            <MiniTerminal title="terminal 3 : staging" delay={650}>
              <T prompt>ccpm run staging</T>
              <T color="accent">→ activated staging (api key)</T>
              <T color="fg">Claude Code v1.0</T>
              <T color="muted">sk-ant-...9x1m | mcp: aws, sentry</T>
            </MiniTerminal>
          </div>
        </div>

        {/* Mobile: show terminals stacked */}
        <div className="lg:hidden space-y-3">
          <MiniTerminal title="terminal 1 : personal" delay={200}>
            <T prompt>ccpm run personal</T>
            <T color="accent">→ activated personal (oauth)</T>
            <T color="muted">nitin@gmail.com | mcp: github, slack</T>
          </MiniTerminal>
          <MiniTerminal title="terminal 2 : work" delay={400}>
            <T prompt>ccpm run work</T>
            <T color="accent">→ activated work (api key)</T>
            <T color="muted">sk-ant-...7f2k | mcp: jira, datadog</T>
          </MiniTerminal>
        </div>
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Features: bento grid with stagger + gradient accent border
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
            One binary. Zero dependencies. Profile isolation as a first-class
            primitive, not a workaround.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-6 lg:grid-rows-2 gap-4">
          <BentoTile
            className="lg:col-span-3 lg:row-span-1 animate-fade-up stagger-1"
            title="True parallel sessions"
            description="Run personal and work Claude Code instances in separate terminals simultaneously. Each session has its own config, memory, and MCP servers. No leaking, no conflicts."
            icon={Layers}
          >
            <pre className="font-mono text-[11px] leading-6 text-fg-muted mt-2 p-3 rounded-lg bg-bg/50 border border-border">
              <span className="text-[#d77757]">$</span> ccpm run personal{"  "}
              <span className="opacity-50"># terminal 1</span>
              {"\n"}
              <span className="text-[#d77757]">$</span> ccpm run work{"      "}
              <span className="opacity-50"># terminal 2</span>
            </pre>
          </BentoTile>

          <BentoTile
            className="lg:col-span-2 lg:row-span-1 animate-fade-up stagger-2"
            title="Encrypted vault"
            description="AES-256-GCM encrypted credential backups. Master key stored in your OS keychain. Migrate machines without exposing secrets."
            icon={KeyRound}
          />

          <BentoTile
            className="lg:col-span-1 lg:row-span-2 animate-fade-up stagger-3"
            title="Both auth modes"
            description="OAuth login or API key per profile. Mix and match."
            icon={Zap}
          >
            <div className="mt-3 space-y-2.5 font-mono text-[11px]">
              <div className="flex items-center gap-2">
                <span className="w-1.5 h-1.5 rounded-full bg-[#d77757]" />
                <span className="text-fg-muted">oauth login</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="w-1.5 h-1.5 rounded-full bg-[#d77757]" />
                <span className="text-fg-muted">api key</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="w-1.5 h-1.5 rounded-full bg-[#d77757]" />
                <span className="text-fg-muted">per-profile</span>
              </div>
            </div>
          </BentoTile>

          <BentoTile
            className="lg:col-span-2 lg:row-span-1 animate-fade-up stagger-4"
            title="Isolated MCP servers"
            description="Different MCP configurations per profile. Work Jira stays in work, personal GitHub stays in personal."
            icon={Plug}
          />

          <BentoTile
            className="lg:col-span-2 lg:row-span-1 animate-fade-up stagger-5"
            title="IDE-aware defaults"
            description="Set the active profile for VS Code with one command. The Claude extension picks up the right credentials."
            icon={Code2}
          />
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
            Built on a single official primitive: <code>CLAUDE_CONFIG_DIR</code>
            . That is it.
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
                <div className={`lg:col-span-6 ${reverse ? "lg:order-2" : ""}`}>
                  <div className="font-mono text-[2.5rem] font-semibold leading-none mb-4 accent-gradient-text inline-block opacity-40">
                    {step.n}
                  </div>
                  <h3 className="text-2xl font-semibold tracking-tight text-fg mb-3">
                    {step.title}
                  </h3>
                  <p className="text-fg-muted leading-relaxed">{step.desc}</p>
                </div>
                <div className={`lg:col-span-6 ${reverse ? "lg:order-1" : ""}`}>
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
   Privacy: visually distinct centered layout with gradient border card
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
          stay on your machine. Always.
        </p>

        <div className="relative p-[1px] rounded-2xl bg-gradient-to-br from-[var(--c-accent-light)] via-[var(--c-accent)] to-[var(--c-accent-dark)] opacity-90">
          <div className="rounded-2xl bg-surface p-8 sm:p-10">
            <Lock
              size={36}
              strokeWidth={1.25}
              className="text-accent mx-auto mb-8 opacity-70"
            />
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-10 gap-y-6 text-left max-w-2xl mx-auto">
              {privacyPoints.map((p, i) => (
                <div
                  key={i}
                  className="flex items-start gap-3 animate-fade-up"
                  style={{ animationDelay: `${i * 60}ms` }}
                >
                  <div className="mt-0.5 shrink-0 w-7 h-7 rounded-md bg-accent-muted flex items-center justify-center">
                    <p.icon
                      size={14}
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
      </div>
    </section>
  );
}

/* ─────────────────────────────────────────────────────────────────────────
   Community
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
            ccpm is a solo project I use every day. If it saves you time too, I
            would love your support.
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
              Stars help others discover ccpm.
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
              Bug? Feature request? Open an issue.
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
              PRs welcome. Check the contributing guide.
            </span>
          </a>
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
