import { Tabs } from "./tabs";
import { CodeBlock } from "./code-block";
import { Button } from "./button";

const CURL_CMD =
  "curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh";

export const DESKTOP_CMD =
  "curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install-desktop.sh | sh";

const SOURCE_CMD = `git clone https://github.com/nitin-1926/claude-code-profile-manager.git
cd claude-code-profile-manager/ccpm
go build -o ccpm .
./ccpm --version`;

const CLI_TABS = [
  {
    id: "npm",
    label: "npm",
    content: <CodeBlock code="npm i -g @ngcodes/ccpm" lang="bash" />,
  },
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
    id: "curl",
    label: "curl",
    content: <CodeBlock code={CURL_CMD} lang="bash" />,
  },
  {
    id: "source",
    label: "source",
    content: <CodeBlock code={SOURCE_CMD} lang="bash" />,
  },
];

export function DesktopInstall() {
  return (
    <div className="space-y-3">
      <CodeBlock code={DESKTOP_CMD} lang="bash" />
      <p className="text-[0.875rem] text-fg-muted leading-relaxed">
        Native macOS app. Picks your chip, verifies the checksum, installs to
        Applications and updates itself. Uses the ccpm CLI for changes.
      </p>
      <Button variant="secondary" size="sm" disabled aria-disabled="true">
        Download .dmg · Coming soon
      </Button>
    </div>
  );
}

export function InstallTabs() {
  return (
    <Tabs
      label="Install ccpm"
      tabs={[
        {
          id: "cli",
          label: "CLI",
          content: <Tabs label="CLI install method" tabs={CLI_TABS} />,
        },
        {
          id: "desktop",
          label: "Desktop",
          content: <DesktopInstall />,
        },
      ]}
    />
  );
}
