import CopyButton from "@/components/CopyButton";

/** A mono block with a copy button. No highlighter: the only things dimmed
 *  are `#` comment lines and a leading `$ ` prompt, which is all a shell
 *  snippet needs to read. `copy` overrides what the button copies (default:
 *  the whole block minus prompts and comments). */
export default function CodeBlock({ code, copy, label }: { code: string; copy?: string; label?: string }) {
  const lines = code.replace(/\n$/, "").split("\n");
  const toCopy =
    copy ??
    lines
      .filter((l) => !l.trimStart().startsWith("#"))
      .map((l) => l.replace(/^\$ /, ""))
      .join("\n");
  return (
    <div className="code">
      <pre>
        <code>
          {lines.map((line, i) => {
            const comment = line.trimStart().startsWith("#");
            const prompt = line.startsWith("$ ");
            return (
              <span key={i} className={comment ? "code-comment" : undefined}>
                {prompt ? (
                  <>
                    <span className="code-prompt" aria-hidden="true">
                      $
                    </span>{" "}
                    {line.slice(2)}
                  </>
                ) : (
                  line
                )}
                {"\n"}
              </span>
            );
          })}
        </code>
      </pre>
      <CopyButton text={toCopy} label={label ?? "Copy this snippet"} />
    </div>
  );
}
