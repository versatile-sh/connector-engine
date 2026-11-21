import { useState } from "react";
import type { Connector } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Checkbox } from "../ui/Checkbox";
import { LongList } from "../ui/LongList";
import { TITLE, WELL } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { AccessPicker } from "./AccessPicker";
import { ChoiceRow } from "./ChoiceRow";
import { FunctionTools } from "./FunctionTools";
import { AsksOnce, KindLabel } from "./KindLabel";
import type { Lens } from "./lens";
import { toggledOne } from "./selection";
import { accessCounts, asksOnce } from "./summary";

const TONE = { on: "paper", ask: "lime", off: "muted" } as const;

type FunctionsProps = { connector: Connector; lens: Lens; scope: string; copy?: { text: string; open(): void } | undefined };

const NONE: ReadonlySet<string> = new Set();

/** A connector's functions in one card, its heading, counts, filter and tools inside, with each one's choice as the lens of a profile reads and writes it; while they can change, each row can be picked for the tools over the list, and the rows picked belong to the profile `scope` they were picked in. */
export function Functions({ connector, lens, scope, copy }: FunctionsProps) {
  const t = useT();
  const [picked, setPicked] = useState<{ scope: string; names: ReadonlySet<string> }>({ scope, names: NONE });
  const selected = picked.scope === scope ? picked.names : NONE;
  const setSelected = (names: ReadonlySet<string>) => setPicked({ scope, names });
  const functions = connector.functions ?? [];
  const counts = accessCounts(functions.map((fn) => ({ access: lens.accessOf(fn.name, fn.access) })));
  return (
    <section aria-labelledby="functions-title" className={`${WELL} min-w-0`}>
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1 px-4 pt-4 sm:px-5">
        <h2 id="functions-title" className={TITLE}>
          {t("detail.functions")}
        </h2>
        <span className="flex flex-wrap gap-x-3">
          <Tag>{functions.length}</Tag>
          {(["on", "ask", "off"] as const).map((access) =>
            counts[access] > 0 ? (
              <Tag key={access} tone={TONE[access]}>
                {t(`access.count.${access}`, { count: counts[access] })}
              </Tag>
            ) : null,
          )}
        </span>
      </div>
      {functions.length === 0 ? (
        <p className="px-4 py-6 text-sm text-muted sm:px-5">{t("detail.none")}</p>
      ) : (
        <div className="px-4 pb-3 sm:px-5">
          <LongList rows={functions} textOf={(fn) => `${fn.name} ${fn.title ?? ""} ${fn.description ?? ""}`} keyOf={(fn) => fn.name} label={t("detail.filter")}>
            {(shown, { matching, filtered }) => (
              <>
                <FunctionTools connector={connector} lens={lens} functions={functions} matching={matching} filtered={filtered} selected={selected} onSelect={setSelected} copy={copy} />
                <ul className="-mx-4 mt-3 border-t border-line sm:-mx-5">
                  {shown.map(({ row: fn, phase }) => (
                    <ChoiceRow
                      key={fn.name}
                      phase={phase}
                      name={fn.name}
                      title={fn.title}
                      description={fn.description}
                      lead={lens.many ? <Checkbox label={t("bulk.pick", { name: fn.name })} checked={selected.has(fn.name)} onChange={() => setSelected(toggledOne(selected, fn.name))} /> : undefined}
                      flags={
                        <>
                          <KindLabel kind={fn.kind} />
                          {asksOnce(fn, lens.accessOf(fn.name, fn.access)) ? <AsksOnce /> : null}
                          {fn.state !== "active" ? (
                            <Tag tone="amber" dot title={fn.state === "quarantined" ? t("detail.quarantined") : undefined}>
                              {fn.state}
                            </Tag>
                          ) : null}
                          {lens.flags(fn.name, fn.access)}
                        </>
                      }
                    >
                      <AccessPicker name={fn.name} value={lens.accessOf(fn.name, fn.access)} most={lens.most(fn.access)} disabled={lens.locked(fn.name)} onChange={(access) => lens.choose(fn.name, access)} />
                    </ChoiceRow>
                  ))}
                </ul>
              </>
            )}
          </LongList>
        </div>
      )}
    </section>
  );
}
