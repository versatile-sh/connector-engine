import { useEffect, useState } from "react";

const SHOWN_MS = 2000;

/** How the last copy went: nothing to say, copied, or refused by the browser. */
export type CopyResult = "idle" | "copied" | "failed";

/** Copies text to the clipboard and says how it went for a moment, then goes quiet again. */
export function useCopy(): { result: CopyResult; copy(text: string): Promise<void> } {
  const [last, setLast] = useState<{ result: Exclude<CopyResult, "idle"> } | undefined>();
  useEffect(
    function quietAfterAWhile() {
      if (!last) return;
      const timer = setTimeout(() => setLast(undefined), SHOWN_MS);
      return () => clearTimeout(timer);
    },
    [last],
  );
  return {
    result: last?.result ?? "idle",
    async copy(text) {
      try {
        await navigator.clipboard.writeText(text);
        setLast({ result: "copied" });
      } catch {
        setLast({ result: "failed" });
      }
    },
  };
}
