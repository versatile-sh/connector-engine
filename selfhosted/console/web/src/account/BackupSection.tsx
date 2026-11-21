import { useState } from "react";
import { useLocale, useT } from "../i18n/I18nProvider";
import { useViewer } from "../people/useViewer";
import { Section } from "../notifications/Section";
import { Notice } from "../ui/Notice";
import { button } from "../ui/styles";
import { daysSince, needsExport } from "./backup";
import { ExportDialog } from "./ExportDialog";
import { ImportDialog } from "./ImportDialog";

/** The backups of this install: when the last full one was made, a nudge when there is none or it is old, and the ways to export and import; a viewer who may only share setups sees that instead of the full-backup state. */
export function BackupSection({ lastExport }: { lastExport: string | undefined }) {
  const t = useT();
  const locale = useLocale();
  const [open, setOpen] = useState<"export" | "import">();
  const days = daysSince(lastExport, new Date());
  const full = useViewer().can("backup.full");
  return (
    <Section
      id="backup-title"
      icon="shield"
      title={t("backup.title")}
      intro={t("backup.intro")}
      action={
        <div className="flex gap-2">
          <button type="button" className={button({ variant: "primary", size: "sm" })} onClick={() => setOpen("export")}>
            {t("backup.export")}
          </button>
          <button type="button" className={button({ variant: "secondary", size: "sm" })} onClick={() => setOpen("import")}>
            {t("backup.import")}
          </button>
        </div>
      }
    >
      {full ? (
        <>
          <p className="text-sm" title={days === undefined ? undefined : new Date(lastExport ?? "").toLocaleString(locale)}>
            {days === undefined ? t("backup.never") : t("backup.last", { when: new Intl.RelativeTimeFormat(locale, { numeric: "auto" }).format(-days, "day") })}
          </p>
          {needsExport(days) ? <Notice live={false} className="mt-3">{t("backup.nudge")}</Notice> : null}
        </>
      ) : (
        <p className="text-sm text-muted">{t("backup.setupOnly")}</p>
      )}
      {open === "export" ? <ExportDialog onClose={() => setOpen(undefined)} /> : null}
      {open === "import" ? <ImportDialog onClose={() => setOpen(undefined)} /> : null}
    </Section>
  );
}
