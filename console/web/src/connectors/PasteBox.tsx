import { useState, type FormEvent } from "react";
import { useT } from "../i18n/I18nProvider";
import { readSource } from "../import/source";
import { Field } from "../ui/Field";
import { button } from "../ui/styles";

/** A field to paste an MCP URL or an install link, handed on once it reads as one. */
export function PasteBox({ onPaste, className = "" }: { onPaste(text: string): void; className?: string }) {
  const t = useT();
  const [text, setText] = useState("");
  const [invalid, setInvalid] = useState(false);
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!readSource(text, "mcp", { local: false, headerSecret: false }).ok) {
      setInvalid(true);
      return;
    }
    onPaste(text.trim());
  };
  return (
    <form onSubmit={onSubmit} noValidate className={`flex items-start gap-2 ${className}`}>
      <Field
        label={t("paste.label")}
        error={invalid ? t("import.invalid") : undefined}
        className="min-w-0 flex-1"
        mono
        value={text}
        placeholder="https://mcp.example.com/mcp"
        autoComplete="off"
        autoCapitalize="none"
        spellCheck={false}
        onChange={(event) => {
          setText(event.target.value);
          setInvalid(false);
        }}
      />
      <button type="submit" className={`${button({ variant: "secondary" })} mt-6.5`}>
        {t("paste.submit")}
      </button>
    </form>
  );
}
