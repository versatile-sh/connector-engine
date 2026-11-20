import { CAPTION } from "../ui/styles";

type NameListProps = { title: string; rows: readonly { name: string; note?: string | undefined }[] };

/** A titled list of names, each with an optional note beside it; nothing when it is empty. */
export function NameList({ title, rows }: NameListProps) {
  if (rows.length === 0) return null;
  return (
    <section className="mt-5">
      <h3 className="text-sm font-medium">{title}</h3>
      <ul className="mt-2 flex flex-col gap-1.5 text-sm">
        {rows.map((row) => (
          <li key={row.name} className="flex flex-wrap items-baseline gap-x-2 [overflow-wrap:anywhere]">
            <span className="font-mono">{row.name}</span>
            {row.note ? <span className={CAPTION}>{row.note}</span> : null}
          </li>
        ))}
      </ul>
    </section>
  );
}
