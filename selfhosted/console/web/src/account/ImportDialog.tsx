import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { errorText, type BackupSummary, type SetupResult, type SlotChoice } from "../api";
import { useT } from "../i18n/I18nProvider";
import { importBackup, inspectBackup } from "../mutations";
import { navigate } from "../router";
import { Field } from "../ui/Field";
import { Notice } from "../ui/Notice";
import { useViewer } from "../people/useViewer";
import { Sheet } from "../ui/Sheet";
import { button } from "../ui/styles";
import { backupKindOf, passwordRefusal } from "./backup";
import { BackupSummaryView } from "./BackupSummaryView";
import { NameList } from "./NameList";
import { keyMissing, optionsOf } from "./setup";
import { SetupSlots } from "./SetupSlots";

type Step = { kind: "pick" } | { kind: "review"; summary: BackupSummary } | { kind: "done"; result: SetupResult };

type ImportDialogProps = { onClose(): void; restoreOnly?: boolean };

function SetupDone({ result, onClose }: { result: SetupResult; onClose(): void }) {
  const t = useT();
  const added = result.added ?? [];
  const signIns = new Set(result.sign_ins ?? []);
  const capped = Object.entries(result.capped ?? {}).flatMap(([connector, lowered]) =>
    Object.entries(lowered).map(([name, access]) => ({ name: `${connector} / ${name}`, note: t("backup.result.cappedTo", { access: t(`access.${access}`) }) })),
  );
  return (
    <div className="swap-in">
      {added.length > 0 ? <Notice live={false}>{t("backup.result.signIn")}</Notice> : <p className="text-sm text-muted">{t("backup.result.nothing")}</p>}
      <NameList title={t("backup.result.added")} rows={added.map((name) => ({ name, note: signIns.has(name) ? t("backup.result.needsSignIn") : undefined }))} />
      <NameList title={t("backup.result.kept")} rows={(result.kept ?? []).map((name) => ({ name }))} />
      <NameList title={t("backup.result.sets")} rows={(result.slots ?? []).map((name) => ({ name }))} />
      <NameList title={t("backup.result.capped")} rows={capped} />
      <NameList title={t("backup.result.failed")} rows={Object.entries(result.failed ?? {}).map(([name, note]) => ({ name, note }))} />
      <button type="button" autoFocus className={`${button({ variant: "primary", size: "sm" })} mt-6`} onClick={onClose}>
        {t("backup.done")}
      </button>
    </div>
  );
}

/** The window that imports a backup file: pick it, give a full backup's passphrase, read what it holds, for a setup answer what becomes of each permission set and whether the keys made already get its connectors, confirm, then import. A full backup restarts the console and moves to the restore screen; a setup shows what it added, kept and failed. `restoreOnly` takes full backups alone, as on first run; otherwise it takes the kinds the server lets the viewer import. */
export function ImportDialog({ onClose, restoreOnly = false }: ImportDialogProps) {
  const t = useT();
  const [file, setFile] = useState<File>();
  const [passphrase, setPassphrase] = useState("");
  const [password, setPassword] = useState("");
  const [step, setStep] = useState<Step>({ kind: "pick" });
  const [choices, setChoices] = useState<Record<string, SlotChoice>>({});
  const [existingKeys, setExistingKeys] = useState(false);
  const inspect = useMutation(inspectBackup);
  const run = useMutation(importBackup);
  const viewer = useViewer();
  const full = restoreOnly || viewer.can("backup.full");
  const setups = !restoreOnly && viewer.can("setup.manage");
  const needsPassphrase = file !== undefined && backupKindOf(file.name) !== "setup";
  const upload = file ? { file, passphrase: needsPassphrase ? passphrase : "" } : undefined;
  const onInspect = (event: FormEvent) => {
    event.preventDefault();
    if (upload) inspect.mutate(upload, { onSuccess: (summary) => setStep({ kind: "review", summary }) });
  };
  const asksPassword = !restoreOnly && step.kind === "review" && step.summary.kind === "full";
  const refusal = run.isError ? passwordRefusal(run.error) : undefined;
  const onImport = (event: FormEvent) => {
    event.preventDefault();
    const options = step.kind === "review" && step.summary.kind === "setup" ? optionsOf(choices, existingKeys) : undefined;
    if (upload) run.mutate(asksPassword ? { ...upload, password } : { ...upload, options }, { onSuccess: (outcome) => (outcome.kind === "full" ? navigate("/restored") : setStep({ kind: "done", result: outcome.result })) });
  };
  const replaces = step.kind === "review" && step.summary.kind === "full" && step.summary.replaces;
  return (
    <Sheet title={t(restoreOnly ? "auth.restore" : "backup.import.title")} closeLabel={t("common.close")} onClose={run.isPending ? () => undefined : onClose}>
      {step.kind === "pick" ? (
        <form onSubmit={onInspect} className="swap-in">
          <Field
            label={t("backup.import.file")}
            hint={t(full && setups ? "backup.import.fileHint" : full ? "backup.restore.fileHint" : "backup.import.setupHint")}
            type="file"
            accept={full && setups ? ".cebak,.yaml,.yml" : full ? ".cebak" : ".yaml,.yml"}
            required
            onChange={(event) => {
              setFile(event.target.files?.[0]);
              inspect.reset();
            }}
          />
          {needsPassphrase ? (
            <Field label={t("backup.passphrase")} hint={t("backup.import.passphraseHint")} type="password" autoComplete="off" required value={passphrase} onChange={(event) => setPassphrase(event.target.value)} />
          ) : null}
          <p role="alert" className="mb-2 text-sm text-amber-400 empty:hidden">
            {inspect.isError ? errorText(inspect.error) : ""}
          </p>
          <button type="submit" disabled={!file || inspect.isPending} className={`${button({ variant: "primary" })} w-full`}>
            {inspect.isPending ? t("backup.inspecting") : t("backup.inspect")}
          </button>
        </form>
      ) : step.kind === "review" ? (
        <form onSubmit={onImport} className="swap-in">
          <BackupSummaryView summary={step.summary} />
          {step.summary.kind === "setup" ? (
            <SetupSlots
              sets={step.summary.setup?.slots ?? []}
              choices={choices}
              onChoose={(set, choice) => setChoices((current) => ({ ...current, [set]: choice }))}
              existingKeys={existingKeys}
              onExistingKeys={setExistingKeys}
            />
          ) : null}
          {replaces ? <Notice live={false} className="mt-5">{t("backup.replaces")}</Notice> : null}
          {asksPassword ? (
            <Field
              label={t("backup.full.password")}
              hint={t("backup.import.passwordHint")}
              error={refusal ? t(refusal) : undefined}
              className="mt-5"
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          ) : null}
          <p role="alert" className="mt-3 text-sm text-amber-400 empty:hidden">
            {run.isError && !refusal ? errorText(run.error) : ""}
          </p>
          <div className="mt-4 flex flex-wrap justify-end gap-2">
            <button
              type="button"
              disabled={run.isPending}
              className={button({ variant: "quiet", size: "sm" })}
              onClick={() => {
                setFile(undefined);
                setChoices({});
                setExistingKeys(false);
                setStep({ kind: "pick" });
              }}
            >
              {t("backup.back")}
            </button>
            <button type="submit" disabled={run.isPending || (step.summary.kind === "setup" && keyMissing(choices))} className={button({ variant: replaces ? "danger" : "primary", size: "sm" })}>
              {run.isPending ? t("backup.importing") : t(step.summary.kind === "setup" ? "backup.importSetup" : replaces ? "backup.replace" : "backup.restore")}
            </button>
          </div>
        </form>
      ) : (
        <SetupDone result={step.result} onClose={onClose} />
      )}
    </Sheet>
  );
}
