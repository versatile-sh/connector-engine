import type { Preview } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { useT } from "../i18n/I18nProvider";
import { Display } from "../ui/Display";
import { CAPTION } from "../ui/styles";

/** Who a connector says it is before it is installed: icon, title, publisher, description, and where it comes from. As the `h1` of the install-link page its title is the page's serif headline; in the import window it is a sans heading. */
export function Identity({ preview, as = "h3" }: { preview: Preview; as?: "h1" | "h3" }) {
  const t = useT();
  const page = as === "h1";
  const facts: { label: string; value: string }[] = [
    { label: t("import.from"), value: preview.url },
    ...(preview.remote_url && preview.remote_url !== preview.url ? [{ label: t("import.server"), value: preview.remote_url }] : []),
    ...(preview.auth && preview.auth.length > 0 ? [{ label: t("common.credentials"), value: preview.auth.join(", ") }] : []),
    ...(preview.protocol ? [{ label: t("import.protocol"), value: `MCP ${preview.protocol}` }] : []),
  ];
  return (
    <div className="min-w-0">
      <div className={`flex items-center ${page ? "gap-5" : "gap-4"}`}>
        <ConnectorIcon src={preview.icon} title={preview.title} size={page ? "lg" : "md"} />
        <div className="min-w-0">
          {page ? <Display className="[overflow-wrap:anywhere]">{preview.title}</Display> : <h3 className="text-xl leading-snug font-medium [overflow-wrap:anywhere]">{preview.title}</h3>}
          <p className={`${CAPTION} ${page ? "mt-3" : "mt-0.5"} truncate`} translate="no">
            {preview.name}@{preview.version}
            {preview.publisher ? ` · ${t("import.by", { publisher: preview.publisher })}` : ""}
          </p>
        </div>
      </div>
      {preview.description ? <p title={preview.description} className="mt-5 line-clamp-4 max-w-xl text-pretty text-muted">{preview.description}</p> : null}
      <dl className="mt-5 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-sm text-muted">
        {facts.map((fact) => (
          <div key={fact.label} className="contents">
            <dt>{fact.label}</dt>
            <dd className="min-w-0 truncate font-mono text-xs leading-5 text-paper" title={fact.value} translate="no">
              {fact.value}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
