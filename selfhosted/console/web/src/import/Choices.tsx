import type { Access, ProbedTool } from "../api";
import { AccessPicker } from "../connectors/AccessPicker";
import { ChoiceRow } from "../connectors/ChoiceRow";
import { KindLabel } from "../connectors/KindLabel";
import { useT } from "../i18n/I18nProvider";
import { LongList } from "../ui/LongList";

/** The proposed access of every function a server lists, each one changeable before the install. */
export function Choices({ tools, accessOf, choose }: { tools: ProbedTool[]; accessOf(name: string, proposed: Access): Access; choose(name: string, access: Access): void }) {
  const t = useT();
  if (tools.length === 0) return <p className="py-6 text-sm text-muted">{t("import.noTools")}</p>;
  return (
    <LongList rows={tools} textOf={(tool) => `${tool.name} ${tool.title ?? ""} ${tool.description ?? ""}`} label={t("detail.filter")} whole>
      {(shown) => (
        <ul>
          {shown.map((tool) => (
            <ChoiceRow key={tool.name} name={tool.name} title={tool.title} description={tool.description} flags={<KindLabel kind={tool.kind} />}>
              <AccessPicker name={tool.name} value={accessOf(tool.name, tool.proposed)} onChange={(access) => choose(tool.name, access)} />
            </ChoiceRow>
          ))}
        </ul>
      )}
    </LongList>
  );
}

/** The shape of a preview while the server is read: an identity and a few function rows. */
export function ChoicesSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <ul aria-hidden="true" className="@container">
      {Array.from({ length: rows }, (_, row) => (
        <li key={row} className="flex items-center gap-6 border-b border-line py-4 last:border-b-0">
          <div className="flex-1">
            <div className="shimmer bubble-word h-4 w-40" />
            <div className="shimmer bubble-word mt-2 h-3 w-64 max-w-full" />
          </div>
          <div className="shimmer bubble-word hidden h-8 w-64 @lg:block" />
        </li>
      ))}
    </ul>
  );
}
