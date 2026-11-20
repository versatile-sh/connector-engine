import type { Preview } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { useT } from "../i18n/I18nProvider";

/** Who a connector says it is before it is installed: icon, title, publisher, description, and where it comes from. On lime it is drawn in ink, large. */
export function Identity({ preview, onLime = false }: { preview: Preview; onLime?: boolean }) {
  const t = useT();
  const soft = onLime ? "text-ink/70" : "text-muted";
  const facts: { label: string; value: string }[] = [
    { label: t("import.from"), value: preview.url },
    ...(preview.remote_url && preview.remote_url !== preview.url ? [{ label: t("import.server"), value: preview.remote_url }] : []),
    ...(preview.auth && preview.auth.length > 0 ? [{ label: t("import.credentials"), value: preview.auth.join(", ") }] : []),
    ...(preview.protocol ? [{ label: t("import.protocol"), value: `MCP ${preview.protocol}` }] : []),
  ];
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-4">
        <ConnectorIcon src={preview.icon} title={preview.title} size={onLime ? "lg" : "md"} onLime={onLime} />
        <div className="min-w-0">
          <h2 className={`font-display font-bold tracking-[-0.04em] break-words ${onLime ? "text-4xl leading-[1.04] sm:text-5xl" : "text-2xl leading-tight"}`}>{preview.title}</h2>
          <p className={`mt-1 truncate font-mono text-xs ${soft}`} translate="no">
            {preview.name}@{preview.version}
            {preview.publisher ? ` · ${t("import.by", { publisher: preview.publisher })}` : ""}
          </p>
        </div>
      </div>
      {preview.description ? <p title={preview.description} className={`mt-4 line-clamp-4 max-w-xl ${onLime ? "text-ink/85" : "text-muted"}`}>{preview.description}</p> : null}
      <dl className={`mt-5 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm ${soft}`}>
        {facts.map((fact) => (
          <div key={fact.label} className="contents">
            <dt>{fact.label}</dt>
            <dd className={`min-w-0 truncate font-mono text-xs leading-5 ${onLime ? "text-ink" : "text-fg"}`} title={fact.value} translate="no">
              {fact.value}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
