import type { Access, ProbedTool } from "../api";
import { AccessPicker } from "../connectors/AccessPicker";
import { ChoiceRow } from "../connectors/ChoiceRow";
import { KindLabel } from "../connectors/KindLabel";
import { useT } from "../i18n/I18nProvider";
import { LongList } from "../ui/LongList";
import { SKELETON_LINE, WELL } from "../ui/styles";

/** The proposed access of every function a server lists, each one changeable before the install. */
export function Choices({ tools, accessOf, choose }: { tools: ProbedTool[]; accessOf(name: string, proposed: Access): Access; choose(name: string, access: Access): void }) {
  const t = useT();
  if (tools.length === 0) return <p className="py-6 text-sm text-muted">{t("import.noTools")}</p>;
  return (
    <LongList rows={tools} textOf={(tool) => `${tool.name} ${tool.title ?? ""} ${tool.description ?? ""}`} keyOf={(tool) => tool.name} label={t("detail.filter")} whole>
      {(shown) => (
        <ul className={WELL}>
          {shown.map(({ row: tool, phase }) => (
            <ChoiceRow key={tool.name} phase={phase} name={tool.name} title={tool.title} description={tool.description} flags={<KindLabel kind={tool.kind} estimated={tool.estimated} />}>
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
    <ul aria-hidden="true" className={`${WELL} @container`}>
      {Array.from({ length: rows }, (_, row) => (
        <li key={row} className="flex items-center gap-6 border-b border-line px-4 py-4 last:border-b-0 sm:px-5">
          <div className="flex-1">
            <div className={`${SKELETON_LINE} h-4 w-40`} />
            <div className={`${SKELETON_LINE} mt-2 h-3 w-64 max-w-full`} />
          </div>
          <div className={`${SKELETON_LINE} hidden h-8 w-44 @lg:block`} />
        </li>
      ))}
    </ul>
  );
}
