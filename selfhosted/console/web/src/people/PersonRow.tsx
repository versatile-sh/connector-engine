import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { errorCode, errorText, type Person } from "../api";
import { useLocale, useT } from "../i18n/I18nProvider";
import { removePerson, setRole } from "../mutations";
import { InlineConfirm } from "../ui/InlineConfirm";
import { Select } from "../ui/Select";
import { CAPTION, button } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { actionValue, rowActionsOf } from "./people";
import type { Viewer } from "./viewer";

type PersonRowProps = { person: Person; viewer: Viewer; onReset(person: Person): void };

const refusal = (error: unknown, lastOwner: string) => (errorCode(error) === "last_owner" ? lastOwner : errorText(error));

/** One person: their name, "you" on the viewer's own row, their role, keys and when they joined, and the one thing the viewer may do to the row: an overflow menu of the other roles, Reset password and Remove for an Owner, the Reset password button alone for an Admin; Remove asks first, naming what it does. */
export function PersonRow({ person, viewer, onReset }: PersonRowProps) {
  const t = useT();
  const locale = useLocale();
  const role = useMutation(setRole);
  const remove = useMutation(removePerson);
  const [asking, setAsking] = useState(false);
  const actions = rowActionsOf(viewer, person);
  const lastOwner = t("people.lastOwner");
  const options = actions.map((action) => ({
    value: actionValue(action),
    text: action.kind === "role" ? t("people.makeRole", { role: t(`role.${action.role}`) }) : action.kind === "reset" ? t("people.reset") : t("common.remove"),
  }));
  const choose = (value: string) => {
    const action = actions.find((entry) => actionValue(entry) === value);
    if (!action) return;
    if (action.kind === "role") role.mutate({ person: person.person, role: action.role });
    else if (action.kind === "reset") onReset(person);
    else {
      remove.reset();
      setAsking(true);
    }
  };
  return (
    <li className={`border-b border-line px-4 py-3 transition-opacity duration-250 last:border-b-0 sm:px-5 ${role.isPending || remove.isPending ? "opacity-60" : ""}`}>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
        <span className="min-w-0 grow basis-40 truncate font-medium" title={person.username} translate="no">
          {person.username}
        </span>
        {person.you ? <Tag tone="paper">{t("people.you")}</Tag> : null}
        <Tag>{t(`role.${person.role}`)}</Tag>
        <span className={CAPTION}>{t("people.keys", { count: person.keys })}</span>
        <span className={CAPTION}>{t("people.joined", { date: new Date(person.created_at).toLocaleDateString(locale, { dateStyle: "medium" }) })}</span>
        {viewer.can("people.manage") && options.length > 0 && !asking ? (
          <Select<string> label={t("people.actionsFor", { name: person.username })} value={undefined} placeholder={t("people.manage")} options={options} onChange={choose} disabled={role.isPending} className="w-44" />
        ) : actions.some((action) => action.kind === "reset") ? (
          <button type="button" className={button({ variant: "secondary", size: "xs" })} onClick={() => onReset(person)}>
            {t("people.reset")}
          </button>
        ) : null}
      </div>
      <p role="alert" className="mt-1 text-sm text-amber-400 empty:hidden">
        {role.isError ? refusal(role.error, lastOwner) : ""}
      </p>
      {asking ? (
        <InlineConfirm
          label={t("people.removeLabel", { name: person.username })}
          question={t("people.removeAsk", { name: person.username, count: person.keys })}
          confirm={t("people.removeConfirm")}
          working={t("people.removing")}
          busy={remove.isPending}
          error={remove.isError ? refusal(remove.error, lastOwner) : undefined}
          onCancel={() => setAsking(false)}
          onConfirm={() => remove.mutate(person.person, { onSuccess: () => setAsking(false) })}
        />
      ) : null}
    </li>
  );
}
