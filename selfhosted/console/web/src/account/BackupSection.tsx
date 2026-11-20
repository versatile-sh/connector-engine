import { useState } from "react";
import { Warning } from "../approve/Warning";
import { useLocale, useT } from "../i18n/I18nProvider";
import { Section } from "../notifications/Section";
import { button } from "../ui/styles";
import { daysSince, needsExport } from "./backup";
import { ExportDialog } from "./ExportDialog";
import { ImportDialog } from "./ImportDialog";

/** The backups of this install: when the last full one was made, a nudge when there is none or it is old, and the ways to export and import. */
export function BackupSection({ lastExport }: { lastExport: string | undefined }) {
  const t = useT();
  const locale = useLocale();
  const [open, setOpen] = useState<"export" | "import">();
  const days = daysSince(lastExport, new Date());
  return (
    <Section
      id="backup-title"
      icon="shield"
      title={t("backup.title")}
      intro={t("backup.intro")}
      action={
        <div className="flex gap-2">
          <button type="button" className={button({ variant: "accent", size: "sm" })} onClick={() => setOpen("export")}>
            {t("backup.export")}
          </button>
          <button type="button" className={button({ variant: "secondary", size: "sm" })} onClick={() => setOpen("import")}>
            {t("backup.import")}
          </button>
        </div>
      }
    >
      <p className="text-sm" title={days === undefined ? undefined : new Date(lastExport ?? "").toLocaleString(locale)}>
        {days === undefined ? t("backup.never") : t("backup.last", { when: new Intl.RelativeTimeFormat(locale, { numeric: "auto" }).format(-days, "day") })}
      </p>
      {needsExport(days) ? <Warning className="mt-3">{t("backup.nudge")}</Warning> : null}
      {open === "export" ? <ExportDialog onClose={() => setOpen(undefined)} /> : null}
      {open === "import" ? <ImportDialog onClose={() => setOpen(undefined)} /> : null}
    </Section>
  );
}
