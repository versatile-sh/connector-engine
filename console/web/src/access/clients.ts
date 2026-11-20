import type { MessageKey, Translate } from "../i18n";

/** The AI apps and ways in the picker offers. */
export type ClientId = "claude-code" | "claude-desktop" | "codex" | "opencode" | "cursor" | "vscode" | "antigravity" | "pi" | "code" | "generic" | "ai";

/** One block to paste: where it goes, the file when it is one, and the exact text with the key filled in; `setup` marks a one-time install step that carries no key. */
export type Block = { where: MessageKey; path?: string; label?: string; text: string; setup?: boolean };

/** How one app connects: its name, what kind of step it takes, a one-click link when it has one, the blocks to paste, and a short note. `choose` shows one block at a time. */
export type Guide = { id: ClientId; name: string; kind: MessageKey; link?: { href: string; app: string }; blocks: Block[]; choose?: boolean; note: MessageKey };

const NAME = "versatile";

const json = (value: unknown) => JSON.stringify(value, null, 2);

const toBase64 = (text: string) => btoa(String.fromCharCode(...new TextEncoder().encode(text)));

/** Every app's guide for the endpoint alone: each app signs in through the browser (OAuth), so nothing secret is pasted. */
export function oauthGuides(url: string, t: Translate): Guide[] {
  return [
    {
      id: "claude-code",
      name: "Claude Code",
      kind: "clients.kind.terminal",
      blocks: [{ where: "clients.where.terminal", text: `claude mcp add --transport http ${NAME} ${url}` }],
      note: "clients.note.claudeCodeOAuth",
    },
    {
      id: "claude-desktop",
      name: "Claude Desktop",
      kind: "clients.kind.config",
      blocks: [{ where: "clients.where.file", path: "claude_desktop_config.json", text: json({ mcpServers: { [NAME]: { command: "npx", args: ["-y", "mcp-remote", url] } } }) }],
      note: "clients.note.claudeDesktopOAuth",
    },
    {
      id: "codex",
      name: "Codex",
      kind: "clients.kind.terminal",
      blocks: [{ where: "clients.where.terminal", text: `codex mcp add ${NAME} --url ${url}` }],
      note: "clients.note.codexOAuth",
    },
    {
      id: "opencode",
      name: "OpenCode",
      kind: "clients.kind.config",
      blocks: [{ where: "clients.where.file", path: "opencode.json", text: json({ $schema: "https://opencode.ai/config.json", mcp: { servers: { [NAME]: { type: "remote", url } } } }) }],
      note: "clients.note.opencodeOAuth",
    },
    {
      id: "cursor",
      name: "Cursor",
      kind: "clients.kind.link",
      link: { app: "Cursor", href: `cursor://anysphere.cursor-deeplink/mcp/install?name=${NAME}&config=${encodeURIComponent(toBase64(JSON.stringify({ url })))}` },
      blocks: [{ where: "clients.where.file", path: "~/.cursor/mcp.json", text: json({ mcpServers: { [NAME]: { url } } }) }],
      note: "clients.note.cursorOAuth",
    },
    {
      id: "vscode",
      name: "VS Code",
      kind: "clients.kind.link",
      link: { app: "VS Code", href: `vscode:mcp/install?${encodeURIComponent(JSON.stringify({ name: NAME, type: "http", url }))}` },
      blocks: [{ where: "clients.where.file", path: ".vscode/mcp.json", text: json({ servers: { [NAME]: { type: "http", url } } }) }],
      note: "clients.note.vscodeOAuth",
    },
    {
      id: "antigravity",
      name: "Antigravity",
      kind: "clients.kind.config",
      blocks: [{ where: "clients.where.file", path: "~/.gemini/config/mcp_config.json", text: json({ mcpServers: { [NAME]: { serverUrl: url } } }) }],
      note: "clients.note.antigravityOAuth",
    },
    {
      id: "pi",
      name: "Pi",
      kind: "clients.kind.terminal",
      blocks: [
        { where: "clients.where.terminal", text: "pi install npm:pi-mcp-adapter", setup: true },
        { where: "clients.where.file", path: "~/.pi/agent/mcp.json", text: json({ mcpServers: { [NAME]: { url } } }) },
      ],
      note: "clients.note.piOAuth",
    },
    {
      id: "generic",
      name: t("clients.generic"),
      kind: "clients.kind.any",
      blocks: [{ where: "clients.where.values", text: `URL        ${url}\nTransport  Streamable HTTP\nSign-in    OAuth, in your browser` }],
      note: "clients.note.genericOAuth",
    },
  ];
}

/** Every app's guide for one endpoint and key, the key already in each block, for apps without OAuth. */
export function guides(url: string, key: string, t: Translate): Guide[] {
  const bearer = `Bearer ${key}`;
  const headers = { Authorization: bearer };
  return [
    {
      id: "claude-code",
      name: "Claude Code",
      kind: "clients.kind.terminal",
      blocks: [{ where: "clients.where.terminal", text: `claude mcp add --transport http ${NAME} ${url} --header "Authorization: ${bearer}"` }],
      note: "clients.note.claudeCode",
    },
    {
      id: "claude-desktop",
      name: "Claude Desktop",
      kind: "clients.kind.config",
      blocks: [
        {
          where: "clients.where.file",
          path: "claude_desktop_config.json",
          text: json({ mcpServers: { [NAME]: { command: "npx", args: ["-y", "mcp-remote", url, "--header", "Authorization:${AUTH_HEADER}"], env: { AUTH_HEADER: bearer } } } }),
        },
      ],
      note: "clients.note.claudeDesktop",
    },
    {
      id: "codex",
      name: "Codex",
      kind: "clients.kind.config",
      blocks: [{ where: "clients.where.file", path: "~/.codex/config.toml", text: `[mcp_servers.${NAME}]\nurl = "${url}"\nhttp_headers = { "Authorization" = "${bearer}" }` }],
      note: "clients.note.codex",
    },
    {
      id: "opencode",
      name: "OpenCode",
      kind: "clients.kind.config",
      blocks: [
        {
          where: "clients.where.file",
          path: "opencode.json",
          text: json({ $schema: "https://opencode.ai/config.json", mcp: { [NAME]: { type: "remote", url, enabled: true, oauth: false, headers } } }),
        },
      ],
      note: "clients.note.opencode",
    },
    {
      id: "cursor",
      name: "Cursor",
      kind: "clients.kind.link",
      link: { app: "Cursor", href: `cursor://anysphere.cursor-deeplink/mcp/install?name=${NAME}&config=${encodeURIComponent(toBase64(JSON.stringify({ url, headers })))}` },
      blocks: [{ where: "clients.where.file", path: "~/.cursor/mcp.json", text: json({ mcpServers: { [NAME]: { url, headers } } }) }],
      note: "clients.note.cursor",
    },
    {
      id: "vscode",
      name: "VS Code",
      kind: "clients.kind.link",
      link: { app: "VS Code", href: `vscode:mcp/install?${encodeURIComponent(JSON.stringify({ name: NAME, type: "http", url, headers }))}` },
      blocks: [{ where: "clients.where.file", path: ".vscode/mcp.json", text: json({ servers: { [NAME]: { type: "http", url, headers } } }) }],
      note: "clients.note.vscode",
    },
    {
      id: "antigravity",
      name: "Antigravity",
      kind: "clients.kind.config",
      blocks: [{ where: "clients.where.file", path: "~/.gemini/config/mcp_config.json", text: json({ mcpServers: { [NAME]: { serverUrl: url, headers } } }) }],
      note: "clients.note.antigravity",
    },
    {
      id: "pi",
      name: "Pi",
      kind: "clients.kind.terminal",
      blocks: [
        { where: "clients.where.terminal", text: "pi install npm:pi-mcp-adapter", setup: true },
        { where: "clients.where.file", path: "~/.pi/agent/mcp.json", text: json({ mcpServers: { [NAME]: { url, headers } } }) },
      ],
      note: "clients.note.pi",
    },
    {
      id: "code",
      name: t("clients.code"),
      kind: "clients.kind.sdk",
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
            `    async with httpx2.AsyncClient(headers={"Authorization": "${bearer}"}, timeout=httpx2.Timeout(30.0, read=300.0)) as http:`,
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
            `  requestInit: { headers: { Authorization: "${bearer}" } },`,
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
            `$client->connect(new HttpTransport(endpoint: '${url}', headers: ['Authorization' => '${bearer}']));`,
            "$tools = $client->listTools();",
          ].join("\n"),
        },
      ],
      note: "clients.note.code",
    },
    {
      id: "generic",
      name: t("clients.generic"),
      kind: "clients.kind.any",
      blocks: [
        { where: "clients.where.values", text: `URL        ${url}\nTransport  Streamable HTTP\nHeader     Authorization: ${bearer}` },
        { where: "clients.where.cnctl", text: `export CN_ENDPOINT=${url}\nexport CN_AGENT_TOKEN=${key}\ncnctl tools list` },
      ],
      note: "clients.note.generic",
    },
    {
      id: "ai",
      name: t("clients.ai"),
      kind: "clients.kind.prompt",
      blocks: [{ where: "clients.where.ai", text: t("clients.prompt", { name: NAME, url, header: `Authorization: ${bearer}` }) }],
      note: "clients.note.ai",
    },
  ];
}
