import { TOUCH_ROW } from "./styles";

type SegmentedProps<T extends string> = {
  label: string;
  options: readonly { value: T; text: string; hint?: string }[];
  value: T | undefined;
  onChange(value: T): void;
  disabled?: boolean;
  size?: "sm" | "md";
  className?: string;
};

const SIZE = { sm: { frame: "h-8", button: "px-2.5 text-xs" }, md: { frame: "h-10", button: "px-4 text-sm" } } as const;

/** A choice between a few things as a bubble whose thumb slides to the chosen one; with no value, as when the things it sets differ, the thumb hides. */
export function Segmented<T extends string>({ label, options, value, onChange, disabled = false, size = "md", className = "" }: SegmentedProps<T>) {
  const index = options.findIndex((option) => option.value === value);
  const at = Math.max(index, 0);
  return (
    <div role="group" aria-label={label} className={`bubble-word relative grid shrink-0 auto-cols-fr grid-flow-col border border-line-strong p-1 transition-opacity duration-300 ${SIZE[size].frame} ${disabled ? "opacity-60" : ""} ${className}`}>
      <span
        aria-hidden="true"
        style={{ width: `calc((100% - 0.5rem) / ${options.length})`, translate: `${at * 100}% 0` }}
        className={`absolute inset-y-1 left-1 bg-paper [border-radius:inherit] transition-[translate,opacity] duration-500 ease-(--ease-glide) ${index < 0 ? "opacity-0" : ""}`}
      />
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          title={option.hint}
          aria-pressed={value === option.value}
          disabled={disabled}
          onClick={() => option.value !== value && onChange(option.value)}
          className={`relative z-10 h-full [border-radius:inherit] font-semibold whitespace-nowrap text-muted transition-colors duration-300 hover:text-paper aria-pressed:text-ink ${TOUCH_ROW} ${SIZE[size].button}`}
        >
          {option.text}
        </button>
      ))}
    </div>
  );
}
