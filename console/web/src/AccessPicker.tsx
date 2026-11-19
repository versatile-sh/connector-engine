import { ACCESS, type Access } from "./api";

type AccessPickerProps = {
  label: string;
  value: Access;
  disabled?: boolean;
  onChange: (next: Access) => void;
};

const HINT: Record<Access, string> = {
  off: "the agent never sees it",
  read: "runs at once, counted as a read",
  write: "runs at once, counted as a write",
  ask: "waits for you on every call",
};

/** Four-way segmented choice for one function. */
export function AccessPicker({ label, value, disabled, onChange }: AccessPickerProps) {
  const index = ACCESS.indexOf(value);
  return (
    <div className="segmented" role="radiogroup" aria-label={label} data-value={value}>
      <span className="segmented-thumb" style={{ transform: `translateX(${index * 100}%)` }} aria-hidden="true" />
      {ACCESS.map((option) => (
        <button
          key={option}
          type="button"
          role="radio"
          aria-checked={option === value}
          title={HINT[option]}
          disabled={disabled}
          className={option === value ? "on" : undefined}
          onClick={() => option !== value && onChange(option)}
        >
          {option}
        </button>
      ))}
    </div>
  );
}
