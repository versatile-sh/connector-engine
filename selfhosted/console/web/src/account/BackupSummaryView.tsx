import type { BackupSummary, SetupConnector } from "../api";
import { overrideCount } from "../connectors/profiles";
import { useLocale, useT } from "../i18n/I18nProvider";
import type { MessageKey } from "../i18n";
import { CAPTION, EYEBROW, WELL } from "../ui/styles";
import { NameList } from "./NameList";

const COUNTS: readonly { key: "connectors" | "skills" | "keys"; label: MessageKey }[] = [
  { key: "connectors", label: "backup.count.connectors" },
  { key: "skills", label: "backup.count.skills" },
  { key: "keys", label: "backup.count.keys" },
];

/** What a backup file holds before it is imported: its kind, build and date, the counts, and for a setup each connector and skill, marked kept when installed already, the access key profiles it applies by key name, the channels to set up again and what was removed for safety. */
export function BackupSummaryView({ summary }: { summary: BackupSummary }) {
  const t = useT();
  const profileRows = (c: SetupConnector) => [
    ...(c.new_keys_off ? [{ name: c.name, note: t("backup.newKeysOff") }] : []),
    ...(c.profiles ?? []).map((p) => ({ name: `${c.name} / ${p.agent}`, note: p.mode === "custom" ? t("keys.modeCustom", { count: overrideCount(p) }) : t(`profile.mode.${p.mode}`) })),
  ];
  const locale = useLocale();
  const setup = summary.setup;
  const kept = (exists: boolean) => (exists ? t("backup.kept") : undefined);
  const facts = [
    summary.created_at ? t("backup.summary.made", { date: new Date(summary.created_at).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" }) }) : undefined,
    summary.build ? t("backup.summary.build", { build: summary.build }) : undefined,
  ].filter(Boolean);
  return (
    <div className="swap-in">
      <p className={EYEBROW}>{t(summary.kind === "full" ? "backup.summary.full" : "backup.summary.setup")}</p>
      {facts.length > 0 ? <p className={`mt-1 ${CAPTION}`}>{facts.join(" · ")}</p> : null}
      <dl className="mt-4 grid grid-cols-3 gap-2">
        {COUNTS.map((count) => (
          <div key={count.key} className={`${WELL} p-3`}>
            <dt className={CAPTION}>{t(count.label)}</dt>
            <dd className="mt-1 text-2xl tabular-nums">{summary.counts?.[count.key] ?? 0}</dd>
          </div>
        ))}
      </dl>
      {setup ? (
        <>
          <NameList title={t("backup.count.connectors")} rows={(setup.connectors ?? []).map((c) => ({ name: c.name, note: kept(c.exists) }))} />
          <NameList title={t("backup.keyProfiles")} rows={(setup.connectors ?? []).filter((c) => !c.exists).flatMap(profileRows)} />
          <NameList title={t("backup.count.skills")} rows={(setup.skills ?? []).map((s) => ({ name: s.name, note: kept(s.exists) }))} />
          <NameList title={t("backup.notifications")} rows={[...new Set(setup.notifications ?? [])].map((name) => ({ name }))} />
          <NameList title={t("backup.stripped")} rows={[...new Set(setup.stripped ?? [])].map((name) => ({ name }))} />
        </>
      ) : null}
    </div>
  );
}
