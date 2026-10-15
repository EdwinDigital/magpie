# Enterprise client and distribution

The light client adds presentation and automatic-discovery policies plus
application branding, not provider authorization. It does not delete credentials
or disable explicitly stored gateway providers.

## Responsibilities and sources of truth

| Part | Source and responsibility |
| --- | --- |
| Local preferences | [`enterprise.go`](../../internal/settings/enterprise.go): `ResolveEnterprise`, locked `SetEnterprise`, revision conflicts and validation. Ordinary `Save` preserves this field; `KeepOwn` retains local overlays. |
| Package identity | [`distribution`](../../internal/distribution): frozen package profile/logo, stable ID, scheme, isolated default port and update trust keys. The checked-in empty profile is a generic development build. |
| GUI and assets | [`enterprise.go`](../../internal/gui/enterprise.go): API, bounded PNG/JPEG upload, content-addressed assets, personalized HTML and guards before scans/network IO. |
| Editor and projection | [`enterprise.js`](../../internal/gui/assets/enterprise.js), `app.js`: explicit save, provider projection, navigation and cross-window refresh. |
| Brand text | [`enterprise-brand.js`](../../internal/gui/assets/enterprise-brand.js), `i18n.js`: audited application-owned keys, not arbitrary third-party text. Commands, paths, package metadata and legal source stay literal. |
| Icon standard | [`brandicon`](../../internal/brandicon): shared bounded PNG/JPEG normalization and light/dark glyph/application tiles, used by upload and package preparation. |
| Native updates | [`enterprise_desktop.go`](../../internal/gui/enterprise_desktop.go): windows, tray, Dock, webview notification and configuration restart. Windows uses WM_SETICON, not beta.24's empty App.SetIcon. |
| Build and publish | [`enterprisebuild`](../../internal/enterprisebuild), [`enterprise-release`](../../cmd/enterprise-release): isolated snapshot, platform resources, signing/notarization, metadata and manifests. |
| Distribution API | [`enterprisedistribution`](../../internal/enterprisedistribution): tenant profiles, queue, durable job state, production publication and download grants; source/signing are operator-owned. |
| Update trust | [`enterprise.go`](../../internal/update/enterprise.go), `enterprise_auth.go`: signed manifests, sequences, Mac Team/bundle identity and Windows publisher verification. |

## Runtime path and state

Boot and initial HTML use effective branding before the state reads finish.
`GET /api/enterprise` returns effective values, package defaults and a revision.
The editor records changed fields in `overrideFields`; other fields inherit the
current package. A legacy overlay without the list treats all fields as explicit.
Reset removes the overlay, not the embedded image. A stale POST returns 409 and
does not replace the draft. Ordinary settings writes cannot erase this overlay.

Desktop windows receive the latest snapshot. Visible browser windows poll every
two seconds and refresh on focus; an unsaved editor is not overwritten. A recreated
lightweight webview reads boot values. Configuration restart does not install a
staged update; browsers cannot restart servers and a busy gateway refuses restart.
Feature controls, webviews, window titles, tray and Dock branding apply without
restart. On macOS, Wails' native menu captions retain launch-time
options; changing the name reports those entries as pending restart until the
next launch. The About panel uses a custom callback for live name/logo while
keeping version and legal attribution from the bundle. The macOS menu-bar app
identity itself follows the signed bundle, like Finder's installation name. System installation branding belongs to the package, not an
editable signed bundle.

Branding follows the shared `.brand` element when the shell moves it between the
top bar and sidebar. Enterprise configuration participates in both settings tab
projections. Codex shutdown failover cleanup skips hidden or foreign-owned wiring,
like its automatic synchronization and daemon maintenance.

The macOS menu-bar image is a 44-pixel alpha template displayed at 22 points,
matching the original bird's Retina resolution. Transparent white logos keep
their shape and gaps; macOS supplies the foreground color for the bar's appearance.
The allowance compositor uses the same template and native label color.
In-app glyphs select dark ink on light surfaces and light ink on dark surfaces,
including the initial header, favicon and routing marks. Manual or system theme
changes update them without saving or losing an enterprise draft. Native Dock,
window and About icons use matching application tiles; system theme events and
saved theme changes refresh them. Finder/Explorer use the neutral light package
tile, readable on both backgrounds; macOS adaptive Assets.car selection is not
claimed. Both light/dark resources are generated, with the dark ICNS included in
the macOS bundle.

## Presentation contracts

- Visible agents default to Codex (`codex`), Claude Code (`claude`), Claude Desktop
  (`claude-desktop`), VS Code (`vscode`), Copilot CLI (`copilot`) and WorkBuddy
  (`workbuddy`). These are editable defaults, unlike the two fixed provider
  defaults. Missing/null `visibleAgents` in an older local overlay or package
  inherits the defaults; an explicit empty array selects none. WSL twins and
  named profiles inherit their base application's selection.
- Settings' provider and agent selectors are the last two content sections,
  in that order, before Save/Cancel. They reuse the gateway model list's heading,
  chevron, count, searchable row/card design and theme styles. Each section folds
  independently, initially closed, with the fold remembered on this computer.
  Folding/searching never drops checkbox selections or overwrites an unsaved draft.
  `/api/enterprise/agents` lists descriptors without running their detection/CLI
  hooks; stale selected IDs remain editable as unavailable entries. Agent rows
  and installation suggestions use the effective policy, including when the
  backend payload is unchanged or another window saves a new selection.
- An effective enterprise presentation policy disables decorative bird motion
  (`enterpriseUI.birdMotion`) even with the original name/logo or no local
  overrides. Routing's request/reply/replay packets, wires, status and timing
  remain intact; no flying carrier is created. A policy arriving during a flight
  removes the carrier and detaches it from active trips without stopping the
  packet. The header's flap/tail gesture is disabled too; static brand marks
  remain. Pages without an enterprise policy retain their existing bird motion
  and reduced-motion behavior. No new user setting or gateway rule is added.
- Copilot (`subscription:copilot`) and local Ollama (`preset:ollama`) are fixed
  defaults. Cloud Ollama is not. Other IDs use subscription/preset/plugin/custom
  namespaces, never translated names or endpoint substring matches.
- Enabled/off lists, Add/search, removed accounts and migration hints use a
  projection. Shared provider/model data remains intact for routing and history.
  The plugin Add shortcut does not disable installed plugin management.
- Local auto-import is suppressed under enterprise presentation so it cannot
  reintroduce hidden suppliers. Import links and provider shortcuts are checked too.
- The persistent plugin install form is first in both views. Public discovery is
  off by default; market/listings/GitHub/search reject before network IO. While
  closed, npm/readme lookup is limited to installed packages. Explicit install and
  maintenance remain available; closing discovery is not offline mode.
  Explicit ZIP upload reuses local-folder loading and survives restart; see the
  [ZIP installation contract](provider-plugins.md#explicit-installation-from-a-zip)
  for package layout, limits and owned-file cleanup.
  An isolated enterprise cache gets its own Bun runtime on provider-plugin
  installation; a local folder/ZIP cannot be reported installed while required
  runtime preparation failed. Installed plugin management recovers older entries
  lacking a runtime and exposes preparation/listing errors. A loaded plugin's
  supplier must still be selected in `visibleProviders`; showing the plugin Add
  shortcut alone does not permit every plugin provider.
- Library is off by default. Navigation and all GUI library routes are gated.
  Existing agent resources, library files, synchronization and CLI are retained.
- Uploads accept PNG/JPEG only, at most 2 MiB and 4 million pixels. `brandicon`
  strips source metadata, fits visible artwork without stretching on a 1024px
  canvas with 80% foreground, and generates two monochrome glyphs plus two
  rounded application tiles. A consistent opaque corner backdrop is removed
  conservatively; this is not arbitrary photo segmentation. Transparent PNG is
  recommended; empty artwork is rejected. Source colors are retained in the
  normalized source, not in the standardized monochrome UI variants.
  The source is addressed by SHA-256; versioned theme URLs derive from it and
  use a bounded eight-entry cache that verifies source integrity on each read.
  No SVG, remote URL or arbitrary path is accepted.
- Choosing an upload prepares both previews immediately but does not save
  preferences. Save waits for preparation; cancellation rejects a late upload
  response without erasing the active logo. Ordinary saves still retain the
  draft/revision rules. Older frozen PNG sources retain their verified digest
  when repackaged, while obtaining the new theme variants.
  Unreferenced uploads are retained rather than delete another draft's asset;
  housekeeping is an operator concern, not an automatic removal promise.

## Package and trust contracts

### Automatic discovery

`AgentDiscoveryAllowed` and `ProviderDiscoveryAllowed` in
[`settings/enterprise.go`](../../internal/settings/enterprise.go) gate automatic
work for an enterprise package or a saved local enterprise configuration. A
generic CLI with neither keeps its previous discovery behavior. The gates are
checked on each scan, not frozen when the application starts. Saving a changed
selection forgets probe/account caches, never credential files.

Hidden agents are rejected before their detector, installation checks and CLI
queries. Native session/call enumeration, startup history warm-up and native
usage scans also skip unselected stores before opening directories/databases,
including WSL; retained gateway history and direct known-session reads are not
deleted or newly authorized. Automatic catalog synchronization, startup legacy migrations, DSH's
wiring keeper and Codex daemon checks skip them. Hidden omp profiles are not
enumerated; WSL probe scripts include only selected application kinds and are
generated for the current policy. Hidden Claude Desktop descriptors avoid the
Windows installed-package/path probe; an explicit `Find` still materializes the
full descriptor for an intentional CLI/API operation.

Credential helpers used by explicit sign-in/request paths are not visibility
gates: WorkBuddy, ZCode, Command Code, Grok and Kiro apply the policy at native
discovery callers instead. The independent global account-discovery-off switch
still denies subscription credentials.

Native subscription discovery requires its provider to be visible and, where
it reads a supported agent's sign-in, that source agent to be visible too,
except for fixed-default Copilot. Copilot reads existing CLI/editor sign-ins
independently of the Agent list: hiding its CLI or editor still suppresses
installation/version/session scanning and integration, not the visible supplier's
credential discovery. The global account-discovery-off setting still blocks both
Copilot sources, including keychain access. Hidden suppliers are
also skipped by background `FetchNew` model discovery. This is not a new gateway
authorization layer: stored API-key providers and saved sign-in records remain;
explicit configured traffic is not denied by the visibility predicate. An
external application's local-only sign-in is not automatically rediscovered
while its source is hidden, with the Copilot exception above; reselecting the
source restores discovery. The
shared models.dev price catalog, existing gateway history and explicitly
installed plugin runtime are not a public-provider scan or a plugin sandbox.

### External Agent ownership

Private client folders do not relocate external Agent configuration files.
An installed WorkBuddy or Codex still reads its own native directory. Package
identity now also scopes the provider ids, local token markers, Codex model
catalog and Claude Desktop profile used there; see
[Agent wiring ownership](agent-wiring.md#package-scoped-ownership).
The public `magpie` identity remains unchanged. Enterprise technical ids use
`magpie-enterprise-<distributionId>` and unambiguous token delimiters, independent
of mutable display branding. These are ownership markers, not new authentication
or administrator-enforcement rules.

The GUI exposes the active foreign owner with an off/disabled connect switch;
explicit connect/pick/reapply/disconnect operations refuse foreign wiring rather
than taking it over. Automatic sync/follow and the Codex daemon keeper leave it
alone. Single-gateway Agents require disconnection in the owning client first.
VS Code's independently named groups can coexist. Installing this client never
patches an already-installed public client: an older client's explicit edits to
a shared native file cannot be made cooperative by an enterprise update.

Legacy enterprise records using public markers are not converted automatically.
Explicit conversion requires both this client's private applied record and its
own gateway address. WorkBuddy replaces only qualified legacy entries; Claude
Desktop retires only its qualified old profile. Another client's records and
Codex history tables/catalogs remain intact. An ambiguous legacy configuration
fails closed and must be disconnected by its former owner.

Installed plugins distinguish loading failures from suppliers hidden by enterprise
settings. Their suppliers remain selectable without reinstallation, and the row's
visibility hint updates when the policy changes; it never auto-grants visibility.
An installed row's Visible providers action opens the matching selector expanded;
the user still has to check and save it. Asynchronous plugin metadata refreshes
bring newly loaded suppliers into view, and the selector can refresh its candidates
without discarding an unsaved branding/visibility draft. Visible signed-in suppliers
link to their model editor. ZIP rows prefer friendly labels over technical package
IDs, as described in the [ZIP contract](provider-plugins.md#explicit-installation-from-a-zip).
Raw plugin descriptors use `id`/`pid`, not the mapped Agent row's `agent` field.
The host gets this client's effective `MAGPIE_ADDR`, restarts on a port change,
and uses a branded Bun basename in its own cache, not public magpie's host.
Own CLI guidance uses the immutable executable name even after a display-name edit.

Reusable enterprise runtimes load a required frozen package configuration: macOS
uses `Contents/Resources/enterprise-runtime.json`; Windows uses signed PE RCDATA
`ENTERPRISE_PACKAGE`. A generic build never loads a neighboring configuration.
Missing, invalid or logo-mismatched data fails before state initialization, not
back to public Magpie. The payload carries the enterprise version separately
from the compiled runtime version. Resources, Info.plist, icon and version
resources are written before platform signing; editing an installed signed
package is not the customization workflow. Local settings cannot change package
identity, feed or public keys. State/cache/startup records and Windows scheme registration are
isolated, including portable WebView2 data and redirected config suggestions.
The executable/CLI basename is `identity.artifactSlug` (plus `.exe` on Windows),
not the mutable display name. Installed state/cache folders replace `magpie`
with that basename, e.g. `~/.config/msai-client`, `~/.cache/msai-client` and
`~/Library/Caches/msai-client`. If the stable distribution ID differs from the
CLI name, its records live under that branded root in `enterprises/<id>`.
Portable `data` has a branded subfolder. There is no implicit migration or
symlink from old `magpie/enterprises/<id>` paths. Windows shortcuts/App Paths
and macOS second-instance activation use the enterprise identity. Public names,
public bundle/scheme identity and Windows device basenames are rejected.
Enterprise packages cannot replace the shared `magpie` PATH command; use the full
executable path so another distribution's command remains intact.
The stable port derives from distribution ID; a different distribution
found there is not reused. Explicit agent management still writes agent files;
this is not authorization against two apps managing the same external agent.

Enterprise version detection, notes and downloads use the immutable package's
download channel, never public Magpie releases or `MAGPIE_UPDATE_FEED` overrides.
The distribution API binds a platform profile's feed to its own BaseURL, tenant,
distribution ID and channel, filling an omitted feed and rejecting another one.
Standalone builds can embed an operator-owned HTTPS feed instead. Settings shows
the configured channel and server; it offers no public mirror or GitHub fallback,
including after a failed enterprise check. Manual packages return a normal
`manual` update state, start no update timer, and show a managed-download explanation
without Check, automatic-update, interval or missing release-notes controls.
The enterprise CLI refuses public mirror configuration and manual-mode automatic
checks; download advice stays on the enterprise release page/channel rather than
usemagpie.ai, and ignored mirror flags are not described as being used.
Platform packages validate Ed25519 manifests with embedded keys and a
monotonic sequence, then validate file hashes and platform signatures. Mac updates
use enterprise bundle names/IDs and the installed app's Team. Windows checks require
`updates.windowsPublisher` matching the certificate subject. Optional private
`enterprise-update-token` is sent only to the feed's HTTPS host; other-host redirects
do not receive it. Credentials do not enter package defaults.

The API is one configured worker queue, not a provisioned cloud portal or corporate
SSO product. Admin/employee bearer principals can expire. Build requests cannot
select arbitrary source/scripts/signing secrets. Dirty or unsigned developer output
cannot be published. Five-minute scoped download capabilities can be delegated;
they cannot revoke already downloaded applications or provider accounts.
The local JSONL audit is operational best-effort logging, not a transactional
compliance ledger; a production portal needs its own durable audit integration.

## Failure and compatibility

Invalid package profiles/logos prevent enterprise activation. Invalid local schemas
resolve to package defaults (closed light defaults for a generic checkout); corrupt
settings cannot be saved over. Missing local logos fall back visibly to the standard
icon without losing the saved overlay. Builds
cannot overwrite outputs, signing failure cannot publish, and interrupted jobs are
failed after restart rather than rerun blindly. Signed package metadata is replaced
only before signing. The standard enterprise path uses its own `.icns`, not the
original Assets.car. Adaptive icons, installers/MDM, portal frontend, corporate SSO
and remotely locked client policy remain integrations, not inferred capabilities.

Clean-source snapshots accept Git's global PAX archive metadata without creating
a file for it. Tracked source links and unsafe archive paths remain rejected;
the metadata exception does not allow following files outside the source tree.

Small PNG-backed macOS icon representations use Retina `ic11`/`ic12` slots,
not `icp4`/`icp5`/`icp6`, which the native decoder can interpret as legacy pixel
data and show as colored corruption. The exported 16/32/64-pixel PNGs remain
available for Windows resources. Verification includes `iconutil` decoding of
the exact white/transparent fixture, not merely PNG decoding or an ICNS header.

Operator commands are in [enterprise releases](../enterprise-releases.md). The
[planning design](../../plan/enterprise-light-client/enterprise-distribution-design.md)
keeps proposed extensions separate from these current contracts.

## Verification

Use isolated HOME and pinned caches as required by the code standards:

```sh
go test -tags nogui ./internal/distribution ./internal/settings ./internal/gui ./internal/enterprisebuild ./internal/enterprisedistribution ./internal/update
go test -race -tags nogui ./internal/settings ./internal/enterprisebuild ./internal/enterprisedistribution
node --test internal/gui/tests/enterprise.test.cjs
make test-ui
```

Fixture keys and fake build bytes prove API isolation and protocols, not real OS
certificates/notarization. Native platform verification is reported separately.
