import { Segmented } from "./Segmented";
import { Select } from "./Select";

type ChoiceProps<T extends string> = {
  label: string;
  value: T | undefined;
  options: readonly { value: T; text: string; hint?: string; disabled?: boolean }[];
  onChange(value: T): void;
  disabled?: boolean;
  room?: keyof typeof ROOM;
};

const ROOM = {
  "22rem": ["hidden @min-[22rem]:block", "@min-[22rem]:hidden"],
  "36rem": ["hidden max-w-full overflow-x-auto @min-[36rem]:block", "@min-[36rem]:hidden"],
} as const;

/** One of a few, fitted to its nearest container: a small `Segmented` where that container has `room`, else a `Select` of the options that can be taken, so a row never stacks or overflows. */
export function Choice<T extends string>({ label, value, options, onChange, disabled = false, room = "22rem" }: ChoiceProps<T>) {
  const [wide, narrow] = ROOM[room];
  return (
    <>
      <span className={wide}>
        <Segmented<T> label={label} size="sm" value={value} disabled={disabled} options={options} onChange={onChange} />
      </span>
      <span className={narrow}>
        <Select<T> label={label} value={value} disabled={disabled} options={options.filter((option) => !option.disabled)} onChange={onChange} />
      </span>
    </>
  );
}
