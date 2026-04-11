import { codeToHtml } from "shiki";
import { CodeBlockCopy } from "./code-block-copy";

type Props = {
  code: string;
  lang?: string;
  filename?: string;
};

export async function CodeBlock({ code, lang = "bash", filename }: Props) {
  const trimmed = code.replace(/\n+$/, "");
  const html = await codeToHtml(trimmed, {
    lang,
    themes: {
      light: "github-light",
      dark: "github-dark-default",
    },
    defaultColor: false,
  });

  return (
    <div className="code-block">
      {filename && (
        <div className="code-block__chrome">
          <span>{filename}</span>
          <span className="opacity-50">{lang}</span>
        </div>
      )}
      <CodeBlockCopy code={trimmed} />
      <div dangerouslySetInnerHTML={{ __html: html }} />
    </div>
  );
}
