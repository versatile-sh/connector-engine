import { useState, type ReactNode } from "react";
import { useT } from "../i18n/I18nProvider";
import type { Kept } from "./kept";
import { LONG_LIST_FROM, shownOf } from "./longList";
import { SearchBox } from "./SearchBox";
import { button } from "./styles";
import { Toolbar } from "./Toolbar";
import { useKept } from "./useKept";

type LongListProps<T> = { rows: readonly T[]; textOf(row: T): string; keyOf(row: T): string; label: string; whole?: boolean; bar?: boolean; toolbar?: ReactNode; children(shown: readonly Kept<T>[], found: { matching: readonly T[]; filtered: boolean }): ReactNode };

/** A list that stays short: past a few rows it shows the first ones, a filter over all of them and Show all; a `whole` list, where every row must be seen, keeps all of them and only the filter. With `bar`, the filter stands in a `Toolbar` row of its own, after the `toolbar` controls given, and the row is drawn as soon as there is a control or a filter to show, so a page puts the list straight under its header. `children` renders the rows shown, each with its phase so a row the filter or Show all brings in enters and one it hides leaves (`rowMotion`), told every row the filter matches and whether one is typed. */
export function LongList<T>({ rows, textOf, keyOf, label, whole = false, bar = false, toolbar, children }: LongListProps<T>) {
  const t = useT();
  const [filter, setFilter] = useState("");
  const [all, setAll] = useState(whole);
  const { shown, hidden } = shownOf(rows, textOf, { filter, all });
  const drawn = useKept(shown, keyOf);
  const search =
    rows.length > LONG_LIST_FROM ? <SearchBox label={label} value={filter} onChange={(event) => setFilter(event.target.value)} className={bar ? "swap-in min-w-0 flex-1 basis-48 sm:max-w-sm" : "swap-in my-2 w-full max-w-72"} /> : null;
  return (
    <>
      {bar ? toolbar || search ? <Toolbar>{toolbar}{search}</Toolbar> : null : search}
      {children(drawn, { matching: filter.trim() ? shown : rows, filtered: filter.trim() !== "" })}
      {shown.length === 0 ? <p className="py-3 text-sm text-muted">{t("list.noMatch")}</p> : null}
      {hidden > 0 ? (
        <button type="button" className={`${button({ variant: "quiet", size: "xs" })} mt-2 self-start`} onClick={() => setAll(true)}>
          {t("list.showAll", { count: rows.length })}
        </button>
      ) : null}
    </>
  );
}
