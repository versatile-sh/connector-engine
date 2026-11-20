import { useEffect, useState } from "react";

/** The wall clock in milliseconds, ticking every second while `running`. */
export function useNow(running: boolean): number {
  const [now, setNow] = useState(Date.now);
  useEffect(
    function tickWithTheClock() {
      if (!running) return;
      const timer = setInterval(() => setNow(Date.now()), 1000);
      return () => clearInterval(timer);
    },
    [running],
  );
  return now;
}
