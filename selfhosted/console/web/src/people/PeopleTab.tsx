import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { errorText, type PeopleData } from "../api";
import type { MessageKey } from "../i18n";
import { useLocale, useT } from "../i18n/I18nProvider";
import { useNow } from "../approve/useNow";
import { revokeLink } from "../mutations";
import { peopleQuery } from "../queries";
import { Disclosure } from "../ui/Disclosure";
import { Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { CAPTION, LEAD, SKELETON_LINE, SUBTITLE, SURFACE, TITLE, WELL, button } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { LinkSheet, type LinkFor } from "./LinkSheet";
import { nameOf, timeLeft } from "./people";
import { PersonRow } from "./PersonRow";
import { SignupSetting } from "./SignupSetting";
import { useViewer } from "./useViewer";

const AUDIT: Partial<Record<string, MessageKey>> = {
  "user.created": "audit.user.created",
  "function.class_raised": "audit.function.classRaised",
  "signup.changed": "audit.signup.changed",
  "invite.created": "audit.invite.created",
  "invite.revoked": "audit.invite.revoked",
  "reset.created": "audit.reset.created",
  "reset.used": "audit.reset.used",
  "role.changed": "audit.role.changed",
  "user.removed": "audit.user.removed",
  "username.changed": "audit.username.changed",
  "password.changed": "audit.password.changed",
  "password.reset": "audit.password.reset",
  "backup.exported": "audit.backup.exported",
  "backup.restored": "audit.backup.restored",
  "setup.imported": "audit.setup.imported",
  "slot.applied": "audit.slot.applied",
  "slot.discarded": "audit.slot.discarded",
  "connector.credentials_changed": "audit.connector.credentials_changed",
};

function Links({ data }: { data: PeopleData }) {
  const t = useT();
  const revoke = useMutation(revokeLink);
  const now = useNow(false);
  if (data.invites.length === 0) return null;
  return (
    <section aria-labelledby="links-title" className={`${WELL} mt-6`}>
      <div className="flex items-baseline gap-3 px-4 pt-4 sm:px-5">
        <h3 id="links-title" className={SUBTITLE}>
          {t("people.links")}
        </h3>
        <Tag>{data.invites.length}</Tag>
      </div>
      <ul className="mt-3 border-t border-line">
        {data.invites.map((link) => {
          const left = timeLeft(link.expires_at, now);
          const what = link.purpose === "join" ? t("people.linkJoin", { role: t(`role.${link.role}`) }) : t("people.linkReset", { name: nameOf(data.people, link.person) });
          return (
            <li key={link.id} className={`flex flex-wrap items-center gap-x-4 gap-y-1 border-b border-line px-4 py-3 last:border-b-0 sm:px-5 ${revoke.isPending && revoke.variables === link.id ? "opacity-60" : ""}`}>
              <span className="min-w-0 grow basis-40 text-sm">{what}</span>
              <span className={CAPTION}>{t(left.unit === "day" ? "people.leftDays" : "people.leftHours", { count: left.count })}</span>
              {link.may_revoke ? (
                <button type="button" disabled={revoke.isPending} className={button({ variant: "danger", size: "xs" })} aria-label={t("people.revokeLabel", { link: what })} onClick={() => revoke.mutate(link.id)}>
                  {t("common.revoke")}
                </button>
              ) : null}
            </li>
          );
        })}
      </ul>
      <p role="alert" className="px-4 pb-3 text-sm text-amber-400 empty:hidden sm:px-5">
        {revoke.isError ? errorText(revoke.error) : ""}
      </p>
    </section>
  );
}

function Changes({ data }: { data: PeopleData }) {
  const t = useT();
  const locale = useLocale();
  if (data.audit.length === 0) return null;
  return (
    <Disclosure
      className={`${WELL} mt-6`}
      summary={
        <>
          <span className="font-medium">{t("people.changes")}</span>
          <Tag>{data.audit.length}</Tag>
        </>
      }
    >
      <ol className="border-t border-line">
        {data.audit.map((entry) => {
          const key = entry.action === "user.created" && entry.actor === entry.target ? "audit.user.signedUp" : AUDIT[entry.action];
          return (
            <li key={`${entry.at}-${entry.action}-${entry.target}`} className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 border-b border-line px-4 py-2.5 text-sm last:border-b-0">
              <span className="min-w-0 grow basis-56">
                <span className="font-medium" translate="no">
                  {nameOf(data.people, entry.actor)}
                </span>{" "}
                <span className="text-muted">{key ? t(key, { target: nameOf(data.people, entry.target) }) : entry.action}</span>
              </span>
              <time dateTime={entry.at} className={CAPTION}>
                {new Date(entry.at).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" })}
              </time>
            </li>
          );
        })}
      </ol>
    </Disclosure>
  );
}

/** The people of this console in one card: the count and Invite someone for those who may invite, one row per person, the links still waiting, and the newest changes behind Recent changes. */
export function PeopleTab() {
  const t = useT();
  const viewer = useViewer();
  const people = useQuery(peopleQuery);
  const [making, setMaking] = useState<LinkFor>();
  const data = people.data;
  return (
    <section aria-labelledby="people-title" className={`${SURFACE} p-5 sm:p-6`}>
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-baseline gap-3">
            <h2 id="people-title" className={TITLE}>
              {t("people.title")}
            </h2>
            {data ? <span className="font-mono text-sm text-muted tabular-nums">{data.people.length}</span> : null}
          </div>
          <p className={`${LEAD} mt-1 text-sm`}>{t("people.intro")}</p>
        </div>
        {viewer.can("people.manage") ? (
          <button type="button" className={button({ variant: "primary", size: "sm" })} onClick={() => setMaking({ purpose: "join" })}>
            <Icon name="plus" className="size-4" />
            {t("people.invite")}
          </button>
        ) : null}
      </div>
      {people.isError ? (
        <Notice className="mt-6">{errorText(people.error)}</Notice>
      ) : !data ? (
        <ul aria-hidden="true" className={`${WELL} mt-6`}>
          {[0, 1].map((row) => (
            <li key={row} className="flex items-center justify-between border-b border-line px-4 py-3.5 last:border-b-0">
              <div className={`${SKELETON_LINE} h-4 w-32`} />
              <div className={`${SKELETON_LINE} h-3 w-40`} />
            </li>
          ))}
        </ul>
      ) : (
        <>
          <ul aria-label={t("people.title")} className={`${WELL} @container mt-6`}>
            {data.people.map((person) => (
              <PersonRow key={person.person} person={person} viewer={viewer} onReset={(target) => setMaking({ purpose: "reset", person: target })} />
            ))}
          </ul>
          {viewer.can("people.manage") ? <SignupSetting settings={data.signup} /> : null}
          <Links data={data} />
          <Changes data={data} />
        </>
      )}
      {making ? <LinkSheet what={making} onClose={() => setMaking(undefined)} /> : null}
    </section>
  );
}
