import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { exportBackup } from "../mutations";
import { Field } from "../ui/Field";
import { Notice } from "../ui/Notice";
import { Sheet } from "../ui/Sheet";
import { Switch } from "../ui/Switch";
import { OPTION_CARD, button } from "../ui/styles";
import { PASSPHRASE_MIN, passwordRefusal, saveFile, secretProblem } from "./backup";
import { SecretPair } from "./SecretPair";

type Step = "pick" | "warn" | "form";

function FullForm({ onBack, onDone }: { onBack(): void; onDone(): void }) {
  const t = useT();
  const [understood, setUnderstood] = useState(false);
  const [password, setPassword] = useState("");
  const [passphrase, setPassphrase] = useState({ value: "", again: "" });
  const [checked, setChecked] = useState(false);
  const run = useMutation(exportBackup);
  const refusal = run.isError ? passwordRefusal(run.error) : undefined;
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    setChecked(true);
    if (!understood || secretProblem(passphrase.value, passphrase.again, PASSPHRASE_MIN)) return;
    run.mutate(
      { mode: "full", confirm: true, password, passphrase: passphrase.value },
      {
        onSuccess: (file) => {
          saveFile(file, "connector-engine-backup.cebak");
          onDone();
        },
      },
    );
  };
  return (
    <form onSubmit={onSubmit} className="swap-in flex flex-col gap-4">
      <Switch checked={understood} onChange={setUnderstood}>
        {t("backup.full.understand")}
      </Switch>
      <div>
        <Field label={t("backup.full.password")} type="password" autoComplete="current-password" required value={password} error={refusal ? t(refusal) : undefined} onChange={(event) => setPassword(event.target.value)} />
        <SecretPair
          label={t("backup.passphrase")}
          againLabel={t("backup.passphraseAgain")}
          hint={t("backup.passphraseHint", { min: PASSPHRASE_MIN })}
          min={PASSPHRASE_MIN}
          value={passphrase.value}
          again={passphrase.again}
          checked={checked}
          onChange={(value, again) => setPassphrase({ value, again })}
        />
        <p role="alert" className="text-sm text-amber-400 empty:hidden">
          {run.isError && !refusal ? errorText(run.error) : ""}
        </p>
      </div>
      <div className="flex flex-wrap justify-end gap-2">
        <button type="button" className={button({ variant: "quiet", size: "sm" })} onClick={onBack}>
          {t("backup.back")}
        </button>
        <button type="submit" disabled={!understood || run.isPending} className={button({ variant: "primary", size: "sm" })}>
          {run.isPending ? t("backup.exporting") : t("backup.full.submit")}
        </button>
      </div>
    </form>
  );
}

function FullWarning({ onBack, onContinue }: { onBack(): void; onContinue(): void }) {
  const t = useT();
  return (
    <div className="swap-in flex flex-col gap-4 text-sm">
      <div>
        <p className="font-medium">{t("backup.full.holds")}</p>
        <ul className="mt-2 flex list-disc flex-col gap-1 pl-5 text-muted">
          <li>{t("backup.full.holds1")}</li>
          <li>{t("backup.full.holds2")}</li>
          <li>{t("backup.full.holds3")}</li>
        </ul>
      </div>
      <Notice live={false}>{t("backup.full.danger")}</Notice>
      <p className="text-muted">{t("backup.full.lost")}</p>
      <div className="flex flex-wrap justify-end gap-2">
        <button type="button" className={button({ variant: "quiet", size: "sm" })} onClick={onBack}>
          {t("backup.back")}
        </button>
        <button type="button" autoFocus className={button({ variant: "primary", size: "sm" })} onClick={onContinue}>
          {t("backup.full.continue")}
        </button>
      </div>
    </div>
  );
}

/** The window that exports this install: a shareable setup downloads at once; a full backup first says plainly what the file holds, then asks for the confirmation, the password and a passphrase typed twice. */
export function ExportDialog({ onClose }: { onClose(): void }) {
  const t = useT();
  const [step, setStep] = useState<Step>("pick");
  const setup = useMutation(exportBackup);
  const onSetup = () =>
    setup.mutate(
      { mode: "setup" },
      {
        onSuccess: (file) => {
          saveFile(file, "connector-engine.setup.yaml");
          onClose();
        },
      },
    );
  return (
    <Sheet title={t("backup.export.title")} closeLabel={t("common.close")} onClose={onClose}>
      {step === "pick" ? (
        <div className="swap-in">
          <p className="text-sm text-muted">{t("backup.export.pick")}</p>
          <div role="group" aria-label={t("backup.export.kind")} className="mt-3 flex flex-col gap-2">
            <button type="button" className={OPTION_CARD} onClick={() => setStep("warn")}>
              <span className="font-semibold">{t("backup.full.title")}</span>
              <span className="text-sm text-muted">{t("backup.full.body")}</span>
            </button>
            <button type="button" disabled={setup.isPending} className={OPTION_CARD} onClick={onSetup}>
              <span className="font-semibold">{t("backup.setup.title")}</span>
              <span className="text-sm text-muted">{setup.isPending ? t("backup.exporting") : t("backup.setup.body")}</span>
            </button>
          </div>
          <p role="alert" className="mt-3 text-sm text-amber-400 empty:hidden">
            {setup.isError ? errorText(setup.error) : ""}
          </p>
        </div>
      ) : step === "warn" ? (
        <FullWarning onBack={() => setStep("pick")} onContinue={() => setStep("form")} />
      ) : (
        <FullForm onBack={() => setStep("warn")} onDone={onClose} />
      )}
    </Sheet>
  );
}
