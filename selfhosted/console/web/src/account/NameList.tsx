import { Tag } from "../ui/Tag";
import { SUBTITLE } from "../ui/styles";

type NameListProps = { title: string; rows: readonly { name: string; note?: string | undefined }[] };

/** A titled list of names, each with an optional note beside it as a tag; nothing when it is empty. */
export function NameList({ title, rows }: NameListProps) {
  if (rows.length === 0) return null;
  return (
    <section className="mt-5">
      <h3 className={SUBTITLE}>{title}</h3>
      <ul className="mt-2 flex flex-col gap-1.5 text-sm">
        {rows.map((row) => (
          <li key={row.name} className="flex flex-wrap items-baseline gap-x-2">
            <span className="min-w-0 font-mono [overflow-wrap:anywhere]">{row.name}</span>
            {row.note ? <Tag>{row.note}</Tag> : null}
          </li>
        ))}
      </ul>
    </section>
  );
}
