import { useId, useState, type FormEvent } from "react";
import type { ImportSource } from "../api";
import { HubCard } from "../connectors/HubCard";
import { useT } from "../i18n/I18nProvider";
import { Collapse } from "../ui/Collapse";
import { Field } from "../ui/Field";
import { Arrow } from "../ui/Icon";
import { Segmented } from "../ui/Segmented";
import { Sheet } from "../ui/Sheet";
import { WELL, button } from "../ui/styles";
import { Switch } from "../ui/Switch";
import { Review } from "./Review";
import { readSource, type SourceKind } from "./source";

/** The import window: the hub one click away, a pasted MCP, listing or skill address, then the review of what it brings. A pasted address given on opening goes straight to the review. */
export function ImportDialog({ initial, initialKind = "mcp", onClose }: { initial?: string | undefined; initialKind?: SourceKind; onClose(): void }) {
  const t = useT();
  const first = initial ? readSource(initial, "mcp", { local: false, headerSecret: false }) : undefined;
  const [text, setText] = useState(initial ?? "");
  const [kind, setKind] = useState<SourceKind>(first?.ok ? first.kind : initialKind);
  const [local, setLocal] = useState(false);
  const [secret, setSecret] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const switches = useId();
  const [reviewing, setReviewing] = useState<ImportSource | undefined>(first?.ok ? first.source : undefined);
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    const read = readSource(text, kind, { local, headerSecret: secret });
    if (!read.ok) {
      setInvalid(true);
      return;
    }
    setKind(read.kind);
    setReviewing(read.source);
  };
  return (
    <Sheet title={t("import.title")} closeLabel={t("common.close")} onClose={onClose} wide>
      {reviewing ? (
        <Review key={JSON.stringify(reviewing)} source={reviewing} onBack={() => setReviewing(undefined)} onInstalled={onClose} />
      ) : (
        <form onSubmit={onSubmit} noValidate className="swap-in flex flex-col gap-5">
          <HubCard />
          <div className="flex flex-col gap-4 sm:flex-row sm:items-end">
            <Segmented<SourceKind>
              label={t("import.kind")}
              value={kind}
              onChange={setKind}
              className="self-start sm:mb-7"
              options={[
                { value: "mcp", text: t("import.kind.mcp") },
                { value: "listing", text: t("import.kind.listing") },
              ]}
            />
            <Field
              label={t(kind === "mcp" ? "import.url.mcp" : "import.url.listing")}
              hint={t(kind === "mcp" ? "import.hint.mcp" : "import.hint.listing")}
              error={invalid ? t("import.invalid") : undefined}
              className="min-w-0 flex-1"
              mono
              autoFocus
              value={text}
              placeholder={kind === "mcp" ? "https://mcp.example.com/mcp" : "https://example.com/listing.yaml"}
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
              onChange={(event) => {
                setText(event.target.value);
                setInvalid(false);
              }}
            />
          </div>
          <Collapse id={switches} open={kind === "mcp"}>
            <div className={`${WELL} flex flex-col gap-4 p-4`}>
              <Switch checked={local} onChange={setLocal} hint={t("import.localHint")}>
                {t("import.local")}
              </Switch>
              <div className="border-t border-line" />
              <Switch checked={secret} onChange={setSecret} hint={t("import.secretHint")}>
                {t("import.secret")}
              </Switch>
            </div>
          </Collapse>
          <div className="flex flex-wrap items-center justify-end gap-2">
            <button type="button" className={button({ variant: "quiet" })} onClick={onClose}>
              {t("common.cancel")}
            </button>
            <button type="submit" className={button({ variant: "primary" })}>
              {t("import.read")}
              <Arrow />
            </button>
          </div>
        </form>
      )}
    </Sheet>
  );
}
