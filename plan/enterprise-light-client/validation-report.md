# 实现与验证记录

日期：2026-10-12（Asia/Shanghai）。分支：`codex/enterprise-light-client`。
基线：`76db4c6c2813f270aeaaf0c16ff97a62f2c8018f`；实现仍在未提交工作树中。

## 1. 交付范围

| 需求 | 实现 |
| --- | --- |
| R1 品牌 | 企业设置、名称/PNG/JPEG 上传、首屏/标题/favicon/自有文案、窗口/托盘/Dock、原生 About；保留技术标识、第三方信息与法律署名 |
| R2 供应商 | Copilot 与本机 Ollama 固定默认；其他名单、插件/自定义快捷入口独立控制；地区端点可单独选择；不删除凭据或改变网关授权/模型/路由 |
| R3 插件目录 | 默认关闭公开目录，GUI 停止读取，Handler 在扫描/外部请求前拒绝；明确安装及已安装维护保留 |
| R4 安装入口 | 发现/已安装均置顶，列表刷新保持输入与安装状态 |
| R5 资源库 | 默认关闭导航、深链接及 GUI API；既有 Agent 资源/底层库/CLI 保留 |
| R6 生效 | 保存、revision 冲突、草稿保护、两秒浏览器传播、旧响应保护；macOS 原生菜单项名称提示重启，About/窗口/托盘/网页实时更新 |
| R7 UI | 复用现有 settings/words/checkbox/button 样式，五语言、浅/深色与窄窗口；不加入前端框架 |
| R8 发行基础 | profile、包内默认/显式本机覆盖、品牌资源/独立身份/目录/协议/更新信任、本地构建工具及租户分发 API |

关键入口：`internal/settings/enterprise.go`、`internal/gui/enterprise.go`、
`internal/gui/assets/enterprise.js`、`internal/distribution`、
`internal/enterprisebuild`、`internal/enterprisedistribution`、
`internal/update/enterprise*.go`、`cmd/enterprise-release`。
当前契约见 [企业客户端子系统](../../docs/subsystems/enterprise-client.md)，
构建/部署接入方法见 [企业发行说明](../../docs/enterprise-releases.md)。

### 2026-10-13：订阅读取、插件供应商与接入归属统一修正

- Copilot 固定默认供应商独立读取既有 CLI/编辑器登录，不再被隐藏 Agent 条目挡住；全局订阅隐私开关仍阻止文件/钥匙串读取。隐藏 Agent 的安装、版本、会话和自动接入扫描未放开。测试采用合成明文登录，未访问实际钥匙串或真实 Copilot 服务。
- 汇入 1.0.7 的首次本地/ZIP 安装准备 Bun 和旧安装恢复。插件行区分加载错误与供应商被企业配置隐藏；修正原始插件描述符 `id`/`pid` 与映射后 `agent` 字段混用。可见供应商选择仍为显式授权展示，保存后提示实时更新。
- 接入归属采用固定发行命名空间、带分隔符的本机令牌、独立 Codex 模型目录和 Desktop profile UUID；接入、选模型、重接、断开、同步/Follow 不静默接管另一客户端。WorkBuddy 仅删除当前命名空间，Codex 不重写公开版历史表/目录，VS Code 的组名独立。旧企业标记只在私有接入记录和自身地址均匹配时允许显式转换，Desktop 会退役其旧共享 UUID；不自动迁移真实 Agent 文件。
- 插件宿主使用本客户端的有效 `MAGPIE_ADDR`，端口变化在下次使用时退役/重建宿主，Bun 文件/进程 basename 跟随企业 CLI 名称。补齐 Agent 有模型/无模型两种介绍文案及 quota CLI 指引，CLI 名称不跟随可变展示名称。公开版归属提示中的 `magpie`、协议/SDK 标识和第三方名称不错误改成企业品牌。
- 整仓 `go test -p 2 -timeout 30m -tags nogui ./... -count=1` 再次通过，52 个有测试包；gateway 576.303s，见 `ownership-all-go-release.log`。最后修改的 agent/plugin/gui 完整复跑通过，分别 178.986s / 71.424s / 59.702s，见 `ownership-final-packages.log`。新增归属、Copilot、插件提示和身份测试 `-race -count=3` 通过：`ownership-race-final.log`；Desktop 旧 profile 与宿主端口变化追加回归亦重复通过：`ownership-last-cases.log`、`ownership-last-race.log`。
- Go overlay 故意恢复错误耦合/取消归属保护/取消宿主地址注入/去掉令牌分隔符后，四类测试产生行为断言 FAIL，不是编译失败：`ownership-mutations.log`。首次 UI 回归亦实际抓到插件 id/agent 映射导致勾选后提示不更新；修复后七个相关 Playwright 文件共 90 项通过：`ownership-ui-final.log`。两引擎五语言、浅/深色及窄窗口覆盖；最后空模型介绍补齐后，品牌/接入/插件的 42 项复跑见 `ownership-ui-shipped.log`，截图在 `ownership-screenshots/`。Browser 插件不可用，使用仓库隔离 Playwright fixture；未重跑整个约 6000 项 UI 套件。
- macOS vet、Linux/Windows 编译期 vet 和两平台交叉构建通过：`ownership-vet-build.log`。仍未运行 Windows 原生环境、真实 Agent 应用端到端接入或真实 AIGateway/Copilot 授权与推理；AppKit 枚举和 objc 链接警告保留。
- 最终 macOS ARM64 开发包 `/Users/edwin/.codex/artifacts/msai-client-1.0.8-macos-arm64`，版本 1.0.8 / Build 10 / profile revision 2。ZIP 17521260 字节，SHA-256 `44679273c7729242fa6eb38efecb75a07108dd8899658f7b9cc721003327eefe`；v5 基座和包内程序 SHA-256 `7b8230b044f054c99fd1128b9d1629afc4c8cab4b30c3dea1726c5b37c97e8f4`。版本/Bundle ID/程序名、摘要一致性与 ZIP CRC 通过：`ownership-release-build-final.log`、`ownership-package-integrity.log`。
- 实际交付二进制在临时 HOME/配置目录中，使用合成供应商与 WorkBuddy JSON 验证：拒绝公开版接入、写入企业独立 vendor/token、断开后原公开条目完整保留，且未创建公开客户端配置目录。未打开桌面窗口或调用真实账号：`ownership-shipped-cli-smoke-final.log`。公开 `/Applications/magpie.app` 的 PID 96144 保留；未替换用户安装程序、修改真实设置/登录/Agent 文件、提交或推送。
- 升级后旧共享 Agent 若仍属于公开版，必须先在那里断开，再在企业版接入。企业更新不能让未更新的旧公开客户端自动遵守新归属保护，不支持它与企业版同时显式改写同一单网关 Agent。未签名、未公证、dirty 的开发包不作为生产发行。

### 2026-10-13：首次本地插件安装遗漏运行环境

- 现场只读检查：`msai-client` 已登记本地 AIGateway 插件，但独立缓存没有 Bun，且没有 `plugin-providers.json`。公开 magpie 的缓存另有 Bun；不共享缓存是预期隔离行为。当前企业供应商白名单为空，打开插件快捷入口不等于允许全部插件供应商。
- 原因：本地文件/文件夹与 ZIP 安装绕过 npm 的 `bun add`，此前只登记插件；已安装页面与后台刷新又在没有 Bun 时跳过宿主。既有 ZIP 宿主测试预先提供 Bun，未覆盖首次安装的空缓存。
- 修复：共享安装器在保存 provider 插件之前准备 Bun；下载失败不登记安装，ZIP 失败清理受管目录。仅在主进程运行的 middleware/Agent 插件不下载 Bun。已安装页面为旧登记项准备宿主，并把宿主/供应商读取错误传到现有“未能加载”行，不再以“没有能用的登录”掩盖运行环境失败。供应商白名单不自动放开。
- 新回归覆盖文件、文件夹、ZIP 的首次准备和复用、失败保留旧记录/清理 ZIP、主进程插件不下载，以及本地 HTTP 提供真实 Bun 后加载合成 ZIP 供应商并跨宿主重启。`plugin-fresh-runtime-host-proof.log` 明确 PASS，没有跳过真实宿主用例。
- 旧逻辑通过 Go overlay 恢复后，准备运行环境、安装失败回滚、ZIP 宿主与旧安装恢复测试均产生行为断言 FAIL：`plugin-fresh-runtime-mutation.log`。未改回实际工作树。
- 两个受影响 Go 包完整通过：plugin 73.917s、GUI 62.762s，日志 `plugin-fresh-runtime-suites.log`；最后补充错误行传播后 GUI 完整复跑 61.042s，通过 `plugin-fresh-runtime-gui-final.log`。新增运行环境/ZIP 回归 `-race -count=3` 通过，最终 GUI 回归亦重复通过：`plugin-fresh-runtime-race.log`、`plugin-fresh-runtime-gui-race-final.log`。
- macOS 与 Linux/Windows 编译期 vet 通过：`plugin-fresh-runtime-vet-final.log`。未修改前端资源，未重跑 Playwright 全量；未测试 Windows 原生环境或真实 AIGateway 授权/推理业务。既有 AppKit 枚举及 objc 链接警告保留。
- 新开发包：`/Users/edwin/.codex/artifacts/msai-client-1.0.7-macos-arm64`，版本 1.0.7 / Build 9；ZIP SHA-256 `c76effdb0835381b5567b777e9f4eb534b5c95ca8a5996fb20a29511d0d255e5`，大小 17503245 字节。基座 v4 SHA-256 `faa9f00a482f1f761ff68187aca91762b81340c99a384084f776a6065549fb1e`，与包内可执行文件一致。ZIP CRC、版本/Bundle ID/程序名通过；隔离 HOME 中 `--help` 显示 `msai-client`，不打开桌面窗口：`plugin-fresh-runtime-build-final.log`、`plugin-fresh-runtime-cli-smoke.log`。
- 保持原企业身份、配置目录、端口与 profile revision 2。已登记插件无需重新上传：用户安装修复版后打开插件页面补齐运行环境，再在企业配置末尾勾选插件供应商。未更改用户设置/登录、替换安装应用或停止公开 magpie；仍为 unsigned + dirty 开发包，不能作为生产签名发行。

### 企业下载频道版本检测补充（2026-10-12）

- 企业包的 GUI/CLI 版本检测、更新内容和下载均以包内发行身份、频道与信任公钥为准，忽略通用版 `MAGPIE_UPDATE_FEED`；频道失败、未授权、公开版无签名响应、错误频道均拒绝，不回退公开 Magpie。
- 分发 API 为 `platform` 档案嵌入本平台的 `BaseURL/v1/updates/{tenant}/{distribution}/{channel}/manifest.json`，拒绝公开源及其他企业/频道；独立构建可显式配置自有 HTTPS 更新服务。
- 关于页显示频道/服务器。`manual` 测试包显示企业分发说明，不报检查失败，也不显示无效的检查、自动更新、间隔和更新内容入口。CLI 不允许企业包配置公开镜像或在手动模式开启自动检测，下载指引不会指向公开官网。
- 新增 Go 渠道/GUI状态/分发 API 三项测试通过：`update-channel-go.log`。反向变异去掉企业 feed 隔离后，真实断言捕获公开源请求并失败，恢复后通过：`update-channel-mutation.log`。
- 新增页面测试在 Chromium/WebKit、五语言、浅/深色、平台/手动模式的 40 组组合通过，并检查移动浏览器布局：`update-channel-enterprise-ui-final.log`。原生桌面在正常窗口尺寸检查；440px 使用浏览器模式而不是低于原生窗口最小宽度的布局。
- 本次 `Enterprise|Update` 定向 Go 回归在主包、update、gui、enterprisedistribution 四包通过；新增渠道/手动状态/平台绑定用例与 CLI 用例的 `-race -count=3` 通过。受影响包及主包的 macOS/Linux/Windows vet（后两者仅编译检查）、nogui CLI 构建、gofmt 与 diff 检查通过：`update-channel-final-go.log`、`update-channel-final-cli.log`。
- 企业页面、原有版本检测/自动更新/镜像行为、繁中/日文/德文检查合计 49 项通过、0 失败：`update-channel-final-ui.log`。既有完整 GUI 收尾状态不由本次定向通过替代。旧 QA macOS v3 包采用 `manual` 且早于本次修正，不能据此认为企业生产 feed 已配置；本次未重建安装该包，也未接入真实生产分发渠道。

## 2. 环境与隔离

### 免编译封装、独立安装身份与最终开发包（2026-10-13）

- 已实现 `runtime` 与 `build --base-runtime`：Mac 复用完全一致的基础程序字节，写入签名前的 bundle 配置/图标/版本；Windows 仅补丁 PE 资源，保留原始版权与 manifest 的 DPI/长路径行为。平台 API 可配置固定基础 runtime，任务入队固定其摘要并在构建时重验。
- 企业版本从包资源读取，与基础版本独立；缺失/损坏/摘要或目标不符时启动前失败，不转为公开身份。通用客户端不读取相邻企业配置文件。真实封装测试把 Go/Git/clang/cc 全部替换为失败见证，同一基础程序生成 Corp Preview 2.0.0，仍成功且程序字节完全相同：`repack-no-compiler.log`。
- 进程与 CLI 使用 artifactSlug，macOS 为 msai-client.app/Contents/MacOS/msai-client，Windows 下载文件为 msai-client.exe；安装配置/缓存根目录使用同名企业目录。重复启动激活企业 bundle ID，Windows App Paths/快捷方式使用独立名称，不扫描/替换公开 magpie 的 shell 入口。旧企业目录不自动移动；文档给出显式备份迁移边界。显式管理同一外部 Agent 仍共享其配置，不能据此承诺两个客户端同时接管它。
- WorkBuddy/ZCode/Command Code/Grok/Kiro 的可见性检查移到自动发现调用点，保留显式凭据/请求读取，独立的全局账号发现关闭规则仍有效。原生会话/调用枚举和启动历史预热补齐选择过滤，检查发生在读取目录/数据库之前，WSL 缓存也过滤；既有网关历史和已知会话直接读取不删除。
- 全仓 `go test -p 2 -timeout 30m -tags nogui ./... -count=1` 通过（52 个有测试的包）：`repack-all-go-final.log`。首次使用默认 10 分钟的网关套件在仍推进时超时；30 分钟预算的独立网关套件 980.929s 通过，全仓重跑中的网关 853.773s 通过。新测试曾写入 TestMain 固定的 Kiro 夹具目录，污染后续测试；改用现有 kiroSandbox 后全仓通过，不归咎于实际账号或平台。
- 策略、资源、身份、实例锁的定向 `-race -count=3` 与 Mac/Linux/Windows 编译期 vet 通过：`repack-vet-race-final.log`；免编译封装/PE 资源用例的竞态重复验证通过：`repack-new-race.log`。最终 enterprisebuild 全包再次通过，包含 PE 原始版权保留和双架构更新 lookup key：`repack-enterprisebuild-final.log`。Windows 原生运行仍未执行。
- 行为反证：关闭包资源加载后启动公开身份，取消路径命名空间后回到 .config/magpie，把可见性重新放入显式 WorkBuddy 读取后拒绝凭据，去掉基础程序摘要校验后接受改动，取消会话过滤后枚举隐藏文件，均产生实际断言失败；全部通过副本/overlay，不改动源文件：`repack-loading-mutation.log`、`repack-paths-mutation-final.log`、`repack-explicit-mutation.log`、`repack-integrity-mutation.log`、`repack-sessions-mutation.log`。
- 整套浏览器回归实际执行 6081 条 TAP 记录：6066 通过、15 失败（包括父级聚合），耗时约 134 分钟：`repack-all-ui.log`。失败涉及旧 SVG use/八页签结构断言、异步保存回执、测试自身 focus 自动滚动与底栏遮挡、市场卡片的过紧响应预算；已按实际行为修正测试，不改产品界面或强制点击。收尾六个文件共 62 条记录复测通过、0 失败：`repack-ui-failures-final.log`；包含延迟保存回执、五语言键盘焦点、实际 wheel/hit-test、SVG/图像/CSS mask 品牌及九页签布局。不把初次全套运行改称一次全绿，也未再次重跑整套浏览器套件。
- 使用最终原生程序与通用 nogui 程序，在同一隔离 HOME 同时启动两个网关；进程分别为 magpie/msai-client，配置分别是 .config/magpie/.config/msai-client，企业默认端口 39551。两个合成密钥分别完成真实本机服务器栈 Chat 请求；未读取真实账号，未启动桌面 UI，当前 /Applications/magpie.app（PID 96144）保持运行：`repack-coexist-smoke-final.log`。测试进程结束后清理，不留下新网关。
- 编译 msai-client 1.0.6（Build 8，profile revision 2，macOS ARM64），包含 ZIP 安装、双主题图标、企业静态路由、Agent/供应商控制与扫描策略。目录 `/Users/edwin/.codex/artifacts/msai-client-1.0.6-macos-arm64`，ZIP SHA-256 `995bea4c7b033c2de841400cbc5aa65e74ebb31ed50dbd983a8f2416f3fe2892`。实际包的版本、CFBundleExecutable、源程序摘要、ZIP CRC 与两套 ICNS 全部 native 解码表示通过：`repack-package-integrity.log`。
- 可复用基础目录 `/Users/edwin/.codex/artifacts/enterprise-runtime-0.1.1184-macos-arm64-v3`；免 Go 编译器的操作工具 `/Users/edwin/.codex/artifacts/enterprise-packager-macos-arm64`。开发包未签名/公证、源目录 dirty、手动企业更新，禁止生产发布；没有配置或假装接入生产分发 feed/企业 SSO。未安装、替换、重启用户现有客户端，未提交、推送或发布。

### 可见 Agent 与自动扫描补充（2026-10-12）

- 新增 `visibleAgents`，默认 Codex、Claude Code、Claude Desktop、VS Code、Copilot CLI、WorkBuddy，均可取消勾选。旧配置缺失/null 继承默认，显式 `[]` 保持空。发行 profile 同步支持该字段；已保存本机配置与企业包的扫描策略实时生效。
- 可见供应商、可见 Agent 位于基本配置后、保存区前的最后两个内容区；复用模型列表标题/箭头/数量/查找框/行式选项，独立折叠且本机记忆，默认折叠。查找/折叠/刷新保留草稿；供应商 Copilot/Ollama 仍是固定默认。后台载荷不变时也刷新隐藏页签，跨窗口变化和空选择不会留旧 Agent/安装建议。
- 后端在 Agent 检测、安装/CLI、自动同步/启动迁移、Codex/DSH keeper、WSL 探测、命名配置枚举和本机订阅来源处检查选择；隐藏供应商跳过后台模型发现。保留显式供应商配置/登录记录，不调用退出/删除/断开；未选择的外部应用本机登录不再自动发现。
- 实际 FIFO/进程见证确认全取消后既不读取被观察的凭据文件，也不运行身份 CLI；设置迁移/去重/空数组、默认六项、动态 WSL 脚本、隐藏 Desktop 的冷描述与显式查找通过。7 个受影响 Go 包完整 `nogui` 测试通过：distribution/settings/agent/provider/gui/enterprisebuild/enterprisedistribution，日志 `agent-visibility-suite.log`。
- 新增策略关键用例与 GUI 保存用例的 `-race -count=3`、macOS vet、Windows 仅编译 vet 通过：`agent-visibility-race-vet.log`。无 Windows 原生环境测试，既有 AppKit 警告保留。
- Chromium/WebKit、五语言、980px/390px、浅/深主题抽样验证默认/折叠/查找/勾选/保存/即时投影/跨窗口空选择；新增 Agent 与图标、ZIP 回归共 26 条 TAP 记录通过：`agent-visibility-final-ui.log`。原有企业页 6 条回归通过：`agent-visibility-existing-ui.log`。实际 mouse wheel 覆盖新增长列表，尊重既有禁止点击自动滚动逻辑。截图在临时 `agent-selection-screenshots`，未接触真实凭据。
- Go overlay 去掉检测保护后实际检测未勾选项；去掉来源策略后 FIFO 见证捕获大量凭据打开。副本恢复仅在可见页签重绘后，旧 Agent 行保留，新 Agent 未显示，UI 用例实际失败。变异不修改源文件，无编译错误/跳过，当前源码回归通过：`agent-detector-mutation.log`、`agent-source-mutation.log`、`agent-projection-mutation.log`、`agent-visibility-restored-go.log`。
- 本次未重建/安装 `.app`、未启动用户客户端，旧包不会立即出现新页面。未运行全仓 Go 或历史完整 UI 套件，本次通过不代表历史全量失败已消除。

### 企业版停用飞鸟装饰补充（2026-10-12）

- 统一 `enterpriseUI.birdMotion()` 判断：存在有效企业展示策略时，路由请求、响应和重放不创建飞鸟；原始名称/未上传 logo/无本机覆盖也不例外。页头振翅与抖尾停用，静态品牌、连线、数据包、状态与时序保留。策略实时到达时移除已有飞鸟并解除在途引用，不停止请求数据包。
- `enterprise-routing-motion.test.cjs` 在 Chromium/WebKit、五语言、浅/深主题抽样、980px/390px 验证请求、响应、重放、默认 logo 和迟到策略；MutationObserver 检查没有任何飞鸟创建，数据包实际运行。共 14 条 TAP 记录通过：`routing-motion-ui.log`。
- 双主题图标回归及 WebKit 隐藏页面/重放/恢复回归共 11 条记录通过；Chromium 隐藏页面/重放/恢复回归 9 条记录通过：`routing-motion-regression.log`、`routing-motion-idle-chromium.log`。页面无脚本错误，截图在临时 `routing-motion-screenshots`；Browser 插件不可用，使用已有 Playwright 隔离 API/轨迹数据。
- 副本恢复旧的“仅自定义 logo 禁用飞鸟”条件后，新测试实际失败：创建 4 个飞鸟而非 0，迟到策略后响应又创建飞鸟。源码未变异，使用当前真实 assets 重新运行通过：`routing-motion-mutation.log`、`routing-motion-restored.log`。
- 隔离 HOME、固定 Go 缓存下，GUI 包完整 `go test -tags nogui ./internal/gui -count=1` 通过：`routing-motion-go.log`。JS 语法与 `git diff --check` 通过；既有 AppKit 编译警告保留。未测试 Windows 原生环境，未重跑历史完整 GUI 套件，未重建/安装 `.app`，已有安装包不包含本次修改。

### ZIP 插件上传安装补充（2026-10-12）

- 插件页顶部新增“上传 ZIP”，保留 npm/GitHub/本机目录安装；公开目录关闭时仍可上传。支持 ZIP 根目录及单层包目录，需包含 `package.json` 和可直接加载的文件，不自动构建源代码。
- 后端限制压缩包 32 MiB、单文件 32 MiB、解压总计 128 MiB、4096 项；拒绝路径越界、Windows 路径别名、大小写重复、链接与特殊文件。校验实际解压流并清理失败目录和 multipart 临时文件。持久化 `archive` 归属，重启后保留，卸载不删除普通本地插件源文件。
- 实际 GUI Handler 在关闭公开目录时的上传、输入拒绝、安装状态和临时文件清理通过；受管文件生命周期、安全/限制/取消测试通过。复制既有 Bun 1.4.2 可执行文件到临时工具目录，使用合成插件验证两次宿主启动和 provider 注册，未读取真实登录信息：`zip-go.log`。
- `go test -tags nogui ./internal/plugin ./internal/gui -count=1` 两个受影响包完整通过：`zip-package-suite.log`。新增持久化、实际宿主、GUI 路由的 `-race -count=3` 通过：`zip-race.log`。macOS native GUI Handler 用例、macOS vet 与 Linux/Windows 仅编译 vet 通过：`zip-vet-native.log`；既有 AppKit/链接警告保留，未运行 Windows 原生环境。
- ZIP 上传 UI 的 Chromium/WebKit、五语言、桌面/390px、浅/深主题抽样共 12 条 TAP 记录通过；真实本地 HTTP 接收器验证二进制 multipart 内容，覆盖忙状态、草稿保留、刷新、错误恢复和同文件重试：`zip-ui.log`。npm/GitHub/本地目录和企业页回归 38 条记录通过：`zip-ui-regression.log`。截图保存在临时 `zip-ui-screenshots` 目录，Browser 插件不可用，使用已有 Playwright。
- Go overlay 反向验证：删除上传路由后 `TestPluginZIPUploadRoute` 实际失败；去掉 portable 路径校验后 `NUL.js` 被安装，安全用例实际失败。无编译错误、无跳过，工作区源码未被变异：`zip-route-mutation.log`、`zip-path-mutation.log`。
- 本次没有重建/安装 `.app`，已有 `msai-client 1.0.5` 包不含此 ZIP 功能；未启动或替换正在使用的客户端。没有执行全仓 Go 套件或历史完整 GUI 套件，本次专项通过不表示此前完整 GUI 的未解决失败已清零。

### 双主题图标与共用处理链

- 新增 `internal/brandicon`，上传与打包复用尺寸/比例/居中留白、纯色背景保守处理、双主题单色图形及圆角应用底板生成。白色透明素材在浅色界面生成深色图形，在深色界面生成浅色图形；原始色值保留在规范化源图，不承诺复杂照片自动抠图。
- 上传后先显示双预览，保存前不替换当前品牌；生成中禁止保存，取消后忽略迟到响应。页面 Logo/favicon/路由随手动或系统主题切换，不重建草稿；原生窗口/Dock/About 使用应用底板，菜单栏仍使用原生 alpha 模板。安装默认浅色底板在浅/深桌面均有对比，不声称 Finder 自动切换 Assets.car。
- 空白透明图像现在拒绝；原 GUI/分发 API 两处全透明占位 fixture 改为有可见内容的图形，与新契约一致。旧冻结 PNG 保留摘要并生成新派生资源。没有修改真实员工配置或退出正在运行的 Magpie/msai-client。
- 浏览器定向回归 21 项通过、0 失败：`dual-icons-ui-final.log`；增加迟到预览断言后双引擎复测通过：`dual-icons-theme-ui-post-mutation.log`。覆盖五语言、手动/系统主题、未保存上传、草稿焦点、取消/失败和移动布局。上传企业图片还暴露了小鸟动画空 `.tail` 节点错误，现已限制动画仅用于原 SVG。
- 反向变异：把浅色图标也生成为白色后，对比度断言真实失败（`dual-icons-contrast-mutation.log`）；去掉上传代际保护后，两引擎捕获迟到响应篡改已取消预览（`dual-icons-upload-mutation.log`），均已恢复并复测。
- 打包 `msai-client 1.0.5`（Build 7，macOS ARM64），在 `/Users/edwin/.codex/artifacts/msai-client-1.0.5-macos-arm64`。实际包内浅/深 glyph、应用 PNG/ICNS 均通过尺寸、对比度、透明边距、macOS 解码、ZIP CRC/摘要及版本检查。SHA-256 `930ed80dfafba1a38d7105c252ee86d6b2a6052d1825c18222c88d63403031fc`。仍是未签名/公证、手动企业分发的开发测试包。
- 相关 7 个 Go 包全量 nogui 测试、native Enterprise 定向测试、关键图标用例 `-race -count=3`、native/Linux/Windows 编译期 vet、gofmt 与 diff 检查通过：`dual-icons-final-go-checks-v2.log`。历史完整 GUI 的 39 条失败记录尚未全部收尾；Windows 原生测试仍按用户要求延期；免重新编译定制方案仍是独立待实施改造。

### msai-client 白色图标修正

- 用户报告 Finder 应用图标为紫黑色乱码；旧包的 `.icns` 经 macOS `iconutil` 解码，16/32 像素表示确实不再是白色，出现 `{80, 0, 112, 255}` 等彩色像素。此前 Pillow 只检查大尺寸表示，未覆盖 macOS 对小尺寸的原生解码，验证不足。
- 保留白色透明 Logo，修正小 PNG 表示为 Retina `ic11`/`ic12`；16 像素 PNG 仍保留供 Windows 独立资源使用。菜单栏模板由 22px 提升至小鸟同规格的 44px/22pt，采用 alpha 与系统着色，不把白色 Logo 当作空白背景丢弃。
- 新增精确 PNG fixture、macOS `iconutil` 往返测试、白色模板透明间隙测试及 Cocoa 菜单栏浅/深色、有/无额度原生绘制测试。定向测试通过：`msai-icon-regression.log`；临时恢复旧 ICNS 标签后真实彩色像素断言失败，恢复修正：`msai-icon-mutation.log`。
- 受影响 enterprisebuild/gui 的企业定向测试、图标用例 `-race -count=3`、native/Linux/Windows 编译期 vet、gofmt 与 diff 检查通过：`msai-icon-final-checks.log`。
- 重建 `msai-client 1.0.4`（Build 6，macOS ARM64）：`/Users/edwin/.codex/artifacts/msai-client-1.0.4-macos-arm64/msai-client-darwin-arm64.zip`；SHA-256 `7101f440c647caaa65e776c46a439ae1de08550478834a0f16b4d268a2d2d60a`。实际包内全部 ICNS 表示经 `iconutil` 解码为白色透明图形；已在 Finder 目视确认白色四格图标，无紫黑乱码；版本/help、ZIP CRC 和摘要通过。该新应用实例未实际启动，菜单栏验证使用部署相同的 Cocoa 绘制路径及测试额度数据，不声称已完成实际账号业务或原生交互验收。
- 完整 GUI 原始执行已结束：6039 个测试记录中 6000 通过、39 失败（含父子测试记录）。本次图标通过不替代这些失败项的收尾；后续包仍标记为开发测试包，不是正式验收或签名发行。

- macOS arm64；Go 1.26.8；Playwright Chromium/WebKit。
- Go 使用临时 HOME/CFFIXED_USER_HOME 和固定 GOPATH/GOMODCACHE/GOCACHE；工具、日志、截图和测试包均在 `/tmp/magpie-enterprise-tools`，不写入项目依赖或真实账号目录。
- 浏览器插件不可用，使用仓库现有 Playwright 工作流。流程：企业设置 → 保存/冲突 → 供应商投影 → 插件安装入口 → 功能开关/跨窗口刷新。
- 原生 `.app` 使用独立 HOME、企业命名空间、虚拟 CLI、禁用账号发现/统计、阻断外部 HTTP 的环境。未使用真实 Copilot/Ollama/其他供应商账号。
- 用户正在运行的 `/Applications/magpie.app` 未关闭、重启或改配置；仅启动/退出自建 QA 包。
- Windows 原生环境测试按用户要求延期；交叉编译/vet 不是 Windows 运行验证。

## 3. 自动化记录

| 检查 | 结果与证据 |
| --- | --- |
| 全量 Go：`go test -tags nogui -p 1 ./... -timeout=45m` | 51 个有测试的包通过；日志 `go-suite-low-contention.log`，无 FAIL。后续原生 About/展示修正另做受影响包复测 |
| 最后受影响包完整复测 | 六个受影响包完整复测通过；日志 `go-affected-head.log` |
| 企业配置/构建/分发/更新专项 | 通过；包括包内继承、并发保存、严格 JSON、哈希/签名/重放、跨租户、发布合并、进程重启恢复；日志 `final-verification.log`、`about-final-verification.log` |
| `-race -count=3` 专项 | settings/distribution/enterprisebuild/enterprisedistribution/update 通过；日志 `final-verification.log` |
| macOS 原生品牌单测 | 通过；包括非正方形模板图标比例/白背景、完整供应商候选与重启状态；日志 `about-final-verification.log` |
| 原生/Linux nogui/Windows vet、构建 | 通过；Windows 仅编译检查；日志 `final-verification.log`、`about-final-verification.log` |
| 企业 UI 专项 | Chromium/WebKit、en/zh/zh-TW/ja/de、light/dark、440px/1100px 均通过；最终全量日志包含八个企业专项测试 |
| 完整 GUI：572 个测试文件、并发 2、TAP | 执行中；日志 `ui-suite-tap-final.log`，完成后记录最终数量与结果 |
| 行为变异检查 | 去掉功能 guard 后真实断言失败；移除自有品牌键后真实断言失败；恢复后通过。日志 `guard-mutation.log`、`brand-mutation.log` |
| 静态检查 | `gofmt`/JS syntax/`git diff --check` 通过；交付前再次检查 |

日志全路径前缀为 `/tmp/magpie-enterprise-tools/`。早期的全量失败/中断日志不作为最终通过证据。
修复内容包括：非字符串翻译异常、旧配置响应/表单刷新覆盖、德语复合词品牌、遗漏翻译、原生深色输入框、地区端点独立可见性、原生 About 绕过 Go 品牌、发行包镜像/发布说明回到上游等问题。
账号别名测试的独立 scroll 操作在异步刷新时使用了已脱离 DOM 的元素，改用试点击重新定位，仍保留真实点击后的滚动/位置断言。
MCP 工具开关测试的后续点击目标被固定页头覆盖，改为在真实滚轮输入后点击可见开关；保持首个点击不滚动/不位移与并发合并断言，两引擎五语言的 20 项复测通过（`library-switch-final.log`）。
桌面像素比较保留未修改页面；Settings/Plugins 按本次预期变更做专门的交互/布局断言，不要求继续与旧版截图完全一致。

## 4. macOS 实际使用与产物

实际启动了企业 `.app` 并核验：

- 首次启动继承包内 `Enterprise QA` 名称、X 测试图标、轻量默认开关；企业设置可编辑。
- 保存名称后，真实窗口/页面立即变化；原生菜单项存在准确的重启说明。
- 使用相同隔离 HOME 重新启动后覆盖保留、待重启状态清除、菜单项采用启动时新名称。
- 自定义 About 回调在不重启时显示新名称/图标，同时保留包内版本和 `yetone` 法律署名。
- 固定配置的真实供应商页仅显示 Copilot、Ollama 和勾选的 DeepSeek，无插件/自定义快捷入口。
- 真实插件页首先显示 npm/GitHub/本机目录安装入口，仅有“已安装”，没有公开发现列表；资源库导航不可见。
- 深色输入框与当前 UI 一致。浏览器专项另覆盖浅色、窄屏、所有语言、保存冲突、重启拒绝、候选更新及草稿保护。

最终开发包：`/tmp/magpie-enterprise-tools/package-final-macos-v3/acme-assistant-darwin-arm64.zip`。
版本 `1.0.2`，build `4`，Bundle ID `com.acme.assistant`，图标 `enterprise.icns`。
大小 `17401560` 字节，SHA-256 `8615571af461e3f0c5380cd39b419c4f6a6109639e4b6f49789d58134029dae5`。
已重新核验 ZIP/元数据摘要、plist 和包内资源；该包是 **unsigned + dirty 开发产物，不能生产发布**。

macOS 菜单栏最左侧的系统应用身份与 Finder 名称跟随已签名包；本机运行时改名不改包的身份。
本次验证同一隔离环境下的重新启动，不冒充已经完成真实证书、安装升级或生产 LaunchServices/MDM 演练。

## 5. 未验证与外部集成

- Windows 原生 UI、HICON、启动项、签名、安装和升级未执行；Linux 原生 UI、macOS amd64/旧版系统未执行。
- 无真实 Developer ID、notarytool/Authenticode 证书实测；无真实受信包升级、跨版本回滚或 MDM/安装器验证。
- 分发 API 是本地可接入后端，不是已部署门户/企业 SSO/多 worker 平台；本地 JSONL 为尽力而为的运营审计，不是事务性合规账本。
- 新增控制为展示配置，不是管理员锁定或供应商网关授权；未改变厂商请求协议或验证真实订阅业务。
- 上传的未引用图标保留，避免删除另一窗口草稿；在途请求阻止立即重启并要求稍后重试；不自动强制中断、不偷偷安装已下载更新。
- 已存在的 AppKit 枚举转换、重复 objc/本地 SDK 链接警告单独保留；通过编译不代表旧 macOS 版本已经运行验证。
- 未提交、推送、正式签名、部署或发布。
