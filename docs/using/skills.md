![Skills, granted](../assets/section-skills.svg)

# Skills

A skill is a folder of instructions and files an AI app can load on demand. The engine serves skills like connectors: pinned, per key, off, on or ask.

## Add a skill

1. Open an install link: `http://127.0.0.1:7465/import?listing=<https URL of a skill catalog entry>`. The versatile.sh skills library uses these links.
2. Review the licence, the source at its pinned commit, the file list with sizes, and a warning when the skill carries scripts.
3. Confirm, then set it per key on the **Skills** page. Keys made later start off.

On confirm, each file is fetched over public `https`, checked against its sha256 and pinned to `source.commit`. Refused: a branch name instead of a commit, `..`, an absolute path, or more than 256 files, 1 MiB a file or 4 MiB a skill.

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
