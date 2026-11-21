import { createRequire } from "node:module";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const opentype = require("opentype.js");
const here = dirname(fileURLToPath(import.meta.url));
const out = join(here, "..");

const font = (pkg, file) => {
  const buf = readFileSync(join(here, "node_modules/@fontsource", pkg, "files", file));
  return opentype.parse(buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength));
};
const serif = font("instrument-serif", "instrument-serif-latin-400-normal.woff");
const italic = font("instrument-serif", "instrument-serif-latin-400-italic.woff");
const mono = font("jetbrains-mono", "jetbrains-mono-latin-400-normal.woff");
const monoMedium = font("jetbrains-mono", "jetbrains-mono-latin-500-normal.woff");

const INK = "#0e1116";
const PAPER = "#f5f5f2";
const MUTED = "#8a919c";
const LIME = "#c6f432";
const LINE = "#f5f5f2";

const width = (f, text, size) => f.getAdvanceWidth(text, size);
const r = (n) => Math.round(n * 10) / 10;

let glyphs;
const reset = () => {
  glyphs = new Map();
};
const glyphId = (f, glyph, size) => {
  const key = `${f.names.fullName.en}|${size}|${glyph.index}`;
  if (!glyphs.has(key)) {
    glyphs.set(key, { id: `g${glyphs.size}`, d: glyph.getPath(0, 0, size).toPathData(size >= 30 ? 0 : 1) });
  }
  return glyphs.get(key);
};
const defs = () => [...glyphs.values()].filter((g) => g.d).map((g) => `<path id="${g.id}" d="${g.d}"/>`).join("");

const text = (f, s, x, y, size, fill) => {
  const uses = [];
  f.forEachGlyph(s, x, y, size, { kerning: true }, (glyph, gx, gy) => {
    const g = glyphId(f, glyph, size);
    if (g.d) uses.push(`<use href="#${g.id}" x="${r(gx)}" y="${r(gy)}"/>`);
  });
  return `<g fill="${fill}">${uses.join("")}</g>`;
};

const runs = (parts, x, y) => {
  let cx = x;
  return parts
    .map(([f, s, size, fill]) => {
      const p = text(f, s, cx, y, size, fill);
      cx += width(f, s, size);
      return p;
    })
    .join("");
};

const mark = (x, y, s) =>
  `<g transform="translate(${x} ${y}) scale(${s})"><rect width="1024" height="1024" rx="160" fill="#151a22"/><rect width="1024" height="190" fill="#1a1f27"/><rect y="190" width="1024" height="4" fill="#2a313c"/><circle cx="320" cy="96" r="22" fill="#3a424e"/><circle cx="392" cy="96" r="22" fill="#3a424e"/><circle cx="464" cy="96" r="22" fill="${LIME}"/><path d="M158 467L310 541V581L158 655V617L287 561L158 505Z" fill="#3a424e"/><path d="M376 432H441L521 688H532L612 432H677L578 732H477Z" fill="${PAPER}"/><rect x="716" y="694.5" width="149" height="37.5" rx="9" fill="${LIME}"/></g>`;

const frame = (w, h, label, body) => {
  const grid = [];
  for (let x = 64; x < w; x += 64) grid.push(`M${x} 0V${h}`);
  for (let y = 64; y < h; y += 64) grid.push(`M0 ${y}H${w}`);
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}" role="img" aria-label="${label}"><title>${label}</title><defs><radialGradient id="l" cx="0.5" cy="0" r="0.75"><stop offset="0" stop-color="#fff" stop-opacity="0.09"/><stop offset="1" stop-color="#fff" stop-opacity="0"/></radialGradient><clipPath id="c"><rect width="${w}" height="${h}" rx="16"/></clipPath>${defs()}</defs><g clip-path="url(#c)"><rect width="${w}" height="${h}" fill="${INK}"/><path d="${grid.join("")}" stroke="${LINE}" stroke-opacity="0.04" fill="none"/><rect width="${w}" height="${h}" fill="url(#l)"/>${body}</g><rect x="0.5" y="0.5" width="${w - 1}" height="${h - 1}" rx="15.5" fill="none" stroke="${LINE}" stroke-opacity="0.18"/></svg>\n`;
};

const wordmark = (x, y) => runs([[mono, "versatile", 18, PAPER], [mono, ".sh", 18, LIME]], x, y);

const hero = () => {
  reset();
  const w = 1280;
  const h = 480;
  const tags = ["MCP gateway", "Connector catalog", "Permissions & approvals", "Self-hosted", "Headless engine"];
  let tx = 72;
  const pills = tags
    .map((t) => {
      const tw = width(mono, t, 15);
      const p = `<rect x="${tx}" y="388" width="${(tw + 32).toFixed(1)}" height="36" rx="18" fill="none" stroke="${LINE}" stroke-opacity="0.18"/>${text(mono, t, tx + 16, 411, 15, MUTED)}`;
      tx += tw + 44;
      return p;
    })
    .join("");
  const body = [
    mark(72, 56, 48 / 1024),
    wordmark(136, 87),
    text(mono, "open source  ·  GPL-3.0-or-later", w - 72 - width(mono, "open source  ·  GPL-3.0-or-later", 14), 86, 14, MUTED),
    text(serif, "connector-engine", 68, 214, 112, PAPER),
    runs([[serif, "Every MCP connector, ", 46, PAPER], [italic, "one gateway you control", 46, LIME], [serif, ".", 46, PAPER]], 72, 284),
    text(serif, "You decide what each AI app may run, and see every call it makes.", 72, 334, 34, MUTED),
    `<path d="M72 364H${w - 72}" stroke="${LINE}" stroke-opacity="0.1"/>`,
    pills,
  ].join("");
  return frame(w, h, "connector-engine: every MCP connector, one gateway you control. You decide what each AI app may run, and see every call it makes.", body);
};

const section = (kicker, plain, accent) => {
  reset();
  const w = 1280;
  const h = 150;
  const label = `${plain}${accent}`.trim();
  const body = [
    mark(56, 47, 56 / 1024),
    text(mono, `connector-engine  /  ${kicker}`, 136, 58, 14, MUTED),
    runs([[serif, plain, 54, PAPER], [italic, accent, 54, LIME]], 134, 114),
  ].join("");
  return frame(w, h, label, body);
};

const diagram = () => {
  reset();
  const w = 1200;
  const h = 570;
  const top = 48;
  const boxH = 330;
  const side = 248;
  const gap = 88;
  const mid = 416;
  const pad = 24;
  const [apps, gw, conn] = [56, 56 + side + gap, 56 + side + gap + mid + gap];
  const fit = (f, s, size, max) => {
    if (width(f, s, size) > max) throw new Error(`diagram: "${s}" overflows its box`);
    return s;
  };
  const box = (x, y, bw, bh) =>
    `<rect x="${x}" y="${y}" width="${bw}" height="${bh}" rx="12" fill="#12161d" stroke="${LINE}" stroke-opacity="0.18"/>`;
  const card = (x, bw, title, sub, rows) =>
    [
      box(x, top, bw, boxH),
      text(monoMedium, fit(monoMedium, title, 26, bw - 2 * pad), x + pad, top + 50, 26, PAPER),
      text(monoMedium, fit(monoMedium, sub, 16, bw - 2 * pad), x + pad, top + 80, 16, MUTED),
      `<path d="M${x + pad} ${top + 104}H${x + bw - pad}" stroke="${LINE}" stroke-opacity="0.1"/>`,
      ...rows.map(([s, fill], i) => text(monoMedium, fit(monoMedium, s, 18, bw - 2 * pad), x + pad, top + 146 + i * 38, 18, fill)),
    ].join("");
  const arrow = (x1, x2, y, color, label) =>
    `<path d="M${x1 + 12} ${y}H${x2 - 14}" stroke="${color}" stroke-width="1.5"/><path d="M${x2 - 22} ${y - 5}L${x2 - 12} ${y}L${x2 - 22} ${y + 5}Z" fill="${color}"/>` +
    text(monoMedium, label, (x1 + x2) / 2 - width(monoMedium, label, 16) / 2, y - 14, 16, color);
  const modes = [
    ["off", "hidden from the app"],
    ["on", "runs, credential injected"],
    ["ask", "waits for your approval"],
  ]
    .map(([m, d], i) => {
      const y = top + 146 + i * 38;
      return `<rect x="${gw + pad}" y="${y - 20}" width="56" height="28" rx="14" fill="none" stroke="${LINE}" stroke-opacity="0.25"/>${text(monoMedium, m, gw + pad + 28 - width(monoMedium, m, 16) / 2, y - 1, 16, PAPER)}${text(monoMedium, fit(monoMedium, d, 18, mid - 2 * pad - 72), gw + pad + 72, y, 18, PAPER)}`;
    })
    .join("");
  let px = gw + pad;
  const pills = ["approvals", "activity", "skills"]
    .map((t) => {
      const tw = width(monoMedium, t, 16);
      const p = `<rect x="${px}" y="${top + 268}" width="${r(tw + 28)}" height="32" rx="16" fill="none" stroke="${LINE}" stroke-opacity="0.18"/>${text(monoMedium, t, px + 14, top + 289, 16, MUTED)}`;
      px += tw + 38;
      return p;
    })
    .join("");
  const cy = top + boxH + 48;
  const cx = gw + mid / 2;
  const body = [
    card(apps, side, "AI apps", "OAuth 2.1 sign-in", [["Claude Code", PAPER], ["Cursor", PAPER], ["Codex", PAPER], ["VS Code", PAPER], ["any MCP client", MUTED]]),
    card(gw, mid, "connector-engine", "/mcp · per-function permissions", []),
    modes,
    `<path d="M${gw + pad} ${top + 250}H${gw + mid - pad}" stroke="${LINE}" stroke-opacity="0.1"/>`,
    pills,
    card(conn, side, "Connectors", "pinned by hash", [["remote MCP", PAPER], ["local MCP", PAPER], ["REST (OpenAPI)", PAPER], ["gRPC (reflection)", PAPER], ["import by URL", MUTED]]),
    arrow(apps + side, gw, top + boxH / 2, MUTED, "MCP"),
    arrow(gw + mid, conn, top + boxH / 2, LIME, "allowed"),
    `<path d="M${cx} ${top + boxH + 8}V${cy - 14}" stroke="${MUTED}" stroke-width="1.5" stroke-dasharray="4 4"/><path d="M${cx - 5} ${cy - 18}L${cx} ${cy - 8}L${cx + 5} ${cy - 18}Z" fill="${MUTED}"/>`,
    text(monoMedium, "ask", cx + 14, top + boxH + 30, 16, MUTED),
    box(gw, cy, mid, 96),
    `<circle cx="${gw + pad + 5}" cy="${cy + 35}" r="5" fill="${LIME}"/>`,
    text(monoMedium, fit(monoMedium, "Self-hosted console", 26, mid - 2 * pad - 22), gw + pad + 22, cy + 44, 26, PAPER),
    text(monoMedium, fit(monoMedium, "set permissions · answer approvals", 16, mid - 2 * pad - 22), gw + pad + 22, cy + 76, 16, MUTED),
  ].join("");
  return frame(
    w,
    h,
    "How a call flows. AI apps such as Claude Code, Cursor and Codex call connector-engine over MCP. The gateway checks each function's permission: off hides it, on runs it with the stored credential, ask waits for your approval in the self-hosted console. Allowed calls reach remote and local MCP servers, REST and gRPC connectors. The gateway also keeps approvals, activity and skills.",
    body,
  );
};

const SECTIONS = [
  ["readme-install", "start here", "Install ", "in one line"],
  ["readme-use", "start here", "Use it with ", "any AI app"],
  ["readme-how", "overview", "How it ", "works"],
  ["readme-features", "overview", "What you ", "get"],
  ["readme-docs", "overview", "Read the ", "docs"],
  ["docs", "documentation", "The ", "documentation"],
  ["install", "getting started", "Install ", "in one line"],
  ["first-run", "getting started", "Your first ", "run"],
  ["connect-an-app", "getting started", "Connect an ", "AI app"],
  ["import-a-connector", "getting started", "Import a ", "connector"],
  ["connectors", "using it", "Connectors, ", "pinned"],
  ["permissions", "using it", "Functions and ", "permissions"],
  ["skills", "using it", "Skills, ", "granted"],
  ["approvals", "using it", "Approvals and ", "notifications"],
  ["activity", "using it", "Every call, ", "on record"],
  ["account-backup", "using it", "Account and ", "backup"],
  ["person", "self-hosted", "The person ", "profile"],
  ["keychain", "self-hosted", "Keys in the ", "keychain"],
  ["approval-surfaces", "self-hosted", "Approval ", "surfaces"],
  ["updating", "self-hosted", "Stay ", "current"],
  ["uninstalling", "self-hosted", "Uninstall ", "cleanly"],
  ["troubleshooting", "self-hosted", "When something ", "fails"],
  ["headless", "enterprise", "The headless ", "engine"],
  ["oidc", "enterprise", "Bring your own ", "identity"],
  ["postgres", "enterprise", "Postgres and ", "tenants"],
  ["scaling", "enterprise", "Scaling and ", "replicas"],
  ["security-model", "enterprise", "Security ", "model"],
  ["cloud", "enterprise", "Deploy to the ", "cloud"],
  ["vps", "self-hosted", "Run it on a ", "VPS"],
  ["architecture", "enterprise", "How it is ", "built"],
  ["config", "reference", "Configuration ", "keys"],
  ["vsh", "reference", "The command line, ", "vsh"],
  ["api", "reference", "Operator ", "API"],
  ["events", "reference", "Events, as ", "CloudEvents"],
  ["contributing", "project", "Contributing ", "a change"],
  ["security", "project", "Report a ", "vulnerability"],
  ["releasing", "project", "Ship a ", "release"],
];

writeFileSync(join(out, "banner-hero.svg"), hero());
writeFileSync(join(out, "diagram-flow.svg"), diagram());
for (const [slug, kicker, plain, accent] of SECTIONS) {
  writeFileSync(join(out, `section-${slug}.svg`), section(kicker, plain, accent));
}
console.log(`wrote banner-hero.svg, diagram-flow.svg and ${SECTIONS.length} section banners to ${out}`);
