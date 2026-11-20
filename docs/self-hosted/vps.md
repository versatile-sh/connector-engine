![Run it on a VPS](../assets/section-vps.svg)

# Run it on a VPS

connector-engine runs on any small Linux server you control. Pick the option for who uses it:

| Option | For | Reach it through | Exposed to the internet |
|---|---|---|---|
| [A. Single user](#option-a-single-user-recommended) (recommended) | you alone | an SSH tunnel | nothing but SSH |
| [B. Team or headless](#option-b-team-or-headless) | several people, or a product | HTTPS through a reverse proxy | the engine's public listener, behind TLS and OIDC |

**Never publish the person console on a public address.** It is a single-owner app built for `127.0.0.1`: its Host allowlist, session cookie and OAuth redirects assume loopback. A reverse proxy in front of it is not a supported setup. Teams use option B, the headless engine.

## Option A: single user (recommended)

The console stays on the server's loopback. Your laptop reaches it through SSH, so the server exposes nothing new.

### 1. Install on the server

With the no-Docker binary (the installers are available after the first release, v0.1.0; until then, [build it](../../selfhosted/README.md#build) and copy it to `/usr/local/bin`):

```sh
curl -fsSL https://raw.githubusercontent.com/versatile-sh/connector-engine/main/scripts/install.sh \
  | CONNECTOR_ENGINE_INSTALL_DIR=/usr/local/bin sudo -E sh
```

Or with Docker, from a clone, as on a laptop:

```sh
git clone https://github.com/versatile-sh/connector-engine
cd connector-engine
make up
```

The Docker console is published on `127.0.0.1:7465` only. After a reboot of the server, run `make unlock` in the clone.

### 2. Run the binary as a service

A server has no OS keychain, so the root key rests in plaintext in the data directory (`--no-keychain`). Keep the directory private and back it up encrypted.

```sh
sudo useradd --system --home /var/lib/connector-engine --shell /usr/sbin/nologin connector-engine
sudo tee /etc/systemd/system/connector-engine.service >/dev/null <<'EOF'
[Unit]
Description=connector-engine (person profile, no Docker)
After=network-online.target
Wants=network-online.target

[Service]
User=connector-engine
Group=connector-engine
StateDirectory=connector-engine
StateDirectoryMode=0700
ExecStart=/usr/local/bin/vsh up --data /var/lib/connector-engine --no-keychain
Restart=on-failure
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload
sudo systemctl enable --now connector-engine
journalctl -u connector-engine -n 5
```

The journal shows:

```text
console: http://127.0.0.1:7465   agent endpoint: http://127.0.0.1:7465/mcp   data: /var/lib/connector-engine
```

Every listener is on `127.0.0.1`: the console on 7465 and the engine on two free ports.

### 3. Open the tunnel from your laptop

```sh
ssh -N -L 7465:127.0.0.1:7465 you@your-server
```

Keep it open, then browse to `http://127.0.0.1:7465` and create the owner account.

- Use the **same port** on both ends. The console accepts only `127.0.0.1:<its port>` as Host, and its OAuth redirects name that port.
- AI apps on your laptop connect to `http://127.0.0.1:7465/mcp` through the tunnel, exactly as on a local install ([connect an AI app](../getting-started/connect-an-app.md)). They work while the tunnel is up.
- To keep the tunnel up, use `autossh` or a `LocalForward 7465 127.0.0.1:7465` line in `~/.ssh/config`.

### 4. Back up and update

```sh
sudo -u connector-engine vsh export --data /var/lib/connector-engine
```

It asks for a passphrase twice and writes a dated `.cebak` file ([account and backup](../using/account-backup.md)). A full backup always needs that passphrase typed, so it cannot run from cron; `export --setup --yes` (no credentials) can.

To update: export, rerun the installer, then `sudo systemctl restart connector-engine` ([updating](updating.md)).

## Option B: team or headless

The headless engine with Postgres, from the repository's Docker Compose file, behind Caddy for TLS. People and agents sign in with your OIDC provider.

### 1. Firewall first

```sh
sudo ufw default deny incoming
sudo ufw allow OpenSSH
sudo ufw allow 80,443/tcp
sudo ufw enable
```

Docker writes its own iptables rules and can bypass ufw for published ports. The compose file publishes the engine on `127.0.0.1:8080` and `127.0.0.1:8081` only, so nothing reaches it except through Caddy. Keep it that way.

### 2. Configure and start the engine

```sh
git clone https://github.com/versatile-sh/connector-engine
cd connector-engine
```

Edit `deploy/profiles/headless/connector-engine.yaml`:

```yaml
server:
  publicURL: https://engine.example.com
auth:
  issuers:
    - {name: "https://login.example.com", url: "https://login.example.com/.well-known/jwks.json"}
```

`publicURL` is the default `aud` of tokens; your IdP must issue that audience, or set `auth.audience` and `auth.manageAudience` ([OIDC](../enterprise/oidc.md)). Then:

```sh
make up PROFILE=headless
```

It starts `init` (it writes a random operator token and the Postgres passwords into the `secrets` volume), Postgres 16 and the engine. Check it:

```sh
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/readyz
```

```text
204
```

### 3. TLS with Caddy

Point a DNS record at the server, install Caddy from its packages, and put this in `/etc/caddy/Caddyfile`:

```caddyfile
engine.example.com {
	reverse_proxy 127.0.0.1:8080
}
```

```sh
sudo systemctl reload caddy
```

Caddy gets and renews the certificate by itself. Only the public listener (`:8080`) is proxied; the operator listener (`:8081`) never is.

### 4. Tenants, roles and the operator token

The operator token lives in the `secrets` volume and never leaves the server. `make admin` reads it for you:

```sh
make admin PROFILE=headless ARGS="tenants create acme"
```

Give your people roles at boot (`bootstrap.roles`) or through the API ([OIDC: roles](../enterprise/oidc.md#roles)). To reach the operator listener from your laptop, use a tunnel: `ssh -N -L 8081:127.0.0.1:8081 you@your-server`.

### 5. Backups

The database and the secrets directory go together: rows without the root key cannot be opened. A nightly root crontab entry:

```sh
0 3 * * * cd /root/connector-engine && docker compose -f deploy/compose.yaml -p connector-engine exec -T postgres pg_dump -U postgres connector_engine | gzip > /var/backups/connector-engine-$(date +\%F).sql.gz && docker run --rm -v connector-engine_engine-data:/data:ro -v /var/backups:/out alpine tar czf /out/connector-engine-keys-$(date +\%F).tgz -C /data .
```

- The keys archive holds the root key in plaintext (`localfile`). Keep `/var/backups` at mode 0700 and encrypt both files before they leave the server.
- Test a restore on another machine before you rely on it.

### 6. Updates

```sh
cd /root/connector-engine
git pull
make up PROFILE=headless
```

Back up first. `make up` rebuilds the image; the engine applies its migrations at start. Pin a release with `git checkout v0.1.0` once tags exist.

## Checklist

- [ ] Only SSH (and 80/443 for option B) are open in `ufw status`.
- [ ] `ss -ltn` shows 7465, 8080 and 8081 on `127.0.0.1` only.
- [ ] Option B: `https://engine.example.com/readyz` answers 204, and a token from your IdP is accepted.
- [ ] A backup was restored once on another machine.

More: [security model](../enterprise/security-model.md), [scaling](../enterprise/scaling.md), [troubleshooting](troubleshooting.md).
