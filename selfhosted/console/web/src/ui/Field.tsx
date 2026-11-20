import { useId, type ComponentProps, type ReactNode } from "react";
import { FIELD } from "./styles";

type FieldProps = Omit<ComponentProps<"input">, "id" | "aria-invalid" | "aria-describedby"> & {
  label: string;
  hint?: ReactNode;
  error?: string | undefined;
  mono?: boolean;
  className?: string;
};

const BOX = `${FIELD} mt-1.5 flex h-11 items-center`;
const INPUT = "h-full min-w-0 flex-1 bg-transparent px-3.5 text-paper placeholder:text-muted-light focus-visible:outline-none";

/** The one text field: a label, the input, and a note below that keeps its height whether it holds the hint or the error. */
export function Field({ label, hint, error, mono = false, className = "", ...input }: FieldProps) {
  const id = useId();
  return (
    <div className={className}>
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      <div className={BOX}>
        <input id={id} aria-invalid={error ? true : undefined} aria-describedby={`${id}-note`} className={`${INPUT} ${mono ? "font-mono text-sm" : ""}`} {...input} />
      </div>
      <p id={`${id}-note`} role={error ? "alert" : undefined} className={`mt-1.5 min-h-5 text-sm ${error ? "text-amber-400" : "text-muted"}`}>
        {error ?? hint}
      </p>
    </div>
  );
}
