![Skills, granted](../assets/section-skills.svg)

# Skills

A skill is a folder of instructions and files an AI app can load on demand. The engine serves skills like connectors: pinned, per key, off, on or ask.

## Add a skill

1. Open an install link: `http://127.0.0.1:7465/import?listing=<https URL of a skill catalog entry>`. The versatile.sh skills library uses these links.
2. Review the licence, the source at its pinned commit, the file list with sizes, and a warning when the skill carries scripts, and a table of which AI apps can use it.
3. Confirm, then set it per key on the **Skills** page. Keys made later start off.

A catalog entry may carry an `icon`, an https address. The console shows it on the review, the **Skills** page, the dashboard and the activity, served through the console so your browser never contacts the icon's host; without one, or when it does not load, the skill shows the first letter of its title. An address that is not https, carries credentials or is longer than 2048 bytes is ignored, and only PNG, JPEG, GIF, WebP or ICO images up to 512 KiB are served. A skill added before its entry had an icon shows the letter until you add it again.

On confirm, each file is fetched over public `https`, checked against its sha256 and pinned to `source.commit`. Refused: a branch name instead of a commit, `..`, an absolute path, or more than 256 files, 1 MiB a file or 4 MiB a skill.

## Which apps can use a skill

A skill that runs bundled scripts does nothing useful in an app with no shell. The review and the skill's page list a "Requires" row (tools, runtimes, network, MCP servers) and one row per app: **Works**, **Instructions only** or **Does not run**, with a one-line reason. A catalog entry's own `requires` and `compat` are kept; otherwise the console derives them from the skill's files by the catalog's rules. Every value is derived from the files and a reviewed table of each app's abilities. Nothing is run or tested.

When an access key belongs to an app that cannot fully use the skill, the skill's page and the review warn, for example "Gemini CLI can load these instructions but cannot run this skill's scripts". An import is never blocked. The console knows a key's app from the OAuth client name or from the **App** picked when the key is made on Access.

## How apps see skills

- Two tools, `skill_list` and `skill_load`, and a short index of the skills in the server instructions.
- The gateway serves skill files as text and never runs them.
- A skill without a redistributable licence is `metadata_only`: `skill_load` answers with its source instead of its files.
- With `gateway.skills: tools+prompts` (the `person` profile), every skill is also a prompt, so an app can offer it as a slash command.

## Export skills to an app's folder

Some apps read skills from a folder. `vsh skills export` writes every skill granted to one key there:

```sh
export CN_ENDPOINT=http://127.0.0.1:7465/mcp
export CN_AGENT_TOKEN=<the access key>
vsh skills export --to ~/.claude/skills
```

| Target | App |
|---|---|
| `~/.claude/skills` | Claude Code |
| `~/.agents/skills` | Codex, Cursor, VS Code, OpenCode, Gemini CLI |

- It checks each file's sha256 and records what it wrote in `DIR/.vsh-skills.json`.
- It never overwrites a file it did not write and never writes through a symlink.
- `--prune` removes only files listed in that manifest that this export did not write.
- `--skill NAME` exports one skill.
- Exported scripts get the trust of local skills, so it warns when it writes one.
