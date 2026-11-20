# plugkit, in-module copy

Upstream: `github.com/agents-control/plugkit` (MIT, see LICENSE in this directory).

Commit: `329d53ecfe458fa413c27b028aaa3dca2fef4494`

Synced: 2026-11-20

Packages: `adapters` `api/acplug/v1` `conform` `discovery` `host` `outbox` `usage` 

Rule: files are upstream files, byte for byte, except import specs, where `github.com/agents-control/plugkit/<p>` reads `github.com/versatile-sh/connector-engine/internal/plugkit/<p>`. Change this directory only with `scripts/sync-plugkit.sh`.

A binary must not link this copy and upstream `api/acplug/v1` together: both register the same .proto files and the protobuf registry panics at init.

Packages whose tests are not copied (a package's tests share helpers, so they come all or none), because a test imports an upstream package outside this set:

- `adapters`: imports github.com/agents-control/plugkit/outbox/sinktest github.com/agents-control/plugkit/serve
- `conform`: imports github.com/agents-control/plugkit/outbox/sinktest github.com/agents-control/plugkit/serve
- `host`: imports github.com/agents-control/plugkit/serve
