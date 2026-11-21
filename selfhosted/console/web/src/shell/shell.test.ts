import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { en } from "../i18n/en.ts";
import {
  closesDrawer,
  COLLAPSED_KEY,
  crumbOf,
  drawerOpen,
  initialOf,
  loadCollapsed,
  NAV_GROUPS,
  navView,
  parseCollapsed,
  railOf,
  saveCollapsed,
  serializeCollapsed,
  shellAt,
  shellOf,
  shellReducer,
  wrapFocus,
  type Section,
  type Store,
} from "./shell.ts";

const SECTIONS: readonly Section[] = ["connectors", "skills", "access", "approvals", "activity", "notifications", "account"];

const memory = (initial: Record<string, string> = {}): Store & { saved: Record<string, string> } => {
  const saved = { ...initial };
  return { saved, getItem: (key) => saved[key] ?? null, setItem: (key, value) => void (saved[key] = value) };
};

const refusing: Store = {
  getItem() {
    throw new DOMException("blocked", "SecurityError");
  },
  setItem() {
    throw new DOMException("full", "QuotaExceededError");
  },
};

describe("the sidebar's links", () => {
  test("hold every section once, Gateway before Settings", () => {
    assert.deepEqual(
      NAV_GROUPS.map((group) => group.items.map((item) => item.section)),
      [["connectors", "skills", "access", "approvals", "activity"], ["notifications", "account"]],
    );
  });

  test("give each section its own path and its own icon", () => {
    const items = NAV_GROUPS.flatMap((group) => group.items);
    assert.equal(new Set(items.map((item) => item.href)).size, items.length);
    assert.equal(new Set(items.map((item) => item.icon)).size, items.length);
  });

  test("name themselves with messages the console has", () => {
    for (const group of NAV_GROUPS) {
      assert.ok(group.label in en, group.label);
      for (const item of group.items) assert.ok(item.label in en, item.label);
    }
  });
});

describe("navView", () => {
  for (const section of SECTIONS) {
    describe(`on ${section}`, () => {
      test("marks that link current and no other", () => {
        const current = navView(section, 0).flatMap((group) => group.items.filter((item) => item.current).map((item) => item.section));
        assert.deepEqual(current, [section]);
      });
    });
  }

  describe("with calls waiting", () => {
    test("counts them on Approvals alone", () => {
      const counts = navView("connectors", 3).flatMap((group) => group.items.map((item) => [item.section, item.waiting]));
      assert.deepEqual(counts.filter(([, waiting]) => waiting !== 0), [["approvals", 3]]);
    });

    test("never counts below zero", () => {
      assert.equal(navView("approvals", -2).flatMap((group) => group.items).find((item) => item.section === "approvals")?.waiting, 0);
    });
  });
});

describe("crumbOf", () => {
  test("names the group and the link of a section", () => {
    assert.deepEqual(crumbOf("activity"), { group: "nav.group.gateway", label: "nav.activity" });
    assert.deepEqual(crumbOf("account"), { group: "nav.group.settings", label: "nav.account" });
  });
});

describe("the saved width", () => {
  test("reads only the word collapsed as a rail", () => {
    assert.equal(parseCollapsed("collapsed"), true);
    for (const other of ["expanded", "en", "true", "", null, undefined]) assert.equal(parseCollapsed(other), false);
  });

  test("round-trips through its own value", () => {
    for (const collapsed of [true, false]) assert.equal(parseCollapsed(serializeCollapsed(collapsed)), collapsed);
  });

  describe("with a store", () => {
    test("opens wide when nothing was saved", () => {
      assert.equal(loadCollapsed(memory()), false);
    });

    test("opens as the rail after a choice to collapse", () => {
      const store = memory();
      saveCollapsed(store, true);
      assert.equal(store.saved[COLLAPSED_KEY], "collapsed");
      assert.equal(loadCollapsed(store), true);
    });

    test("opens wide again after a choice to expand", () => {
      const store = memory({ [COLLAPSED_KEY]: "collapsed" });
      saveCollapsed(store, false);
      assert.equal(loadCollapsed(store), false);
    });

    test("ignores a value another key of a shared store would read as true", () => {
      assert.equal(loadCollapsed({ getItem: () => "en", setItem() {} }), false);
    });
  });

  describe("without a usable store", () => {
    test("opens wide when there is none", () => {
      assert.equal(loadCollapsed(undefined), false);
    });

    test("opens wide when the store refuses to be read", () => {
      assert.equal(loadCollapsed(refusing), false);
    });

    test("keeps working when the store refuses to be written", () => {
      assert.doesNotThrow(() => saveCollapsed(refusing, true));
    });
  });
});

describe("the shell", () => {
  describe("at the start", () => {
    test("keeps the saved width and the drawer shut", () => {
      assert.deepEqual(shellOf(true), { collapsed: true, drawerAt: undefined });
    });
  });

  describe("on a wide screen", () => {
    test("is a rail only when chosen", () => {
      assert.equal(railOf(shellOf(true), true), true);
      assert.equal(railOf(shellOf(false), true), false);
    });

    test("never opens the drawer", () => {
      assert.equal(drawerOpen(shellReducer(shellOf(false), { type: "open", at: "/" }), true, "/"), false);
    });
  });

  describe("on a narrow screen", () => {
    test("is never a rail, whatever was chosen", () => {
      assert.equal(railOf(shellOf(true), false), false);
    });

    test("opens the drawer on the page it is on", () => {
      assert.equal(drawerOpen(shellReducer(shellOf(false), { type: "open", at: "/skills" }), false, "/skills"), true);
    });

    test("shuts the drawer on a move to another page", () => {
      const open = shellReducer(shellOf(false), { type: "open", at: "/skills" });
      assert.equal(drawerOpen(open, false, "/access"), false);
    });

    test("shuts the drawer when asked", () => {
      const open = shellReducer(shellOf(false), { type: "open", at: "/" });
      assert.equal(drawerOpen(shellReducer(open, { type: "close" }), false, "/"), false);
    });

    test("keeps the viewer's rail choice while the drawer opens and shuts", () => {
      const open = shellReducer(shellOf(true), { type: "open", at: "/" });
      assert.equal(shellReducer(open, { type: "close" }).collapsed, true);
    });
  });

  describe("when the page moves on and comes back", () => {
    test("leaves the drawer shut on the address it was opened on", () => {
      const open = shellReducer(shellOf(false), { type: "open", at: "/" });
      const away = shellAt(open, "/skills");
      assert.equal(drawerOpen(shellAt(away, "/"), false, "/"), false);
    });

    test("keeps the drawer open while the page stays where it was opened", () => {
      const open = shellReducer(shellOf(false), { type: "open", at: "/" });
      assert.equal(shellAt(open, "/"), open);
    });

    test("keeps the viewer's rail choice", () => {
      const open = shellReducer(shellOf(true), { type: "open", at: "/" });
      assert.equal(shellAt(open, "/skills").collapsed, true);
    });
  });

  describe("when the screen grows wide", () => {
    test("forgets an open drawer, so a narrow screen later does not find it open", () => {
      const open = shellReducer(shellOf(false), { type: "open", at: "/" });
      const wide = shellReducer(open, { type: "widen" });
      assert.equal(drawerOpen(wide, false, "/"), false);
    });

    test("returns the same state when no drawer was open", () => {
      const state = shellOf(true);
      assert.equal(shellReducer(state, { type: "widen" }), state);
    });
  });

  test("takes the rail from a choice", () => {
    assert.equal(shellReducer(shellOf(false), { type: "rail", collapsed: true }).collapsed, true);
  });
});

describe("wrapFocus", () => {
  test("sends Tab from the last control to the first", () => {
    assert.equal(wrapFocus(4, 3, false), 0);
  });

  test("sends Shift+Tab from the first control to the last", () => {
    assert.equal(wrapFocus(4, 0, true), 3);
  });

  test("sends Shift+Tab from outside the controls to the last", () => {
    assert.equal(wrapFocus(4, -1, true), 3);
  });

  test("leaves Tab between the ends to the browser", () => {
    assert.equal(wrapFocus(4, 1, false), undefined);
    assert.equal(wrapFocus(4, 2, true), undefined);
  });

  test("has nowhere to send focus with no controls", () => {
    assert.equal(wrapFocus(0, -1, false), undefined);
    assert.equal(wrapFocus(0, -1, true), undefined);
  });

  test("keeps one control on itself", () => {
    assert.equal(wrapFocus(1, 0, false), 0);
    assert.equal(wrapFocus(1, 0, true), 0);
  });
});

describe("closesDrawer", () => {
  test("closes on Escape while open", () => {
    assert.equal(closesDrawer("Escape", true, false), true);
  });

  test("leaves Escape to an open menu inside the drawer", () => {
    assert.equal(closesDrawer("Escape", true, true), false);
  });

  test("ignores Escape once shut and every other key", () => {
    assert.equal(closesDrawer("Escape", false, false), false);
    assert.equal(closesDrawer("Enter", true, false), false);
  });
});

describe("initialOf", () => {
  test("takes the first letter in capitals", () => {
    assert.equal(initialOf("  ana"), "A");
    assert.equal(initialOf("émile"), "É");
  });

  test("falls back to a question mark for a name with no letter", () => {
    assert.equal(initialOf(" ~ "), "?");
    assert.equal(initialOf(""), "?");
  });
});
