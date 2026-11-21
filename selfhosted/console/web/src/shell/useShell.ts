import { useEffect, useReducer, useSyncExternalStore } from "react";
import { drawerOpen, loadCollapsed, railOf, saveCollapsed, shellAt, shellOf, shellReducer, WIDE, type Store } from "./shell";

const store = (): Store | undefined => {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
};

const subscribe = (onChange: () => void) => {
  const query = window.matchMedia(WIDE);
  query.addEventListener("change", onChange);
  return () => query.removeEventListener("change", onChange);
};

/** How the shell is laid out at `at`, the address on screen: whether the screen is wide, whether the sidebar is the icon rail (kept in localStorage) and whether the drawer is open, with the ways to change them. A drawer opened on one page is shut on the next, and by a wide screen. */
export function useShell(at: string) {
  const wide = useSyncExternalStore(subscribe, () => window.matchMedia(WIDE).matches, () => false);
  const [state, dispatch] = useReducer(shellReducer, undefined, () => shellOf(loadCollapsed(store())));
  if (shellAt(state, at) !== state) dispatch({ type: "close" });
  useEffect(function shutTheDrawerWhenWide() {
    const query = window.matchMedia(WIDE);
    const widen = (event: MediaQueryListEvent) => {
      if (event.matches) dispatch({ type: "widen" });
    };
    query.addEventListener("change", widen);
    return () => query.removeEventListener("change", widen);
  }, []);
  return {
    rail: railOf(state, wide),
    drawer: drawerOpen(state, wide, at),
    toggleRail() {
      const collapsed = !state.collapsed;
      saveCollapsed(store(), collapsed);
      dispatch({ type: "rail", collapsed });
    },
    openDrawer: () => dispatch({ type: "open", at }),
    closeDrawer: () => dispatch({ type: "close" }),
  };
}
