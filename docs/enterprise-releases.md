# Enterprise releases

`cmd/enterprise-release` is a standalone builder and tenant-scoped distribution API.
Use the project's Go version. Destinations should be outside the approved checkout.
It does not mutate the source checkout or reuse upstream release credentials.

## Profile and assets

Start from [`profile.example.json`](../build/enterprise/profile.example.json).
Choose tenant/product IDs, display brand and registered platform identities; keep
IDs stable across branding changes. An empty logo digest is filled on preparation;
a frozen nonempty digest must match the image. No account/token/private key belongs
in this JSON.

`enterpriseDefaults.visibleAgents` accepts stable application IDs, for example
`["codex", "claude", "claude-desktop", "vscode", "copilot", "workbuddy"]`.
Omitting it/null inherits those six defaults; `[]` selects none. Employees can
override the list locally. Together with `visibleProviders`, it limits automatic
installation/CLI/sign-in/model discovery, not gateway authorization. See the
[discovery contract](subsystems/enterprise-client.md#automatic-discovery).

```sh
go run ./cmd/enterprise-release prepare \
  --profile /absolute/path/release-profile.json \
  --logo /absolute/path/company-logo.png \
  --out /absolute/path/prepared-brand
```

Preparation validates, normalizes a square PNG, hashes it and creates icon sizes and
`.icns`, plus a frozen `profile.json`. It refuses nonempty outputs. The same data
drives package and runtime defaults. The standard builder does not reuse the original
Magpie Assets.car or promise adaptive icon variants.

Upload and preparation share `internal/brandicon`: a 1024px transparent glyph
canvas, 80% centered foreground without stretching, dark ink for light UI and
light ink for dark UI. Application icons use the original bird's neutral rounded
tile proportions, with contrast appropriate to each theme. Input metadata is
discarded; bounded PNG/JPEG only, no SVG. Uniform backgrounds are treated
conservatively, not with general-purpose photographic cutout. Prefer a transparent
graphic and inspect both generated previews before saving.

Preparation emits `logo-light.png`, `logo-dark.png`, `app-light.png`, `app-dark.png`,
the light `icon-*.png`/`enterprise.icns` and dark `icon-dark-*.png`/
`enterprise-dark.icns`. The Mac bundle includes both sets of main theme resources;
runtime app surfaces follow the saved/system theme. Finder and Windows executable
resources use the neutral light tile by default, not a white-on-transparent mark
that disappears on light surfaces. Windows native behavior remains separately
validated; generation or cross-compilation is not a runtime test.

## Build

Use a clean approved checkout. Dirty snapshots require explicit developer flags and
cannot be published:

```sh
go run ./cmd/enterprise-release build \
  --source /absolute/path/approved-source \
  --profile /absolute/path/prepared-brand/profile.json \
  --logo /absolute/path/prepared-brand/logo.png \
  --target windows/amd64 --version 1.0.0 --build-number 1 \
  --out /absolute/path/development-package --unsigned --allow-dirty
```

Targets: darwin/arm64, darwin/amd64, windows/amd64, windows/arm64. Mac GUI builds need
Mac workers; unsigned Windows builds can cross-compile. Signed Windows releases
need a Windows worker. Isolated snapshots reject source symlinks rather than follow
them outside the approved source.

For production omit developer flags and configure authorized worker tooling:

- Mac: `--mac-identity` (Developer ID Application) and `--notary-profile` (notarytool
  keychain profile). The tool signs, waits for notarization, staples, verifies and
  writes the final ZIP, then computes hashes.
- Windows: `--windows-thumbprint` and trusted `--timestamp-url`, with signtool
  installed. Resources are replaced before EXE compilation/signing.
- Enterprise versions are x.y.z and build numbers are 1–65535. A changed brand
  needs a new version. Metadata separately records the source SHA and profile.

Output is enterprise `.app` ZIP or EXE plus `build-metadata.json` with signature
status, target, exact size/hash and source. No installer, PATH alias or system-wide
icon-cache modification is implied.

The enterprise client's Command line section does not replace the shared `magpie`
command. Automation can invoke `YourProduct.app/Contents/MacOS/your-product` (macOS) or
the enterprise EXE by its full path. A separately named CLI alias is an operator
choice, not an automatic takeover of another installed distribution.

## Updates

For employee downloads with enterprise version detection, use `updates.mode: platform`.
The feed is the stable endpoint of the same enterprise download
channel, not public Magpie's version number or an expiring employee download URL.
In the distribution API, supply the channel and `manifestKeys`; `feedUrl` may be
empty and is filled with:

```text
{BaseURL}/v1/updates/{tenantId}/{distributionId}/{channel}/manifest.json
```

An explicitly different feed (including public Magpie or another tenant/channel)
is rejected by this API. Standalone builds must configure their own stable HTTPS
feed. The profile, feed, identity and public keys are frozen into package resources before signing;
runtime enterprise preferences cannot change them. Upstream source releases do
not count as enterprise releases: rebuild and publish the enterprise package to
advance its download channel. About displays the channel and its server.

Use `updates.mode: manual` only for explicitly managed/offline deployments without
app-internal version detection. About then explains where new versions come from,
instead of reporting an update failure or offering ineffective automatic-update
controls. The example QA profile intentionally uses this offline mode; it does
not demonstrate a configured production feed.

For `platform`, configure channel, `manifestKeys` mapping key IDs
to base64 Ed25519 public keys, and `windowsPublisher` for Windows certificate-subject
checking. Private keys remain on the publishing worker. Enterprise packages have
no upstream fallback, ignore public update-feed environment overrides, and do not
trust replacement public keys supplied by a feed. An offline, failed, unsigned or
unauthorized enterprise feed never triggers a check against public Magpie.

```sh
go run ./cmd/enterprise-release manifest \
  --metadata /absolute/path/production-package/build-metadata.json \
  --base-url https://downloads.example.com/artifacts \
  --key-id company-release-1 --private-key /private/path/ed25519-key.pem \
  --sequence 1 > /absolute/path/manifest.json
```

Only signed, clean results may produce a manifest, and bytes are rechecked. The
signed payload is `json.Marshal` of `distribution.Manifest` with an empty signature;
use this implementation, not a different serializer's field/escaping order.

Private feeds can use a scoped expiring employee/MDM credential, never a package
default or URL. Provision the mode-0600 `enterprise-update-token` in that distribution's
state directory, or use the enrollment helper (reads stdin, not a token argument):

```sh
go run ./cmd/enterprise-release enroll --distribution-id acme-assistant
```

Managed deployment can provision this file without employees installing Go. Missing
or expired credentials cause authentication failure, not a fallback update.

## API for a future portal

Create a private mode-0600 access JSON array of `{tenantId, role, token, expiresAt?}`.
Roles are admin/employee; tokens have at least 32 characters, optional expiry is
RFC3339. This bootstrap/ingress contract is not a corporate SSO implementation.
The portal/identity-aware ingress must authenticate employees and map authorized
identities to it. Use HTTPS ingress externally.

```sh
go run ./cmd/enterprise-release serve \
  --listen 127.0.0.1:3460 --out /absolute/path/private-store \
  --source /absolute/path/approved-clean-source \
  --access-config /private/path/access.json \
  --base-url https://downloads.example.com \
  --mac-identity "Authorized Developer ID identity" \
  --notary-profile authorized-notary-profile
```

Configure Windows signing flags on Windows workers and optional `--private-key` /
`--key-id` for platform manifests. A publishing worker must not use developer flags.

| Endpoint | Role | Contract |
| --- | --- | --- |
| POST /v1/profiles | Admin | `{profile, logoBase64}` creates the next immutable revision; tenant and existing identity match |
| GET /v1/profiles/{id}/{revision} | Tenant member | Read own frozen profile, without credentials |
| POST /v1/builds | Admin | `{distributionId, profileRevision, version, buildNumber, target}`; source/signing are operator-owned |
| GET /v1/builds/{id} | Tenant member | Queue/build/sign/verify/ready/failed/published state and result |
| POST /v1/builds/{id}/publish | Admin | Recheck production status/bytes, refuse overwrite, publish artifacts and optional signed manifest |
| GET /v1/downloads | Tenant member | Published files, hashes and five-minute scoped grants |
| GET /download/{grant} | Grant holder | Granted artifact before expiry; capability URLs are sensitive and delegable |
| GET /v1/updates/{tenant}/{id}/{channel}/manifest.json | Tenant member | Stable signed feed, distinct from expiring employee download links |
| GET /v1/artifacts/{tenant}/{id}/{version}/{target}/{name} | Tenant member | Stable authenticated update artifact |

Employee flow: authenticate in the portal, choose OS/architecture, download the
approved package, install and launch. Package defaults work offline. Local overrides
do not rename a signed installation; static brand changes require a new package, but not a new app compilation
when an approved reusable runtime is available.

The API is available for portal integration. It does not provision a portal frontend,
cloud infrastructure, IdP, certificates, installers/MDM or remotely locked policy.
Run an appropriate OS worker per platform; multi-worker scheduling belongs above
this API. Grant revocation cannot delete downloaded files or revoke Copilot accounts.

## Release gate

See [the subsystem contract](subsystems/enterprise-client.md) for verification.
Fixture keys and fake signed-build metadata do not prove actual OS signing. Publish
only after full Go/GUI checks and real signing, notarization, startup/upgrade tests
on the corresponding workers. Never borrow the upstream release workflow's secrets.

## Compile once, customize without recompiling

The `runtime` operation builds one approved base per source/OS/architecture.
The base has no tenant credentials or active public identity; it refuses to run
without a valid packaged configuration. Keep its directory immutable on the
operator-controlled worker. Its metadata records source, dirty status, runtime
version, target and hashes. It is not a downloadable employee application.

```sh
enterprise-release runtime --source /absolute/approved-source \
  --target darwin/arm64 --version 0.1.1172 --out /absolute/runtime-base

enterprise-release build --base-runtime /absolute/runtime-base \
  --profile /absolute/company-profile.json --logo /absolute/company-logo.png \
  --target darwin/arm64 --version 1.0.6 --build-number 8 \
  --out /absolute/company-package --unsigned
```

Developer bases from dirty source require `runtime --allow-dirty`, and cannot
produce production-signed packages. Without `--base-runtime`, `build` retains
its existing convenience workflow, compiling a temporary base first. With it,
packaging does not invoke Go or Git: a prebuilt `enterprise-release` executable
and the platform packaging/signing tools are sufficient. macOS copies the exact
base program under the enterprise CLI name and writes bundle resources;
Windows patches only PE resources (profile/logo RCDATA, icon, manifest, version),
not machine code, then signs the final EXE. The original legal/version
resource attribution and base manifest capabilities are retained. Rebranding needs re-signing and
macOS notarization, not recompilation. Changes to application code need a new base.
Never patch a signed employee installation in place.

`serve --base-runtime /absolute/runtime-base` uses the same profile/build/download
API with an operator-selected base. Requests cannot upload/select arbitrary
binaries or source paths. Target mismatch is rejected; jobs pin the runtime hash
at enqueue and recheck it at packaging. Configure a worker/base per target.
The portal frontend, corporate SSO and production certificates remain external
integrations. The current test package is manual/offline; no production feed is
invented or implicitly configured.

### Process, CLI and storage names

Set `brand.displayName` for the user-facing name and `identity.artifactSlug` for
the filesystem-safe process/CLI basename. For `msai-client`, macOS runs
`msai-client.app/Contents/MacOS/msai-client`; Windows runs `msai-client.exe`.
Windows update manifests use OS/architecture-specific lookup keys but their URLs
point to the plainly named EXE in target-specific directories, so publishing two
architectures does not collide. CLI help displays that command, and full-path invocation does not take over the
public `magpie` command. A manually installed, separately named PATH link is an
operator choice; the client does not replace shell profiles or the public link.
Reserved public names/schemes/bundle ID and Windows device basenames are rejected.

State and cache roots match the basename: `~/.config/msai-client`,
`~/.cache/msai-client`, and the OS cache's `msai-client` folder. Windows uses the
same existing per-user config convention with the enterprise basename and a
separate WebView2 executable profile. Portable output uses `data/msai-client`.
If distribution ID differs from the basename, `enterprises/<id>` under that root
keeps distributions separate. Bundle/App ID, URL scheme and distribution ID must
stay stable across upgrades. Defaults use an independent hashed gateway port;
an occupied port belonging to another distribution is not reused.

Older developer packages used `~/.config/magpie/enterprises/msai-client`.
Migration is opt-in: quit the old enterprise client, back up its folder, and copy
that enterprise subtree to the new root only if the destination does not exist.
Do not copy the public `~/.config/magpie` root, merge two active state trees or
change the public CLI. No code silently migrates/deletes user data.
Two clients can run independently, but explicitly connecting both to the same
external Agent still edits that Agent's shared configuration; assign one owner.
