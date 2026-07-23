import { Tabs } from "./tabs";
import { CodeBlock } from "./code-block";
import { DESKTOP_DMG, DESKTOP_RELEASES_URL } from "@/lib/version";

const CURL_CMD =
  "curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh";

const SOURCE_CMD = `git clone https://github.com/nitin-1926/claude-code-profile-manager.git
cd claude-code-profile-manager/ccpm
go build -o ccpm .
./ccpm --version`;

export function InstallTabs() {
  return (
    <Tabs
      tabs={[
        {
          id: "go",
          label: "go",
          content: (
            <CodeBlock
              code="go install github.com/nitin-1926/claude-code-profile-manager/ccpm@latest"
              lang="bash"
            />
          ),
        },
        {
          id: "npm",
          label: "npm",
          content: <CodeBlock code="npm i -g @ngcodes/ccpm" lang="bash" />,
        },
        {
          id: "curl",
          label: "curl",
          content: <CodeBlock code={CURL_CMD} lang="bash" />,
        },
        {
          id: "source",
          label: "source",
          content: <CodeBlock code={SOURCE_CMD} lang="bash" />,
        },
        {
          id: "desktop",
          label: "Desktop (.dmg)",
          content: (
            <div className="text-[0.875rem] text-fg-muted leading-relaxed [&_a]:text-accent [&_a]:underline [&_a]:underline-offset-2">
              Native macOS GUI —{" "}
              <a href={DESKTOP_DMG.appleSilicon}>Apple Silicon</a> or{" "}
              <a href={DESKTOP_DMG.intel}>Intel</a> .dmg (
              <a href={DESKTOP_RELEASES_URL} target="_blank" rel="noopener noreferrer">all releases</a>). Requires the ccpm CLI for writes.
            </div>
          ),
        },
      ]}
    />
  );
}
