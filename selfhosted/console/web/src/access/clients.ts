import type { MessageKey, Translate } from "../i18n";

/** The AI apps and ways in the picker offers. */
export type ClientId = "claude-code" | "claude-desktop" | "codex" | "opencode" | "cursor" | "vscode" | "antigravity" | "pi" | "omp" | "gemini" | "code" | "generic" | "ai";

/** One block to paste: where it goes, the file when it is one, and the exact text with the key filled in; `setup` marks a one-time install step that carries no key. */
export type Block = { where: MessageKey; path?: string; label?: string; text: string; setup?: boolean };

/** How one app connects: its name, what kind of step it takes, a one-click link when it has one, the blocks to paste, and a short note. `choose` shows one block at a time. */
export type Guide = { id: ClientId; name: string; kind: MessageKey; link?: { href: string; app: string }; blocks: Block[]; choose?: boolean; note: MessageKey };

const NAME = "versatile";

const json = (value: unknown) => JSON.stringify(value, null, 2);

const toBase64 = (text: string) => btoa(String.fromCharCode(...new TextEncoder().encode(text)));

const bearerOf = (key: string) => `Bearer ${key}`;

const headersOf = (key: string) => ({ Authorization: bearerOf(key) });

const terminal = (text: string, setup?: true): Block => ({ where: "clients.where.terminal", text, ...(setup ? { setup } : {}) });

const file = (path: string, text: string): Block => ({ where: "clients.where.file", path, text });

/** How an app connects one way: the kind of step when it is not the app's own, then the rest of its guide. */
type Way = Omit<Guide, "id" | "name" | "kind"> & { kind?: MessageKey };

/** One app of the picker with both ways in: by browser sign-in (OAuth) with the endpoint alone, and with an access key for apps without OAuth; an app lacking a way is left out of that list. */
type App = {
  id: ClientId;
  name: string | ((t: Translate) => string);
  kind: MessageKey;
  oauth?(url: string, t: Translate): Way;
  withKey?(url: string, key: string, t: Translate): Way;
};

const APPS: readonly App[] = [
  {
    id: "claude-code",
    name: "Claude Code",
    kind: "clients.kind.terminal",
    oauth: (url) => ({ blocks: [terminal(`claude mcp add --transport http ${NAME} ${url}`)], note: "clients.note.claudeCodeOAuth" }),
    withKey: (url, key) => ({ blocks: [terminal(`claude mcp add --transport http ${NAME} ${url} --header "Authorization: ${bearerOf(key)}"`)], note: "clients.note.claudeCode" }),
  },
  {
    id: "claude-desktop",
    name: "Claude Desktop",
    kind: "clients.kind.config",
    oauth: (url) => ({ blocks: [file("claude_desktop_config.json", json({ mcpServers: { [NAME]: { command: "npx", args: ["-y", "mcp-remote", url] } } }))], note: "clients.note.claudeDesktopOAuth" }),
    withKey: (url, key) => ({
      blocks: [file("claude_desktop_config.json", json({ mcpServers: { [NAME]: { command: "npx", args: ["-y", "mcp-remote", url, "--header", "Authorization:${AUTH_HEADER}"], env: { AUTH_HEADER: bearerOf(key) } } } }))],
      note: "clients.note.claudeDesktop",
    }),
  },
  {
    id: "codex",
    name: "Codex",
    kind: "clients.kind.terminal",
    oauth: (url) => ({ blocks: [terminal(`codex mcp add ${NAME} --url ${url}`)], note: "clients.note.codexOAuth" }),
    withKey: (url, key) => ({
      kind: "clients.kind.config",
      blocks: [file("~/.codex/config.toml", `[mcp_servers.${NAME}]\nurl = "${url}"\nhttp_headers = { "Authorization" = "${bearerOf(key)}" }`)],
      note: "clients.note.codex",
    }),
  },
  {
    id: "opencode",
    name: "OpenCode",
    kind: "clients.kind.config",
    oauth: (url) => ({ blocks: [file("opencode.json", json({ $schema: "https://opencode.ai/config.json", mcp: { servers: { [NAME]: { type: "remote", url } } } }))], note: "clients.note.opencodeOAuth" }),
    withKey: (url, key) => ({
      blocks: [file("opencode.json", json({ $schema: "https://opencode.ai/config.json", mcp: { [NAME]: { type: "remote", url, enabled: true, oauth: false, headers: headersOf(key) } } }))],
      note: "clients.note.opencode",
    }),
  },
  {
    id: "cursor",
    name: "Cursor",
    kind: "clients.kind.link",
    oauth: (url) => ({
      link: { app: "Cursor", href: `cursor://anysphere.cursor-deeplink/mcp/install?name=${NAME}&config=${encodeURIComponent(toBase64(JSON.stringify({ url })))}` },
      blocks: [file("~/.cursor/mcp.json", json({ mcpServers: { [NAME]: { url } } }))],
      note: "clients.note.cursorOAuth",
    }),
    withKey: (url, key) => ({
      link: { app: "Cursor", href: `cursor://anysphere.cursor-deeplink/mcp/install?name=${NAME}&config=${encodeURIComponent(toBase64(JSON.stringify({ url, headers: headersOf(key) })))}` },
      blocks: [file("~/.cursor/mcp.json", json({ mcpServers: { [NAME]: { url, headers: headersOf(key) } } }))],
      note: "clients.note.cursor",
    }),
  },
  {
    id: "vscode",
    name: "VS Code",
    kind: "clients.kind.link",
    oauth: (url) => ({
      link: { app: "VS Code", href: `vscode:mcp/install?${encodeURIComponent(JSON.stringify({ name: NAME, type: "http", url }))}` },
      blocks: [file(".vscode/mcp.json", json({ servers: { [NAME]: { type: "http", url } } }))],
      note: "clients.note.vscodeOAuth",
    }),
    withKey: (url, key) => ({
      link: { app: "VS Code", href: `vscode:mcp/install?${encodeURIComponent(JSON.stringify({ name: NAME, type: "http", url, headers: headersOf(key) }))}` },
      blocks: [file(".vscode/mcp.json", json({ servers: { [NAME]: { type: "http", url, headers: headersOf(key) } } }))],
      note: "clients.note.vscode",
    }),
  },
  {
    id: "antigravity",
    name: "Antigravity",
    kind: "clients.kind.config",
    oauth: (url) => ({ blocks: [file("~/.gemini/config/mcp_config.json", json({ mcpServers: { [NAME]: { serverUrl: url } } }))], note: "clients.note.antigravityOAuth" }),
    withKey: (url, key) => ({ blocks: [file("~/.gemini/config/mcp_config.json", json({ mcpServers: { [NAME]: { serverUrl: url, headers: headersOf(key) } } }))], note: "clients.note.antigravity" }),
  },
  {
    id: "pi",
    name: "Pi",
    kind: "clients.kind.terminal",
    oauth: (url) => ({ blocks: [terminal("pi install npm:pi-mcp-adapter", true), file("~/.pi/agent/mcp.json", json({ mcpServers: { [NAME]: { url } } }))], note: "clients.note.piOAuth" }),
    withKey: (url, key) => ({ blocks: [terminal("pi install npm:pi-mcp-adapter", true), file("~/.pi/agent/mcp.json", json({ mcpServers: { [NAME]: { url, headers: headersOf(key) } } }))], note: "clients.note.pi" }),
  },
  {
    id: "gemini",
    name: "Gemini CLI",
    kind: "clients.kind.terminal",
    oauth: (url) => ({ blocks: [terminal(`gemini mcp add -s user -t http ${NAME} ${url}`)], note: "clients.note.geminiOAuth" }),
    withKey: (url, key) => ({ blocks: [file("~/.gemini/settings.json", json({ mcpServers: { [NAME]: { httpUrl: url, headers: headersOf(key) } } }))], note: "clients.note.gemini" }),
  },
  {
    id: "omp",
    name: "oh-my-pi",
    kind: "clients.kind.config",
    oauth: (url) => ({ blocks: [file("~/.omp/agent/mcp.json", json({ mcpServers: { [NAME]: { type: "http", url } } }))], note: "clients.note.ompOAuth" }),
    withKey: (url, key) => ({ blocks: [file("~/.omp/agent/mcp.json", json({ mcpServers: { [NAME]: { type: "http", url, headers: headersOf(key) } } }))], note: "clients.note.omp" }),
  },
  {
    id: "code",
    name: (t) => t("clients.code"),
    kind: "clients.kind.sdk",
    withKey: (url, key) => ({
      choose: true,
      blocks: [
        {
          where: "clients.where.code",
          label: "Python",
          text: [
            '# pip install "mcp>=2.3"',
            "import anyio",
            "import httpx2",
            "from mcp import Client",
            "from mcp.client.streamable_http import streamable_http_client",
            "",
            "",
            "async def main():",
            `    async with httpx2.AsyncClient(headers={"Authorization": "${bearerOf(key)}"}, timeout=httpx2.Timeout(30.0, read=300.0)) as http:`,
            `        async with Client(streamable_http_client("${url}", http_client=http)) as client:`,
            "            tools = await client.list_tools()",
            "            print([tool.name for tool in tools.tools])",
            "",
            "",
            "anyio.run(main)",
          ].join("\n"),
        },
        {
          where: "clients.where.code",
          label: "TypeScript",
          text: [
            "// npm install @modelcontextprotocol/sdk",
            'import { Client } from "@modelcontextprotocol/sdk/client/index.js";',
            'import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";',
            "",
            `const transport = new StreamableHTTPClientTransport(new URL("${url}"), {`,
            `  requestInit: { headers: { Authorization: "${bearerOf(key)}" } },`,
            "});",
            'const client = new Client({ name: "my-app", version: "1.0.0" });',
            "await client.connect(transport);",
            "console.log((await client.listTools()).tools.map((tool) => tool.name));",
          ].join("\n"),
        },
        {
          where: "clients.where.code",
          label: "PHP / Laravel",
          text: [
            "<?php",
            "// composer require mcp/sdk php-http/guzzle7-adapter",
            "use Mcp\\Client\\Client;",
            "use Mcp\\Client\\Transport\\HttpTransport;",
            "",
            "$client = Client::builder()->setClientInfo('my-app', '1.0.0')->build();",
            `$client->connect(new HttpTransport(endpoint: '${url}', headers: ['Authorization' => '${bearerOf(key)}']));`,
            "$tools = $client->listTools();",
          ].join("\n"),
        },
      ],
      note: "clients.note.code",
    }),
  },
  {
    id: "generic",
    name: (t) => t("clients.generic"),
    kind: "clients.kind.any",
    oauth: (url) => ({ blocks: [{ where: "clients.where.values", text: `URL        ${url}\nTransport  Streamable HTTP\nSign-in    OAuth, in your browser` }], note: "clients.note.genericOAuth" }),
    withKey: (url, key) => ({
      blocks: [
        { where: "clients.where.values", text: `URL        ${url}\nTransport  Streamable HTTP\nHeader     Authorization: ${bearerOf(key)}` },
        { where: "clients.where.vsh", text: `export CN_ENDPOINT=${url}\nexport CN_AGENT_TOKEN=${key}\nvsh tools list` },
      ],
      note: "clients.note.generic",
    }),
  },
  {
    id: "ai",
    name: (t) => t("clients.ai"),
    kind: "clients.kind.prompt",
    withKey: (url, key, t) => ({ blocks: [{ where: "clients.where.ai", text: t("clients.prompt", { name: NAME, url, header: `Authorization: ${bearerOf(key)}` }) }], note: "clients.note.ai" }),
  },
];

const guideOf = (app: App, way: Way, t: Translate): Guide => ({ ...way, id: app.id, name: typeof app.name === "string" ? app.name : app.name(t), kind: way.kind ?? app.kind });

/** The apps most people connect, first in the picker in this order; the others wait behind More apps. */
export const COMMON_APPS: readonly ClientId[] = ["claude-code", "codex", "cursor", "vscode", "claude-desktop"];

/** Splits guides into the common apps, in the order of `COMMON_APPS`, and the others in their own order. */
export function byUse(all: readonly Guide[]): { common: Guide[]; more: Guide[] } {
  return { common: COMMON_APPS.flatMap((id) => all.filter((g) => g.id === id)), more: all.filter((g) => !COMMON_APPS.includes(g.id)) };
}

/** Every app's guide for the endpoint alone: each app signs in through the browser (OAuth), so nothing secret is pasted. */
export function oauthGuides(url: string, t: Translate): Guide[] {
  return APPS.flatMap((app) => (app.oauth ? [guideOf(app, app.oauth(url, t), t)] : []));
}

/** Every app's guide for one endpoint and key, the key already in each block, for apps without OAuth. */
export function guides(url: string, key: string, t: Translate): Guide[] {
  return APPS.flatMap((app) => (app.withKey ? [guideOf(app, app.withKey(url, key, t), t)] : []));
}

