import type { ReactNode } from "react";

type DisplayProps = { as?: "h1" | "h2" | "h3"; size?: "hero" | "page" | "title"; id?: string; className?: string; children: ReactNode };

const SIZE = {
  hero: "text-[2.75rem] sm:text-6xl xl:text-7xl",
  page: "text-5xl sm:text-6xl",
  title: "text-4xl sm:text-5xl",
} as const;

/** The serif display voice, and the only place the console sets Instrument Serif: the one page title of a screen with its lime word, the setup and sign-in headline, and the headline of a large empty state. Everything else is Geist. */
export function Display({ as: Tag = "h1", size = "page", id, className = "", children }: DisplayProps) {
  return (
    <Tag id={id} className={`text-lit serif-room font-serif leading-[0.98] font-normal tracking-[-0.02em] text-balance ${SIZE[size]} ${className}`}>
      {children}
    </Tag>
  );
}
