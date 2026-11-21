![Uninstall cleanly](../assets/section-uninstalling.svg)

# Uninstalling

Stopping the stack keeps your data. Deleting the data is a separate, final step.

## Stop, keep the data

```sh
mise run down
```

The containers go; the volumes stay. `mise run up` brings everything back as it was.

## Delete everything

> This deletes every connector, credential, access key, skill, the account and the root keys. Nothing can bring them back. Run `mise run export` first if you might want any of it.

```sh
mise run export
docker compose -f selfhosted/deploy/compose.yaml -p connector-engine --profile person down -v
docker image rm connector-engine:local
```

`-v` removes the four volumes (`pgdata`, `engine-data`, `console-data`, `secrets`); `docker image rm` removes the image `mise run up` built.

Then remove the unlock key from your OS keychain. `mise run vsh` builds `bin/vsh` if Go is installed:

```sh
mise run vsh
bin/vsh keychain delete --name connector-engine
```

| Also left on the machine | Remove with |
|---|---|
| backups in `~/connector-engine-backups/` | delete the folder once you no longer need them |
| the notification device key, `~/.config/versatile/notify.key` | delete the file |
| the clone | delete the folder |
| AI app entries pointing at `http://127.0.0.1:7465/mcp` | `claude mcp remove versatile`, `codex mcp remove versatile`, or the app's settings |

## Without Docker

`uninstall` stops the install; `--purge` also deletes the data directory and the unlock key in the keychain. Export first:

```sh
vsh export
vsh uninstall --purge
```

Then delete the binaries the installer wrote:

```sh
rm ~/.local/bin/vsh
```

On Windows, delete `%LOCALAPPDATA%\connector-engine`.
