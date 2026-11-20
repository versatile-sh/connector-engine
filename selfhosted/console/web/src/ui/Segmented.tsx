import { TOUCH_ROW } from "./styles";

type SegmentedProps<T extends string> = {
  label: string;
  options: readonly { value: T; text: string; hint?: string; lang?: string }[];
  value: T | undefined;
  onChange(value: T): void;
  disabled?: boolean;
  size?: "sm" | "md";
  className?: string;
};

const SIZE = { sm: { frame: "h-8", button: "px-2.5 text-xs" }, md: { frame: "h-9", button: "px-4 text-sm" } } as const;

/** A choice between a few things, in the chip language: one hairline pill whose chosen part is paper at 10%, the mark gliding to it; with no value, as when the things it sets differ, nothing is marked. */
export function Segmented<T extends string>({ label, options, value, onChange, disabled = false, size = "md", className = "" }: SegmentedProps<T>) {
  const index = options.findIndex((option) => option.value === value);
  const at = Math.max(index, 0);
  return (
    <div role="group" aria-label={label} className={`relative grid shrink-0 auto-cols-fr grid-flow-col rounded-full border border-line-strong p-0.5 transition-opacity duration-250 ${SIZE[size].frame} ${disabled ? "opacity-50" : ""} ${className}`}>
      <span
        aria-hidden="true"
        style={{ width: `calc((100% - 0.25rem) / ${options.length})`, translate: `${at * 100}% 0` }}
        className={`absolute inset-y-0.5 left-0.5 rounded-full bg-paper/10 transition-[translate,opacity] duration-250 ease-(--ease-glide) ${index < 0 ? "opacity-0" : ""}`}
      />
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          title={option.hint}
          lang={option.lang}
          aria-pressed={value === option.value}
          disabled={disabled}
          onClick={() => option.value !== value && onChange(option.value)}
          className={`relative z-10 h-full rounded-full font-medium whitespace-nowrap text-muted transition-colors duration-250 hover:text-paper aria-pressed:text-paper ${TOUCH_ROW} ${SIZE[size].button}`}
        >
          {option.text}
        </button>
      ))}
    </div>
  );
}
