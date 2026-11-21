import { useMutation } from "@tanstack/react-query";
import { errorText, type SignupSettings } from "../api";
import { useT } from "../i18n/I18nProvider";
import { saveSignup } from "../mutations";
import { Select } from "../ui/Select";
import { Switch } from "../ui/Switch";
import { CAPTION, WELL } from "../ui/styles";

/** The Owner's sign-up choice on the People tab: a switch with its one-line explanation and, while on, the role new accounts get and a note to keep it off on a console the internet reaches. */
export function SignupSetting({ settings }: { settings: SignupSettings }) {
  const t = useT();
  const save = useMutation(saveSignup);
  const current = save.isPending ? save.variables : settings;
  return (
    <section aria-label={t("people.signup.title")} className={`${WELL} mt-6 flex flex-col gap-4 p-4 sm:p-5`}>
      <Switch checked={current.enabled} hint={t("people.signup.hint")} onChange={(enabled) => save.mutate({ ...current, enabled })}>
        {t("people.signup.title")}
      </Switch>
      {current.enabled ? (
        <>
          <Select<SignupSettings["role"]>
            label={t("people.signup.role")}
            value={current.role}
            options={(["member", "admin"] as const).map((role) => ({ value: role, text: t(`role.${role}`) }))}
            onChange={(role) => save.mutate({ ...current, role })}
            className="w-full sm:w-60"
          />
          <p className={CAPTION}>{t("people.signup.warn")}</p>
        </>
      ) : null}
      <p role="alert" className="text-sm text-amber-400 empty:hidden">
        {save.isError ? errorText(save.error) : ""}
      </p>
    </section>
  );
}
