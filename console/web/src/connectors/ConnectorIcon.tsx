import { useState } from "react";
import { iconURL } from "../api";

const SIZE = { sm: "size-9 text-sm", md: "size-11 text-base", lg: "size-16 text-2xl" } as const;

/** A listing icon proxied through the console as image bytes on a paper disc, or the first letter of its title. */
export function ConnectorIcon({ src, title, size = "md", onLime = false }: { src?: string | undefined; title: string; size?: keyof typeof SIZE; onLime?: boolean }) {
  const [broken, setBroken] = useState(false);
  const url = iconURL(src);
  if (!url || broken) {
    return (
      <span aria-hidden="true" className={`grid shrink-0 place-items-center rounded-full font-mono uppercase ${onLime ? "bg-ink text-lime" : "bg-paper/10 text-muted"} ${SIZE[size]}`}>
        {title.slice(0, 1).toUpperCase()}
      </span>
    );
  }
  return (
    <span aria-hidden="true" className={`grid shrink-0 place-items-center overflow-hidden rounded-full bg-paper ${SIZE[size]}`}>
      <img className="size-[62%] object-contain" src={url} alt="" referrerPolicy="no-referrer" onError={() => setBroken(true)} />
    </span>
  );
}
