import { useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { CopyBox } from "../ui/CopyBox";
import { Icon } from "../ui/Icon";
import { Segmented } from "../ui/Segmented";
import { OPTION_CARD, TITLE, button } from "../ui/styles";
import { guides, oauthGuides, type Block, type ClientId, type Guide } from "./clients";

function Blocks({ guide }: { guide: Guide }) {
  const t = useT();
  const [at, setAt] = useState(0);
  const label = (block: Block) => t(block.where, { path: block.path ?? "", label: block.label ?? "" });
  if (guide.choose) {
    const block = guide.blocks[at] ?? guide.blocks[0];
    return (
      <>
        <div className="-m-1.5 max-w-[calc(100%+0.75rem)] self-start overflow-x-auto p-1.5">
          <Segmented<string>
            label={t("clients.language")}
            value={String(at)}
            onChange={(value) => setAt(Number(value))}
            className="w-max"
            options={guide.blocks.map((option, index) => ({ value: String(index), text: option.label ?? "" }))}
          />
        </div>
        {block ? <CopyBox key={at} label={label(block)} text={block.text} className="swap-in" /> : null}
      </>
    );
  }
  return guide.blocks.map((block) => <CopyBox key={block.where + (block.path ?? "")} label={label(block)} text={block.text} />);
}

/** The apps that connect to the endpoint, as cards; the chosen one shows its one-click link or the exact block to paste: URL only (the app signs in through the browser), or with the access key inside when one is given. */
export function ClientPicker({ url, accessKey, heading: Heading = "h2" }: { url: string; accessKey?: string; heading?: "h2" | "h3" }) {
  const t = useT();
  const all = accessKey ? guides(url, accessKey, t) : oauthGuides(url, t);
  const [chosen, setChosen] = useState<ClientId | undefined>();
  const guide = all.find((g) => g.id === chosen);
  return (
    <div className="@container">
      <Heading className={TITLE}>{t("clients.title")}</Heading>
      <div role="group" aria-label={t("clients.title")} className="mt-4 grid grid-cols-2 gap-2 @md:grid-cols-3 @3xl:grid-cols-5">
        {all.map((g) => (
          <button key={g.id} type="button" aria-pressed={g.id === chosen} className={OPTION_CARD} onClick={() => setChosen(g.id)}>
            <span className="text-sm font-medium">{g.name}</span>
            <span className="font-mono text-[0.6875rem] text-muted">{t(g.kind)}</span>
          </button>
        ))}
      </div>
      {guide ? (
        <div key={guide.id} className="swap-in mt-5 flex flex-col gap-3">
          {guide.link ? (
            <div className="flex flex-wrap items-center gap-3">
              <a href={guide.link.href} className={button({ variant: "primary" })}>
                <Icon name="external" className="size-4" />
                {t("clients.open", { app: guide.link.app })}
              </a>
              <span className="text-sm text-muted">{t("clients.orByHand")}</span>
            </div>
          ) : null}
          <Blocks guide={guide} />
          <p className="text-sm text-pretty text-muted">{t(guide.note)}</p>
        </div>
      ) : (
        <p className="mt-4 text-sm text-pretty text-muted">{t(accessKey ? "clients.pick" : "clients.pickOAuth")}</p>
      )}
    </div>
  );
}
