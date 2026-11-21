import { useEffect, useState } from "react";

/** The time now, read once when the component first draws and again every `every` milliseconds (half a minute by default), so what it words as "5 minutes ago" keeps up; the timer stops with the component. */
export function useNow(every = 30_000): number {
  const [now, setNow] = useState(Date.now);
  useEffect(
    function tickEvery() {
      const timer = setInterval(() => setNow(Date.now()), every);
      return () => clearInterval(timer);
    },
    [every],
  );
  return now;
}
