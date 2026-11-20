import { useState, type ReactNode } from "react";
import { useT } from "../i18n/I18nProvider";
import { LONG_LIST_FROM, shownOf } from "./longList";
import { SearchBox } from "./SearchBox";
import { button } from "./styles";

type LongListProps<T> = { rows: readonly T[]; textOf(row: T): string; label: string; whole?: boolean; children(shown: readonly T[], found: { matching: readonly T[]; filtered: boolean }): ReactNode };

/** A list that stays short: past a few rows it shows the first ones, a filter over all of them and Show all; a `whole` list, where every row must be seen, keeps all of them and only the filter. `children` renders the rows shown, told every row the filter matches and whether one is typed. */
export function LongList<T>({ rows, textOf, label, whole = false, children }: LongListProps<T>) {
  const t = useT();
  const [filter, setFilter] = useState("");
  const [all, setAll] = useState(whole);
  const { shown, hidden } = shownOf(rows, textOf, { filter, all });
  return (
    <>
      {rows.length > LONG_LIST_FROM ? <SearchBox label={label} value={filter} onChange={(event) => setFilter(event.target.value)} className="swap-in my-2 w-full max-w-72" /> : null}
      {children(shown, { matching: filter.trim() ? shown : rows, filtered: filter.trim() !== "" })}
      {shown.length === 0 ? <p className="py-3 text-sm text-muted">{t("list.noMatch")}</p> : null}
      {hidden > 0 ? (
        <button type="button" className={`${button({ variant: "quiet", size: "xs" })} mt-2`} onClick={() => setAll(true)}>
          {t("list.showAll", { count: rows.length })}
        </button>
      ) : null}
    </>
  );
}
