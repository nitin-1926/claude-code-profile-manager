import {
  ArrowUpRight,
  BookOpenCheck,
  Code2,
  EyeOff,
  HardDrive,
  KeyRound,
  Layers,
  Plug,
  ShieldCheck,
  UserCheck,
  Workflow,
  Zap,
} from "lucide-react";
import { Nav } from "./components/nav";
import { Footer } from "./components/footer";
import { Eyebrow } from "./components/eyebrow";
import { Button } from "./components/button";
import { Tabs } from "./components/tabs";
import { CodeBlock } from "./components/code-block";
import { TerminalMock } from "./components/terminal-mock";
import { BentoTile } from "./components/bento-tile";
import { DotGrid, AccentOrb } from "./components/dot-grid";

function Hero() {
  return (
    <section className="relative pt-20 pb-24 px-6 overflow-hidden">
      <DotGrid className="opacity-60" />
      <AccentOrb className="top-20 right-[-10%] w-[600px] h-[600px]" />
      <div className="relative max-w-6xl mx-auto grid lg:grid-cols-12 gap-12 items-center">
        <div className="lg:col-span-7">
          <div className="mb-5">
            <Eyebrow>{"// v0.1 · CLI · macOS · linux · windows"}</Eyebrow>
          </div>
          <h1
            className="font-semibold tracking-[-0.025em] leading-[1.05] text-fg"
            style={{ fontSize: "var(--t-display)" }}
          >
            Switch Claude Code
            <br />
            profiles in{" "}
            <span className="text-accent">one command.</span>
          </h1>
          <p
            className="mt-6 text-fg-muted leading-relaxed max-w-xl"
            style={{ fontSize: "var(--t-body-lg)" }}
          >
            Run personal and work Claude Code accounts in parallel — each with
            its own credentials, settings, MCP servers, and memory. Fully
            isolated. 100% local.
          </p>

          <div className="mt-8 max-w-md">
            <Tabs
              tabs={[
                {
                  id: "npm",
                  label: "npm",
                  content: (
                    <CodeBlock
                      code="npm i -g @ngcodes/ccpm"
                      lang="bash"
                    />
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

        <div className="lg:col-span-5">
          <TerminalMock />
        </div>
      </div>
    </section>
  );
}

function Features() {
  return (
    <section
      id="features"
      className="relative py-24 px-6 border-t border-border"
    >
      <div className="max-w-6xl mx-auto">
        <div className="mb-12 max-w-2xl">
          <Eyebrow>{"// features"}</Eyebrow>
          <h2
            className="mt-3 font-semibold tracking-tight text-fg"
            style={{ fontSize: "var(--t-h2)" }}
          >
            Built for developers who context-switch.
          </h2>
          <p className="mt-3 text-fg-muted leading-relaxed">
            One binary. Zero magic. Every feature is the product of one design
            decision: profiles should be properly isolated, not just
            renamed-config-files.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-6 lg:grid-rows-2 gap-4">
          <BentoTile
            className="lg:col-span-3 lg:row-span-1"
            title="Parallel sessions"
            description="Run different Claude accounts in different terminals at the same time. Each is fully isolated — config, memory, MCP servers, the lot."
            icon={Layers}
          >
            <pre className="font-mono text-[11px] leading-6 text-fg-muted">
              <span className="text-accent">$</span> ccpm run personal{"  "}
              <span className="opacity-50"># terminal 1</span>
              {"\n"}
              <span className="text-accent">$</span> ccpm run work{"      "}
              <span className="opacity-50"># terminal 2</span>
            </pre>
          </BentoTile>

          <BentoTile
            className="lg:col-span-2 lg:row-span-1"
            title="Encrypted vault"
            description="AES-256-GCM encrypted backups. Master key in your OS keychain. Zero plaintext on disk."
            icon={KeyRound}
          />

          <BentoTile
            className="lg:col-span-1 lg:row-span-2"
            title="Both auth modes"
            description="OAuth login or API key. Whichever the profile needs."
            icon={Zap}
          >
            <div className="mt-2 space-y-2 font-mono text-[11px] text-fg-muted">
              <div>oauth ✓</div>
              <div>api key ✓</div>
              <div>per-profile</div>
            </div>
          </BentoTile>

          <BentoTile
            className="lg:col-span-2 lg:row-span-1"
            title="MCP servers"
            description="Different MCP setups per profile. No more cross-account config drift."
            icon={Plug}
          />

          <BentoTile
            className="lg:col-span-2 lg:row-span-1"
            title="IDE-aware"
            description="Set the active profile for VS Code with one command. The extension reads the right credentials."
            icon={Code2}
          />
        </div>
      </div>
    </section>
  );
}

const steps = [
  {
    n: "01",
    title: "Create isolated profiles",
    desc: (
      <>
        <code>ccpm add</code> creates a directory under{" "}
        <code>~/.ccpm/profiles/&lt;name&gt;/</code> with its own credentials,
        settings, and memory.
      </>
    ),
    code: `$ ccpm add work
Choose authentication method:
  1) OAuth (browser login)
  2) API key
> 1
✓ profile "work" authenticated`,
  },
  {
    n: "02",
    title: "Switch with one command",
    desc: (
      <>
        <code>ccpm run</code> sets <code>CLAUDE_CONFIG_DIR</code> to the
        profile directory and execs Claude. On macOS, each profile gets its own
        keychain entry.
      </>
    ),
    code: `$ ccpm run work
→ activated work
$ claude
Welcome to Claude Code`,
  },
  {
    n: "03",
    title: "Fully isolated",
    desc: (
      <>
        Each terminal runs a completely separate Claude instance. Different
        accounts, different settings, different MCP servers. No conflicts.
      </>
    ),
    code: `$ ccpm list
NAME       AUTH      STATUS
personal   oauth     ✓
work       api_key   ✓ ★`,
  },
];

async function HowItWorks() {
  return (
    <section className="relative py-24 px-6 border-t border-border bg-bg-subtle">
      <div className="max-w-6xl mx-auto">
        <div className="mb-16 max-w-2xl">
          <Eyebrow>{"// how it works"}</Eyebrow>
          <h2
            className="mt-3 font-semibold tracking-tight text-fg"
            style={{ fontSize: "var(--t-h2)" }}
          >
            One mechanism, three steps.
          </h2>
          <p className="mt-3 text-fg-muted leading-relaxed">
            Built on a single official primitive:{" "}
            <code>CLAUDE_CONFIG_DIR</code>. No daemons, no patches, no magic.
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
                  <div className="font-mono text-[2.5rem] font-semibold text-fg-subtle leading-none mb-4">
                    {step.n}
                  </div>
                  <h3 className="text-2xl font-semibold tracking-tight text-fg mb-3">
                    {step.title}
                  </h3>
                  <p className="text-fg-muted leading-relaxed [&_code]:text-fg [&_code]:bg-bg [&_code]:border [&_code]:border-border [&_code]:rounded [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:text-[0.85em]">
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

const privacyPoints = [
  {
    icon: EyeOff,
    title: "No telemetry",
    desc: "Zero analytics, zero tracking, zero network calls. ccpm never contacts any server.",
  },
  {
    icon: UserCheck,
    title: "No data collection",
    desc: "We don't know you exist. No usage data, no error reporting, no phone-home.",
  },
  {
    icon: KeyRound,
    title: "OS keychain storage",
    desc: "API keys live in macOS Keychain, Linux Secret Service, or Windows Credential Manager.",
  },
  {
    icon: ShieldCheck,
    title: "AES-256-GCM vault",
    desc: "Backups encrypted with a master key stored in your OS keychain. Industry-standard.",
  },
  {
    icon: HardDrive,
    title: "Local config only",
    desc: "Everything lives in ~/.ccpm/. No cloud, no sync, no external storage.",
  },
  {
    icon: BookOpenCheck,
    title: "Open source",
    desc: "Every line of code is public. Audit it yourself on GitHub. MIT licensed.",
  },
];

function Privacy() {
  return (
    <section
      id="privacy"
      className="relative py-24 px-6 border-t border-border"
    >
      <div className="max-w-6xl mx-auto">
        <div className="mb-12 max-w-2xl">
          <Eyebrow>{"// privacy"}</Eyebrow>
          <h2
            className="mt-3 font-semibold tracking-tight text-fg"
            style={{ fontSize: "var(--t-h2)" }}
          >
            100% local. 100% private.
          </h2>
          <p className="mt-3 text-fg-muted leading-relaxed">
            Your data never leaves your machine. ccpm does not collect,
            transmit, or store any data externally. Period.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {privacyPoints.map((p) => (
            <div
              key={p.title}
              className="p-6 rounded-xl border border-border bg-surface"
            >
              <div className="inline-flex items-center justify-center w-10 h-10 rounded-md bg-accent-muted mb-4">
                <p.icon size={20} strokeWidth={1.75} className="text-accent" />
              </div>
              <h3 className="text-base font-semibold tracking-tight text-fg mb-1.5">
                {p.title}
              </h3>
              <p className="text-sm text-fg-muted leading-relaxed">{p.desc}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function CTA() {
  return (
    <section className="relative py-24 px-6 border-t border-border overflow-hidden">
      <AccentOrb className="bottom-[-200px] left-1/2 -translate-x-1/2 w-[800px] h-[400px]" />
      <div className="relative max-w-3xl mx-auto text-center">
        <Eyebrow>{"// ready?"}</Eyebrow>
        <h2
          className="mt-4 font-semibold tracking-tight text-fg"
          style={{ fontSize: "var(--t-h2)" }}
        >
          One install. Every account.
        </h2>
        <p className="mt-3 text-fg-muted leading-relaxed">
          Free, open source, MIT licensed. No account, no credit card, no
          tracking.
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
            <Workflow size={15} strokeWidth={1.75} />
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
        <CTA />
      </main>
      <Footer />
    </>
  );
}
