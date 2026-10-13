# 技术设计

本文保留开发前的设计基线。当前字段、端点和运行契约以 [企业客户端子系统参考](../../docs/subsystems/enterprise-client.md) 为准；实现差异及测试证据见 [验证记录](validation-report.md)。后文的“当前结构”指分析基线 `76db4c6c`，不是本分支完成后的行为。

## 1. 当前结构与设计约束

- GUI 使用 Go 嵌入静态资产和原生 JavaScript，没有前端框架或 bundler；保持该结构。
- `Handler` 动态生成 `/boot.js`，它不是磁盘上的静态 `assets/boot.js`。目前只传主题、语言、字号、字体及 gateway mode 等首屏配置。
- `settingsState`、`/api/state` 与 `POST /api/settings` 提供偏好读写；通用保存会显式保留其他页面拥有的字段。新增企业字段必须同时处理这条旧写入路径。
- `providersState` 的数据被供应商、网关、路由等视图共用。不能直接裁剪 `provider.All`、`Served`、`Catalog` 或共享 JSON 数组来实现页面隐藏。
- 插件 `load` 当前并行读取安装状态、推荐列表和 GitHub 标签目录，再查询 npm / git 信息；`manual` 当前只出现在未搜索的发现列表底部。
- `show`、`setGatewayMode`、URL / 页签恢复和各页面快捷跳转共同决定导航。隐藏按钮不是完整的入口关闭。

## 2. 配置模型与持久化

在 `settings.Settings` 中新增可选 `Enterprise *EnterpriseConfig`，建议在 `internal/settings/enterprise.go` 定义校验和默认值，不依赖 GUI 或 provider 包。

```json
{
  "enterprise": {
    "schemaVersion": 1,
    "overrideFields": ["appName"],
    "appName": "Enterprise Assistant",
    "logoAsset": "sha256:<normalized-png-hash>",
    "visibleProviders": [],
    "showPluginProviderEntry": false,
    "showCustomProviderEntry": false,
    "publicPluginCatalog": false,
    "libraryEnabled": false
  }
}
```

- 新配置的 `overrideFields` 记录用户明确覆盖的运行时字段，其余继承包内企业档案或程序默认；旧配置的迁移规则见发行设计。示例仅显式覆盖名称，其他快照值不阻止后续包默认更新。
- 恢复 `appName` / `logoAsset` 默认时移除对应覆盖：企业包回退发行品牌，通用构建回退原品牌；不删除包内资源。
- `visibleProviders` 仅存额外选择，使用第4节的带来源选择 ID；有效名单为 `{"subscription:copilot", "preset:ollama"} + visibleProviders`，默认两项不允许删除。
- 缺少整个字段时使用包内企业档案，再回退企业轻量默认；明确覆盖为 false 的字段不能被反序列化为“未设置”。列表去重，顺序稳定，禁止空 ID。
- 新增供应商不自动加入名单。升级后已保存但暂时缺失的 ID 保留并标注不可用；用户可以显式移除，不能把暂时读取失败当成供应商消失。
- `KeepOwn` 保留本机 `Enterprise` 覆盖，备份 / 同步不能覆盖为另一台机器的名单或不存在的 logo；包内默认由企业发行 / 升级提供。这一选择须在实现时核对备份、WebDAV / S3 共用路径。
- 通用 `POST /api/settings` 保留企业字段，仅企业专用端点可写，避免旧窗口或通用页面保存清空它。还需在普通 `settings.Save` 的锁内保存路径保留磁盘最新的企业字段，覆盖窗口尺寸、托盘设置等其他持旧快照的写入者，不能只保护一个 API。
- 企业专用 setter 在同一 `fileMu` 内读取最新设置、验证版本、只更新企业字段并执行原子保存。不使用无锁 `Load → Save` 覆盖整份旧快照。
- 沿用 `Save` 的权限、损坏文件保护和 last-good 规则；读取失败不是空配置，写入失败不发布新配置。
- 回退为轻量默认配置不删除其他 settings、凭据、插件、资源库、路由或 Agent 配置。恢复旧配置使用合法快照，而不是重置整个文件。

## 3. 企业配置 API

| 拟议接口 | 用途 | 约束 |
| --- | --- | --- |
| `GET /api/enterprise` | 当前配置、有效配置、revision、保存状态和生效说明 | 不含密钥、账号凭据或用户 logo 的原始绝对路径 |
| `GET /api/enterprise/providers` | 企业选择器的完整候选目录 | 返回稳定 ID、名称、来源、默认 / 已选 / 不可用状态；使用已加载的本地目录，不为选择器加载公开插件市场 |
| `POST /api/enterprise` | 原子保存企业配置 | 携带读取时的 revision；冲突返回 409，草稿保留，禁止悄悄覆盖另一窗口的保存 |
| `POST /api/enterprise/logo` | 上传、解码及规范化 logo | multipart；限制大小、格式、尺寸；返回受控 asset ID，不接受任意文件路径或远程 URL |
| `GET /api/enterprise/logo/{hash}.png` | 获取已保存或当前预览的规范化图片 | 只匹配应用自管文件，不允许目录遍历；使用正确 Content-Type 与 `nosniff` |
| `POST /api/application/restart` | 仅在确有待重启项时重新启动当前版本 | 与下载 / 安装更新分离；浏览器连接服务时不能任意重启服务器 |

所有端点复用当前 Handler / `magpie web` 的访问保护，不添加匿名例外。建议 revision 使用规范化配置的内容摘要，读取与比较发生在同一配置锁内；具体算法和响应格式在实现时固化为测试契约。

本机覆盖的 logo 文件保存在配置目录自有的 `branding/` 下，按规范化 PNG 内容寻址；发行包 logo 是只读嵌入资源。统一品牌解析器区分两个来源，图片接口只服务其登记的受控资产，不把嵌入 logo 当成缺失的本机文件，也不接受任意路径。

本机上传先完成校验及文件落盘，再原子保存配置引用；失败时旧引用继续有效。上传失败、取消草稿、保存失败留下的未引用资源按延迟清理策略处理，不能提前删除其他窗口或已保存配置仍引用的资源；清理不触及发行包嵌入资产。

保存响应包括新配置、revision、`appliedNow` 和 `restartRequired`。后者必须是具体表面及原因，不是永远返回“需要重启”。待重启状态通过保存配置与当前进程实际应用状态的差异推导，下一次启动成功应用后清除，不保存易过期的绝对路径。

## 4. 供应商目录投影

新增可编辑 `visibleAgents`，默认六项 Agent；缺失/null 继承包默认值，
显式空列表代表不显示任何 Agent，旧版配置保持可读。WSL 与命名配置沿用
基础应用选择。企业设置的“可见供应商”“可见 Agent”作为底部最后两个
内容区，标题、箭头、数量、查找框和行式选项复用网关模型列表风格。
折叠状态仅本机保存，默认折叠，过滤/折叠不丢失草稿选项。

自动发现不再仅靠前端投影：企业包/已保存本机企业配置通过共享策略限制
安装检测、CLI 查询、自动同步/迁移、WSL 探测、本机登录发现和后台模型
发现。供应商与来源 Agent 分别判定；未知/未安装 Agent 仍可在静态目录
勾选。保留设置与登录记录，不调用删除/退出/断开。最新实现契约见
[自动发现策略](../../docs/subsystems/enterprise-client.md#automatic-discovery)。

### 4.1 保持运行数据完整

后端提供候选目录与展示判定信息，前端使用一个共用的“供应商页面可见性”函数生成投影。共享 `providers` 数据仍保持完整，网关、路由、历史记录不依赖这个投影。

```text
effectiveIDs = {subscription:copilot, preset:ollama} union extraSelectedIDs
visibleOnProvidersPage(p) = effectiveIDs contains canonicalSelectionID(p)
availableToAdd(candidate) = effectiveIDs contains candidate.selectionID
```

不能仅按名称匹配：同一供应商可能被改名、使用多个账号或多个实例。建议候选项使用带来源的选择 ID，例如 `subscription:copilot`、`preset:ollama`、`plugin:<actual-id>`、`custom:<stored-id>`；底层 `copilot` / `ollama` 是对应的默认逻辑 ID。实现时统一在配置与 API 间规范化，不混用命名空间。

内置 / moved 订阅共享其逻辑订阅选择 ID；通过 `move.state`、`onPlugins`、插件 `pid` 和真实 provider ID 建立映射，不能盲目剥离 `-plugin` 后缀。独立安装插件的同名 provider 若确实是另一个实例，使用单独候选项，避免自动授予可见性。

`preset:<id>` 应覆盖绑定到该预设的实例（当前 provider 的 `Preset`），自定义实例按保存的实际 ID 选择。Ollama 默认是本地预设，不包含云预设；如果用户自定义了远程 Ollama 地址，不应把此展示名单误称为网络安全限制。

### 4.2 所有页面入口共用判定

在 `app.js` 中核对并覆盖以下路径：

- `renderProviders` 的启用 / 关闭列表、计数及空状态。
- 添加表单的 `SUBS`、预设 / 合作供应商、本地供应商、插件供应商、自定义入口。
- `unusedSub` / `replacedSub` / `subOf` / `pluginSubs` 的既有迁移语义。
- 已移除账号恢复、迁移提示、本地配置发现和手动导入中的可展示项。
- 搜索、从插件页登录后返回供应商页、其他视图跳转、保存后的刷新及 `?view=providers&edit=...`。

企业选择器从完整目录取候选项，而供应商页面只取投影。入口开关只管理快捷创建 / 跳转控件，不把所有插件或自定义实例放进名单。

白名单收缩不调用 `SetOff`、Remove、SignOut、Adopt 或 HandOver。正在编辑将被隐藏的供应商时先处理脏表单；提交成功后关闭失效编辑视图，保留现有运行数据。

不对整个 provider GUI API 增加“非名单不可请求”的运行授权：目前需求是页面展示。若未来要求严格供应商授权，必须另设覆盖网关和底层 provider 的策略，不能复用这个显示判定冒充授权。

## 5. 品牌统一入口

企业展示策略统一通过 `enterpriseUI.birdMotion()` 禁用飞鸟装饰，包括未
上传自定义 logo 的企业默认配置；路由请求、响应、重放不创建飞鸟 SVG，
页头也不再触发振翅/抖尾。策略实时到达时移除已有飞鸟并解除在途引用，
不停止数据包或路由状态更新。静态品牌标识和连线保留，不新增配置开关。

建议新增轻量的品牌读取模块（例如 `internal/branding`），从有效企业配置提供应用自有展示名称和标准化图标，不改变程序技术身份。前端新增独立品牌 helper，避免向全局 `t()` 注入不分语义的字符串替换。

| 表面 | 当前锚点 | 拟议变化 |
| --- | --- | --- |
| 页头、标题、favicon | `assets/index.html`，`assets/app.js` 的 `icon` | 使用 display name / logo；在状态请求前按 boot 配置应用，避免旧品牌首屏闪现 |
| 自有设置与业务文案 | `index.html`、`app.js`、`i18n.js`、`plugins.js`、`library.js` 等 | 按语义改为 `{app}` 等参数，补齐 en / zh / zh-TW / ja / de，兼顾 tooltip、aria、通知及空状态 |
| 启动配置 | `api.go`: `/boot.js` | 增加最小有效品牌与展示策略；继续 `no-store`，不序列化凭据 |
| 原生应用 / 主窗 / 面板 | `app.go`: `Run`、`panelOptions`；`lightweight.go` | 读取展示品牌；核实 Wails 各平台可热更新的标题与图标接口 |
| 托盘与额度合成图 | `app.go`: `setBird`、`flap`；`trayusage_tray.go` | 所有刷新、动画结束、合成和回退都读取当前品牌；自定义 logo 时停用原小鸟振翅帧 |
| 菜单与通知 | `traymenu.go`: `trayMenuLabels`；`quota_alert_notify.go` | 品牌参数和语言分开处理，避免改变语言后回到 magpie |
| 发行包 | `build/darwin`、`build/windows`、`build/icon` | 首次明确注册企业发行身份并在后续版本保持稳定；展示字段和安装图标按 profile 生成，独立输出验证记录 |

实现前通过 `rg -n -i 'magpie|bird|trayIcon|appIcon'` 审计自有界面及原生展示，并建立“应用品牌 / 技术标识 / 第三方信息 / 法律来源”分类清单。每个保留项有原因，不能以“搜索无 magpie”作为验收，因为命令、URL 和包名必须保留。

名称只作为文本或 JSON 编码值渲染，不使用 `innerHTML`、脚本拼接或无转义的 EvaluateJavaScript。国际化 key 的修改同步检查 GUI 测试 fixture，不能只新增一套中文文案。

### 安装包限制

`CFBundleDisplayName`、已签名 bundle 的图标、Windows 程序资源等不是普通用户配置。运行时保存不修改已签名安装包，也不能通过重启承诺更新静态元数据。

系统安装品牌现已确认纳入企业发行设计，方法见 [企业发行包与分发平台](enterprise-distribution-design.md)。平台配置保存为版本化 profile，构建时替换品牌资源、签名 / 公证，供员工下载；本机运行时保存不改变已签名包的静态品牌。发行默认与本机覆盖分层，不将“需生成企业新包”误报为“下次启动生效”。

Windows 锁定 Wails 版本的应用级 SetIcon 是空实现，且窗口初始图标优先取 EXE 内嵌资源；运行时图标需要原生适配，发行图标需要编译期资源替换。macOS 可动态更换 Dock 图标，但发行 Assets.car / `.icns` 仍要单独生成并验证。

## 6. 插件页面与目录开放

补充 ZIP 上传安装：持久表单中的“上传 ZIP”按钮使用系统/浏览器文件
选择器，上传一个包含 `package.json` 和可加载产物的 ZIP。支持根目录或
单层包目录，不自动构建源代码；npm、GitHub、本机目录入口不变。
复用本地目录安装及插件宿主，成功文件存放在当前配置下的
`plugins/imports`，失败清理，卸载只删除该次上传的受管文件。
请求、解压限制、跨平台路径校验、目录关闭与信任边界的当前契约统一见
[ZIP 安装子系统说明](../../docs/subsystems/provider-plugins.md#explicit-installation-from-a-zip)。

- 将 `manual()` 安装表单提升为页面顶层持久区域，布局顺序为：安装表单、信任提醒、视图切换 / 工具条、发现或已安装内容。
- 不把表单放进频繁 `replaceChildren` 的列表 body；搜索、异步数据到达、列表刷新不能清空安装输入、IME 状态或焦点。
- `publicPluginCatalog=false` 时默认显示安装与已安装管理，移除 Discover / 搜索 / 公开推荐；空列表不提供被关闭的 Discover 跳转。
- `load` 先读取有效策略，再决定是否调用 `loadListings`、`loadTagged` 或 `searchNPM`。明确安装不依赖目录已成功加载。
- 后端在 `/api/plugins/listings`、`github`、`market`、`search` 等公开目录请求执行同一开关检查。关闭时在外部网络请求前返回结构化 `feature_disabled` 错误；旧页面 / 直接请求也不能重新加载目录。
- `/api/plugins/npm` 和 README / page 等混合接口须区分公开目录与已安装包信息：目录关闭时仅允许已有插件或本次明确安装目标所需的信息，不接受任意 names 作为公开目录绕路。
- `/api/plugins`、明确来源安装、已有插件启停 / 更新 / 卸载继续可用；git / npm 更新读取限定已有插件，不将“关闭目录”设计成破坏维护功能的全域网络阻断。
- 关闭时取消搜索定时器和可取消的目录请求，清空显示缓存；晚到响应按 revision 丢弃。插件宿主和 moved provider 的请求所有权不变。

## 7. 资源库开放

- 与现有 gateway mode 的导航规则求交集，不允许企业开关把 gateway mode 原本隐藏的页面重新开启。
- 在主导航、`show`、URL / 视图恢复、窗口打开参数和快捷入口统一判断；关闭页面被请求时显示说明并回退到可见页面（建议供应商）。
- 在 `libraryRoutes` 的 GUI `/api/library` 及子路由读写前执行模块开关检查，防止旧页面后台加载、深链接编辑或目录代理请求继续工作；不得拒绝后仍做文件扫描或写入。
- 保存将关闭正在编辑的资源库时，先完成当前应用的放弃确认；保存失败保留草稿和原页面。
- 不修改 `internal/library` 的已应用内容或 CLI 本体，不卸载 MCP / skills / RTK，不把关闭入口扩展成自动删除 Agent 文件。
- 已有异步操作需要区分“保存前已经开始”和“保存后新发起”：后者禁止，前者按现有安全完成 / 取消规则处理，不截断文件写入。

## 8. 保存、广播和重启

```text
读取当前配置与 revision
  -> 本地草稿 / logo 预览（无全局副作用）
  -> 校验及必要的放弃确认
  -> 上传规范化 logo（如有）
  -> 带 revision 提交配置
  -> 锁内检查最新版本、只更新企业字段、原子落盘
  -> 生成新的有效配置与生效结果
  -> 更新本视图并广播到其他窗口 / 面板
  -> 仅有待重启项时提示：立即重启 / 下次启动生效
```

桌面参考字体配置的通知路径，把完整有效企业快照和 revision 发给主窗口与托盘面板；释放中的轻量 webview 在下次创建时读取 boot 配置。通知不重建脏表单，只更新全局展示；正在编辑时显示外部更改提示，保存仍由 revision 检测冲突。

浏览器模式通过现有 state 刷新路径传播配置，必要时增加轻量 revision 查询；不要为本期引入新的实时框架。当前页面保存立即应用，其他页面按约定的有界刷新机制更新；具体时延在实现阶段记录和验证。

| 配置变化 | 预期生效方式 |
| --- | --- |
| 供应商投影 / 插件及自定义入口 | 保存后重绘相关内容；不改运行中的请求 |
| 公开插件目录 | 保存后显示 / 隐藏、取消新目录读取、更新后端请求判定 |
| 资源库 | 保存后更新导航与 GUI API；处理已打开页面和草稿 |
| Web UI 名称 / logo | 保存后更新已有窗口、页面标题、favicon 和自有文案 |
| 托盘 / 原生窗口可变部分 | 平台支持则热更新；不支持的具体项标注待重启 |
| 仅启动时创建的应用菜单 / 原生注册信息 | 核实后列出待重启项；下一次启动读取保存值 |
| 安装包静态信息 | 发行包处理，不归类为重启项 |

现有 `restartToUpdate` 会安装已下载更新，不能直接用作“配置重启”。新的重启操作只重启当前版本，保留窗口 / 页签，默认等待在途网关请求结束；强制中断必须说明影响并再次确认。启动失败或重启失败保留已保存配置并给出错误，不能展示已生效的假状态。

## 9. UI 设计与兼容性

- 复用 `.prefs` / `.row` / `.who` / `.val`、现有分组标题、开关、按钮、输入、弹窗和 `openProtoMenu`；不引入独立视觉系统或新依赖。
- 跟随现有浅色、深色、系统主题，字号和桌面字体；logo 等比例 contain，深浅主题均可识别，超长名称不挤占导航或托盘额度。
- 新字符串全部走 `t()`，名称参数不翻译，模板占位符在五种语言中一致。
- 不使用原生 `<select>`、彩色左边框；点击不主动滚动，遵循 `scrollOnPurpose` 和现有焦点 / 脏表单规则。
- 目录勾选有文字标签和键盘操作；搜索与 logo 预览有明确 aria 名称，错误关联对应字段，保存状态可被辅助技术读取。
- 宽窗口、窄窗口、托盘面板和网页模式分别验证；热更新不造成意外自动跳转、滚动或覆盖正在输入的内容。

## 10. 文档契约更新

本次规划不修改当前子系统参考。实际实现时按契约归属更新：

- `gui-shell.md`：企业设置、boot 字段、品牌来源、导航规则、实时传播与重启失败行为。
- `providers-accounts.md`：供应商页面投影与运行数据 / 授权分离，明确内置及插件候选身份。
- `provider-plugins.md`：目录可见性不改变所有权和宿主生命周期；关闭目录时安装 / 更新接口行为。
- `library.md`：GUI 开放开关、API 拒绝行为和既有 Agent 内容 / CLI 保留边界。
- 企业发行新增配置加载、应用身份、启动项、构建及更新信任契约时，在实际实现 PR 新增相应子系统参考并注册索引；不能把拟议平台写成已存在服务。

## 免编译封装实现补充

`enterprise-release runtime` 将代码按 OS/架构编译一次，输出源 SHA、dirty 状态、
基础版本、目标和哈希。`build --base-runtime` 校验审核过的 runtime 并复用其程序。
macOS 使用 Contents/Resources/enterprise-runtime.json；Windows 使用 ENTERPRISE_PACKAGE
RCDATA。包数据包含 profile、已规范化 logo、企业版本、build number 和基础版本。
缺失/损坏/摘要不符/目标不符时在状态初始化前拒绝启动，不回退公共产品。

文件系统技术名称使用 artifactSlug：进程/CLI 文件名和安装配置/缓存根目录一致。
macOS second-instance 激活绑定企业 bundle ID，Windows 快捷方式/App Paths 绑定企业名称。
既有独立端口、命名空间锁、协议和登录启动项保留；不自动创建共享 magpie CLI 链接。
普通名称与技术名称分开，避免实时展示名称修改造成目录搬迁或升级路径失效。

Windows 下载文件保持 <slug>.exe；签名更新清单使用带 OS/架构的逻辑 lookup key，
URL 指向目标子目录中的真实文件，从而允许同版本 amd64/arm64 共存且进程名无架构后缀。
PE 资源补丁保留基础 manifest 的 DPI/长路径行为和原始版权/许可归属。

原生会话枚举使用延迟 reader 回调，策略检查在目录/数据库读取之前。
WSL 探测和已有会话缓存也过滤当前选择；直接读取已知会话及网关历史保持原语义。
