import type { Connector, FunctionView } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Checkbox } from "../ui/Checkbox";
import { Select } from "../ui/Select";
import { WELL, button } from "../ui/styles";
import { AccessPicker } from "./AccessPicker";
import type { Lens } from "./lens";
import { PRESETS, pickedOf, presetNames, toggledAll, widestOf } from "./selection";

type More = "read" | "write" | "destructive" | "copy";

type ToolsProps = {
  connector: Connector;
  lens: Lens;
  functions: readonly FunctionView[];
  matching: readonly FunctionView[];
  filtered: boolean;
  selected: ReadonlySet<string>;
  onSelect(next: ReadonlySet<string>): void;
  copy?: { text: string; open(): void } | undefined;
};

/** The tools over a connector's function list while its rows can change: a select-all over the rows the filter shows and one menu of class presets and the profile copy; with rows picked, one bar sets them all to off, on or ask, or back to the default for a key, in one request, and says what it refused. */
export function FunctionTools({ connector, lens, functions, matching, filtered, selected, onSelect, copy }: ToolsProps) {
  const t = useT();
  const many = lens.many;
  if (!many && !copy) return null;
  const names = matching.map((fn) => fn.name);
  const picked = pickedOf(selected, names);
  const chosen = functions.filter((fn) => selected.has(fn.name));
  const widest = widestOf(chosen.map((fn) => fn.access));
  const more = [
    ...(many ? PRESETS.map((preset) => ({ value: preset.kind, text: t(`bulk.preset.${preset.kind}`) })) : []),
    ...(copy ? [{ value: "copy" as const, text: copy.text }] : []),
  ];
  const run = (action: More) => {
    const preset = PRESETS.find((p) => p.kind === action);
    if (preset) many?.set(presetNames(functions, preset), preset.access);
    else copy?.open();
  };
  return (
    <div className="mt-3 flex flex-col gap-2">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        {many ? (
          <Checkbox
            label={filtered ? t("bulk.selectShown", { count: names.length }) : t("bulk.selectAll")}
            visibleLabel
            checked={picked === "all"}
            mixed={picked === "some"}
            disabled={names.length === 0}
            onChange={() => onSelect(toggledAll(selected, names))}
          />
        ) : (
          <span />
        )}
        <Select<More> label={t("bulk.moreLabel", { connector: connector.title })} placeholder={t("bulk.more")} value={undefined} options={more} disabled={many?.busy ?? false} onChange={run} className="max-w-56" />
      </div>
      {many && selected.size > 0 ? (
        <div className={`${WELL} swap-in @container flex flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 py-2.5`}>
          <span className="text-sm font-medium">{t("bulk.selected", { count: selected.size })}</span>
          <div className="flex flex-wrap items-center gap-2">
            <AccessPicker name={t("bulk.selected", { count: selected.size })} value={undefined} most={lens.most(widest ?? "on")} disabled={many.busy} onChange={(access) => many.set([...selected], access)} />
            {many.resets && chosen.some((fn) => many.overridden(fn.name)) ? (
              <button type="button" disabled={many.busy} className={button({ variant: "secondary", size: "xs" })} onClick={() => many.set([...selected], "default")}>
                {t("bulk.reset")}
              </button>
            ) : null}
            <button type="button" className={button({ variant: "quiet", size: "xs" })} onClick={() => onSelect(new Set())}>
              {t("bulk.clear")}
            </button>
          </div>
        </div>
      ) : null}
      <p role="alert" className="text-sm text-amber-400 empty:hidden">
        {many && many.refused.length > 0 ? t("bulk.refused", { names: many.refused.map((result) => result.name).join(", ") }) : ""}
      </p>
    </div>
  );
}
