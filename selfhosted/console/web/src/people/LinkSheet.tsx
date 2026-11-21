import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { errorText, type NewLink, type Person, type Role } from "../api";
import { useLocale, useT } from "../i18n/I18nProvider";
import { createInvite, createReset } from "../mutations";
import { CopyBox } from "../ui/CopyBox";
import { Notice } from "../ui/Notice";
import { Sheet } from "../ui/Sheet";
import { OPTION_CARD, button } from "../ui/styles";
import { isLoopbackLink, ROLES } from "./people";

/** What the sheet makes: a join link, or a reset link for one person. */
export type LinkFor = { purpose: "join" } | { purpose: "reset"; person: Person };

/** A link just made, shown once: the copy box, when it stops working, how to send it, and a warning when only this machine can open it. */
export function LinkMade({ link }: { link: NewLink }) {
  const t = useT();
  const locale = useLocale();
  return (
    <div className="swap-in flex flex-col gap-3">
      <CopyBox label={t("invite.link")} text={link.link} />
      <p className="text-sm text-muted">{t("invite.expires", { date: new Date(link.expires_at).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" }) })}</p>
      <Notice tone="lime" live={false}>
        {t("invite.private")}
      </Notice>
      {isLoopbackLink(link.link) ? <Notice live={false}>{t("invite.loopback")}</Notice> : null}
    </div>
  );
}

/** The window that makes a join link, after one role card is picked, or a reset link for one person, and then shows it once in a copy box with when it stops working, how to send it, and a warning when it points at this machine only. */
export function LinkSheet({ what, onClose }: { what: LinkFor; onClose(): void }) {
  const t = useT();
  const [role, setRole] = useState<Role>("member");
  const invite = useMutation(createInvite);
  const reset = useMutation(createReset);
  const make = what.purpose === "join" ? invite : reset;
  const link = make.data;
  const title = what.purpose === "join" ? t("invite.title") : t("reset.title", { name: what.person.username });
  return (
    <Sheet title={title} closeLabel={t("common.close")} onClose={onClose}>
      {link ? (
        <LinkMade link={link} />
      ) : (
        <div className="flex flex-col gap-4">
          {what.purpose === "join" ? (
            <div role="group" aria-label={t("invite.role")} className="grid gap-2">
              {ROLES.map((option) => (
                <button key={option} type="button" aria-pressed={option === role} className={OPTION_CARD} onClick={() => setRole(option)}>
                  <span className="text-sm font-medium">{t(`role.${option}`)}</span>
                  <span className="text-[0.8125rem] leading-snug text-pretty text-muted">{t(`invite.line.${option}`)}</span>
                </button>
              ))}
            </div>
          ) : (
            <p className="text-pretty text-muted">{t("reset.body", { name: what.person.username })}</p>
          )}
          <p role="alert" className="text-sm text-amber-400 empty:hidden">
            {make.isError ? errorText(make.error) : ""}
          </p>
          <button
            type="button"
            disabled={make.isPending}
            className={`${button({ variant: "primary" })} w-full`}
            onClick={() => (what.purpose === "join" ? invite.mutate(role) : reset.mutate(what.person.person))}
          >
            {make.isPending ? t("invite.making") : t("invite.make")}
          </button>
        </div>
      )}
    </Sheet>
  );
}
