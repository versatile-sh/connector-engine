import { useT } from "../i18n/I18nProvider";
import { Field } from "../ui/Field";
import { secretProblem } from "./backup";

type SecretPairProps = { label: string; againLabel: string; hint: string; min: number; value: string; again: string; checked: boolean; onChange(value: string, again: string): void };

/** A new secret typed twice: once `checked`, the first field says when it is too short and the second when the two differ. */
export function SecretPair({ label, againLabel, hint, min, value, again, checked, onChange }: SecretPairProps) {
  const t = useT();
  const problem = checked ? secretProblem(value, again, min) : undefined;
  return (
    <>
      <Field label={label} hint={hint} error={problem === "account.tooShort" ? t(problem, { min }) : undefined} type="password" autoComplete="new-password" minLength={min} required value={value} onChange={(event) => onChange(event.target.value, again)} />
      <Field label={againLabel} error={problem === "account.mismatch" ? t(problem) : undefined} type="password" autoComplete="new-password" required value={again} onChange={(event) => onChange(value, event.target.value)} />
    </>
  );
}
