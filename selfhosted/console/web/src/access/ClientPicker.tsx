import { useId, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { CopyBox } from "../ui/CopyBox";
import { Icon } from "../ui/Icon";
import { Collapse } from "../ui/Collapse";
import { OPTION_CARD, TITLE, button } from "../ui/styles";
import { Tabs, tabId } from "../ui/Tabs";
import { byUse, guides, oauthGuides, type Block, type ClientId, type Guide } from "./clients";

function Blocks({ guide }: { guide: Guide }) {
  const t = useT();
  const panel = useId();
  const [at, setAt] = useState("0");
  const label = (block: Block) => t(block.where, { path: block.path ?? "", label: block.label ?? "" });
  if (guide.choose) {
    const block = guide.blocks[Number(at)] ?? guide.blocks[0];
    return (
      <>
        <Tabs<string> label={t("clients.language")} value={at} onChange={setAt} panel={panel} options={guide.blocks.map((option, index) => ({ value: String(index), text: option.label ?? "" }))} />
        <div id={panel} role="tabpanel" aria-labelledby={tabId(panel, at)}>
          {block ? <CopyBox key={at} label={label(block)} text={block.text} className="swap-in" /> : null}
        </div>
      </>
    );
  }
  return guide.blocks.map((block) => <CopyBox key={block.where + (block.path ?? "")} label={label(block)} text={block.text} />);
}

const GRID = "grid grid-cols-2 gap-2 @md:grid-cols-3 @3xl:grid-cols-6";

/** The apps that connect to the endpoint, as cards: the five most people use in one row with More apps after them, which opens the others in place; the chosen one shows its one-click link or the exact block to paste: URL only (the app signs in through the browser), or with the access key inside when one is given. */
export function ClientPicker({ url, accessKey, heading: Heading = "h2" }: { url: string; accessKey?: string; heading?: "h2" | "h3" }) {
  const t = useT();
  const moreId = useId();
  const all = accessKey ? guides(url, accessKey, t) : oauthGuides(url, t);
  const { common, more } = byUse(all);
  const [chosen, setChosen] = useState<ClientId | undefined>();
  const [open, setOpen] = useState(false);
  const guide = all.find((g) => g.id === chosen);
  const card = (g: Guide) => (
    <button key={g.id} type="button" aria-pressed={g.id === chosen} className={OPTION_CARD} onClick={() => setChosen(g.id)}>
      <span className="text-sm font-medium">{g.name}</span>
      <span className="font-mono text-[0.6875rem] text-muted">{t(g.kind)}</span>
    </button>
  );
  return (
    <div className="@container">
      <Heading className={TITLE}>{t("clients.title")}</Heading>
      <div role="group" aria-label={t("clients.title")} className="mt-4">
        <div className={GRID}>
          {common.map(card)}
          {more.length > 0 ? (
            <button type="button" aria-expanded={open} aria-controls={moreId} className={`${OPTION_CARD} group text-muted hover:text-paper`} onClick={() => setOpen(!open)}>
              <span className="flex items-center gap-1.5 text-sm font-medium">
                {t(open ? "clients.fewer" : "clients.more")}
                <Icon name="chevron" className="size-4 transition-transform duration-250 ease-(--ease-glide) group-aria-expanded:rotate-180" />
              </span>
              <span className="font-mono text-[0.6875rem]">+{more.length}</span>
            </button>
          ) : null}
        </div>
        <div className="-mx-1.5">
          <Collapse id={moreId} open={open}>
            <div className={`${GRID} px-1.5 pt-2 pb-1.5`}>{more.map(card)}</div>
          </Collapse>
        </div>
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
