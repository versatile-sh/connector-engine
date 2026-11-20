type CheckboxProps = { label: string; checked: boolean; mixed?: boolean; disabled?: boolean; visibleLabel?: boolean; onChange(checked: boolean): void };

/** A small check in paper for picking rows: its label read aloud or shown beside it, and `mixed` when only some of what it stands for is picked. */
export function Checkbox({ label, checked, mixed = false, disabled = false, visibleLabel = false, onChange }: CheckboxProps) {
  return (
    <label className="relative inline-flex shrink-0 cursor-pointer items-center gap-2 text-sm text-muted select-none has-disabled:cursor-not-allowed has-disabled:opacity-50 pointer-coarse:min-h-11">
      <input
        type="checkbox"
        ref={(input) => {
          if (input) input.indeterminate = mixed && !checked;
        }}
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
        className="size-4 cursor-[inherit] rounded accent-paper focus-visible:outline-1 focus-visible:outline-offset-4 focus-visible:outline-paper/50"
      />
      <span className={visibleLabel ? "" : "sr-only"}>{label}</span>
    </label>
  );
}
