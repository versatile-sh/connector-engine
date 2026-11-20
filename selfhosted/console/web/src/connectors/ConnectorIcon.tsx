import { useState } from "react";
import { iconURL } from "../api";

const SIZE = { xs: "size-5 rounded-md text-[0.625rem]", sm: "size-9 rounded-lg text-sm", md: "size-11 rounded-xl text-base", lg: "size-16 rounded-2xl text-2xl" } as const;

/** A listing icon proxied through the console as image bytes on a light tile, as the site's directory draws logos, or the first letter of its title on a hairline tile. */
export function ConnectorIcon({ src, title, size = "md" }: { src?: string | undefined; title: string; size?: keyof typeof SIZE }) {
  const [broken, setBroken] = useState(false);
  const url = iconURL(src);
  if (!url || broken) {
    return (
      <span aria-hidden="true" className={`grid shrink-0 place-items-center border border-line-strong font-mono text-muted uppercase ${SIZE[size]}`}>
        {title.slice(0, 1).toUpperCase()}
      </span>
    );
  }
  return (
    <span aria-hidden="true" className={`grid shrink-0 place-items-center overflow-hidden bg-paper ${SIZE[size]}`}>
      <img className="size-[68%] object-contain" src={url} alt="" referrerPolicy="no-referrer" onError={() => setBroken(true)} />
    </span>
  );
}
