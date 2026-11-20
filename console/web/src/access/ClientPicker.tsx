import { useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { CopyBox } from "../ui/CopyBox";
import { Icon } from "../ui/Icon";
import { Segmented } from "../ui/Segmented";
import { PRESS, button } from "../ui/styles";
import { guides, oauthGuides, type Block, type ClientId, type Guide } from "./clients";

const CARD = `bubble flex min-h-16 flex-col items-start justify-center gap-0.5 border border-line-strong bg-card px-4 py-2.5 text-left hover:border-fg/40 aria-pressed:border-transparent aria-pressed:bg-bubble aria-pressed:text-on-bubble ${PRESS}`;

function Blocks({ guide }: { guide: Guide }) {
  const t = useT();
  const [at, setAt] = useState(0);
  const label = (block: Block) => t(block.where, { path: block.path ?? "", label: block.label ?? "" });
  if (guide.choose) {
    const block = guide.blocks[at] ?? guide.blocks[0];
    return (
      <>
        <Segmented<string>
          label={t("clients.language")}
          value={String(at)}
          onChange={(value) => setAt(Number(value))}
          className="self-start"
          options={guide.blocks.map((option, index) => ({ value: String(index), text: option.label ?? "" }))}
        />
        {block ? <CopyBox key={at} label={label(block)} text={block.text} className="swap-in" /> : null}
      </>
    );
  }
  return guide.blocks.map((block) => <CopyBox key={block.where + (block.path ?? "")} label={label(block)} text={block.text} />);
}

/** The apps that connect to the endpoint, as cards; the chosen one shows its one-click link or the exact block to paste: URL only (the app signs in through the browser), or with the access key inside when one is given. */
export function ClientPicker({ url, accessKey }: { url: string; accessKey?: string }) {
  const t = useT();
  const all = accessKey ? guides(url, accessKey, t) : oauthGuides(url, t);
  const [chosen, setChosen] = useState<ClientId | undefined>();
  const guide = all.find((g) => g.id === chosen);
  return (
    <div>
      <h3 className="font-display text-lg font-bold tracking-[-0.03em]">{t("clients.title")}</h3>
      <div role="group" aria-label={t("clients.title")} className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
        {all.map((g) => (
          <button key={g.id} type="button" aria-pressed={g.id === chosen} className={CARD} onClick={() => setChosen(g.id)}>
            <span className="text-sm font-semibold">{g.name}</span>
            <span className="font-mono text-[0.6875rem] opacity-70">{t(g.kind)}</span>
          </button>
        ))}
      </div>
      {guide ? (
        <div key={guide.id} className="swap-in mt-5 flex flex-col gap-3">
          {guide.link ? (
            <div className="flex flex-wrap items-center gap-3">
              <a href={guide.link.href} className={button({ variant: "accent" })}>
                <Icon name="external" className="size-4" />
                {t("clients.open", { app: guide.link.app })}
              </a>
              <span className="text-sm text-muted">{t("clients.orByHand")}</span>
            </div>
          ) : null}
          <Blocks guide={guide} />
          <p className="text-sm text-muted">{t(guide.note)}</p>
        </div>
      ) : (
        <p className="mt-4 text-sm text-muted">{t(accessKey ? "clients.pick" : "clients.pickOAuth")}</p>
      )}
    </div>
  );
}
