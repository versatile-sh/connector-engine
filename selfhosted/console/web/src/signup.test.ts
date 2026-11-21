import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { createElement } from "react";
import type { Session } from "./api.ts";
import { App } from "./App.tsx";
import { Auth } from "./Auth.tsx";
import { TEAM_ADMIN, TEAM_OWNER, pageData } from "./testing/fixtures.ts";
import { renderMarkup, type Seed } from "./testing/render.ts";

const MCP = "http://127.0.0.1:7465/mcp";
const SIGNED_OUT = { owner_exists: true, signed_in: false, mcp_url: MCP } as const satisfies Session;
const page = (session: Session, path: string, extra: Seed["data"] = []) => renderMarkup(createElement(App), { path, data: [...pageData(session), ...extra] });
const login = (signupOpen: boolean, startCreating = false) => renderMarkup(createElement(Auth, { mode: "login", endpoint: MCP, signupOpen, startCreating }));

describe("the sign-in screen", () => {
  describe("with sign-up closed", () => {
    const html = page(SIGNED_OUT, "/");

    test("says whose account it is", () => {
      assert.ok(html.includes("Your account on this console.") && !html.includes("The owner account"));
    });

    test("points to an invite link and offers no Create account", () => {
      assert.ok(html.includes("No account? Ask an Owner or Admin for an invite link.") && !html.includes("Create account"));
    });
  });

  describe("with sign-up open", () => {
    const html = page({ ...SIGNED_OUT, signup_open: true }, "/");

    test("offers Create account under Sign in and no hint", () => {
      assert.ok(html.includes("Create account") && !html.includes("No account?"));
    });

    test("keeps the sign-in form first", () => {
      assert.ok(html.indexOf(">Sign in<") < html.indexOf("Create account") && !html.includes("Back to sign in"));
    });
  });

  describe("while creating an account", () => {
    const html = login(true, true);

    test("asks for a username, a password and its confirmation", () => {
      assert.ok(html.includes("Create your account") && html.includes("New password again") && html.includes("At least 10 characters."));
    });

    test("offers the way back to sign in", () => {
      assert.ok(html.includes("Back to sign in"));
    });
  });

  describe("on the first visit", () => {
    test("still creates the Owner and offers no sign-up", () => {
      const html = renderMarkup(createElement(App), { data: [[["session"], { owner_exists: false, signed_in: false, mcp_url: MCP, signup_open: true }]] });
      assert.ok(html.includes("Create the owner account") && !html.includes("Back to sign in") && !html.includes("No account?"));
    });
  });
});

describe("the People tab sign-up setting", () => {
  test("shows an Owner the switch with its explanation", () => {
    const html = page(TEAM_OWNER, "/account/people");
    assert.ok(html.includes('role="switch"') && html.includes("Allow sign-up") && !html.includes("New accounts are"));
  });

  test("shows the role and the internet note once on", () => {
    const html = page(TEAM_OWNER, "/account/people", [[["people"], { ...pageDataPeople(), signup: { enabled: true, role: "member" } }]]);
    assert.ok(html.includes("New accounts are") && html.includes("Leave this off on a console reachable from the internet."));
  });

  test("hides it from an Admin", () => {
    assert.ok(!page(TEAM_ADMIN, "/account/people").includes("Allow sign-up"));
  });
});

function pageDataPeople() {
  const found = pageData(TEAM_OWNER).find(([key]) => key[0] === "people");
  return found?.[1] as Record<string, unknown>;
}
