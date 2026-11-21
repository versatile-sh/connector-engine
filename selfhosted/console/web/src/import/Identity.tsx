import type { Preview } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { useT } from "../i18n/I18nProvider";
import { CAPTION } from "../ui/styles";

/** The machine facts of a connector's preview, each as a label and a truncated value: where it comes from (unless `from` is false), the server, the credentials and the protocol. */
export function PreviewFacts({ preview, from = true }: { preview: Preview; from?: boolean }) {
  const t = useT();
  const facts: { label: string; value: string }[] = [
    ...(from ? [{ label: t("import.from"), value: preview.url }] : []),
    ...(preview.remote_url && preview.remote_url !== preview.url ? [{ label: t("import.server"), value: preview.remote_url }] : []),
    ...(preview.auth && preview.auth.length > 0 ? [{ label: t("common.credentials"), value: preview.auth.join(", ") }] : []),
    ...(preview.protocol ? [{ label: t("import.protocol"), value: `MCP ${preview.protocol}` }] : []),
  ];
  if (facts.length === 0) return null;
  return (
    <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-sm text-muted">
      {facts.map((fact) => (
        <div key={fact.label} className="contents">
          <dt>{fact.label}</dt>
          <dd className="min-w-0 truncate font-mono text-xs leading-5 text-paper" title={fact.value} translate="no">
            {fact.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}

/** Who a connector says it is before it is installed, in the import window: icon, sans title, publisher, description, and where it comes from. */
export function Identity({ preview }: { preview: Preview }) {
  const t = useT();
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-4">
        <ConnectorIcon src={preview.icon} title={preview.title} size="md" />
        <div className="min-w-0">
          <h3 className="text-xl leading-snug font-medium [overflow-wrap:anywhere]">{preview.title}</h3>
          <p className={`${CAPTION} mt-0.5 truncate`} translate="no">
            {preview.name}@{preview.version}
            {preview.publisher ? ` · ${t("import.by", { publisher: preview.publisher })}` : ""}
          </p>
        </div>
      </div>
      {preview.description ? <p title={preview.description} className="mt-5 line-clamp-4 max-w-xl text-pretty text-muted">{preview.description}</p> : null}
      <div className="mt-5">
        <PreviewFacts preview={preview} />
      </div>
    </div>
  );
}
