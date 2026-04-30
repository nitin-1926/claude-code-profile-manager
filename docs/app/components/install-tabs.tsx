import { Tabs } from "./tabs";
import { CodeBlock } from "./code-block";

const CURL_CMD =
  "curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh";

const SOURCE_CMD = `git clone https://github.com/nitin-1926/claude-code-profile-manager.git
cd claude-code-profile-manager
make build
# binary at ./bin/ccpm`;

export function InstallTabs() {
  return (
    <Tabs
      tabs={[
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
      ]}
    />
  );
}
