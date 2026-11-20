import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { changePassword, renameOwner } from "../mutations";
import { Section } from "../notifications/Section";
import { Field } from "../ui/Field";
import { Notice } from "../ui/Notice";
import { button } from "../ui/styles";
import { PASSWORD_MIN, passwordRefusal, secretProblem } from "./backup";
import { SecretPair } from "./SecretPair";

function UsernameForm({ username }: { username: string }) {
  const t = useT();
  const [name, setName] = useState(username);
  const rename = useMutation(renameOwner);
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    rename.mutate(name.trim());
  };
  return (
    <form onSubmit={onSubmit} aria-label={t("account.username")}>
      <Field
        label={t("account.username")}
        hint={rename.isSuccess ? t("account.renamed") : t("account.usernameHint")}
        error={rename.isError ? errorText(rename.error) : undefined}
        autoComplete="username"
        maxLength={64}
        required
        value={name}
        onChange={(event) => {
          setName(event.target.value);
          rename.reset();
        }}
      />
      <button type="submit" disabled={rename.isPending || name.trim() === username} className={button({ variant: "secondary", size: "sm" })}>
        {rename.isPending ? t("account.saving") : t("account.rename")}
      </button>
    </form>
  );
}

function PasswordForm() {
  const t = useT();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState({ value: "", again: "" });
  const [checked, setChecked] = useState(false);
  const change = useMutation(changePassword);
  const refusal = change.isError ? passwordRefusal(change.error) : undefined;
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    setChecked(true);
    if (secretProblem(next.value, next.again, PASSWORD_MIN)) return;
    change.mutate(
      { current, password: next.value },
      {
        onSuccess: () => {
          setCurrent("");
          setNext({ value: "", again: "" });
          setChecked(false);
        },
      },
    );
  };
  return (
    <form onSubmit={onSubmit} aria-label={t("account.changePassword")}>
      <Field label={t("account.current")} type="password" autoComplete="current-password" required value={current} error={refusal ? t(refusal) : undefined} onChange={(event) => setCurrent(event.target.value)} />
      <SecretPair
        label={t("account.newPassword")}
        againLabel={t("account.againPassword")}
        hint={t("account.minHint", { min: PASSWORD_MIN })}
        min={PASSWORD_MIN}
        value={next.value}
        again={next.again}
        checked={checked}
        onChange={(value, again) => setNext({ value, again })}
      />
      <p role="alert" className="mb-2 text-sm text-amber-400 empty:hidden">
        {change.isError && !refusal ? errorText(change.error) : ""}
      </p>
      {change.isSuccess ? (
        <div className="mb-3">
          <Notice tone="lime">{t("account.changed")}</Notice>
        </div>
      ) : null}
      <button type="submit" disabled={change.isPending} className={button({ variant: "secondary", size: "sm" })}>
        {change.isPending ? t("account.saving") : t("account.changePassword")}
      </button>
    </form>
  );
}

/** How the owner signs in: the name, changed in place, and the password, changed with the current one; changing it signs every other session out. */
export function SignInSection({ username }: { username: string }) {
  const t = useT();
  return (
    <Section id="sign-in-title" icon="key" title={t("account.signIn.title")} intro={t("account.signIn.intro")}>
      <div className="grid gap-8 @3xl:grid-cols-2">
        <UsernameForm username={username} />
        <PasswordForm />
      </div>
    </Section>
  );
}
