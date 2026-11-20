import type { ComponentProps } from "react";
import { Icon } from "./Icon";
import { PILL } from "./styles";

type SearchBoxProps = Omit<ComponentProps<"input">, "type" | "aria-label" | "placeholder" | "className"> & { label: string; className?: string };

/** A small search pill: the glass, then the input, named by its label, which also stands as its placeholder. */
export function SearchBox({ label, className = "", ...input }: SearchBoxProps) {
  return (
    <span className={`${PILL} flex items-center gap-2 focus-within:border-paper/80 ${className}`}>
      <Icon name="search" className="size-3.5 shrink-0 text-muted" />
      <input type="search" aria-label={label} placeholder={label} autoComplete="off" spellCheck={false} className="h-full min-w-0 flex-1 bg-transparent placeholder:text-muted-light focus-visible:outline-none" {...input} />
    </span>
  );
}
