# 企业发行包与分发平台设计

## 1. 目标和交付边界

补充需求 R8：企业可在未来分发平台配置应用名称、logo 和功能目录，由平台生成企业专属 macOS / Windows 发行包，供员工下载。员工首次启动即采用包内企业配置，不需要逐台手工配置品牌。

本文件保留发行与未来平台的设计基线。本分支已实现配置档案、构建命令、受信更新和租户分发 API；实际接口见 [企业发行说明](../../docs/enterprise-releases.md)，交付与验证边界见 [验证记录](validation-report.md)。未部署生产平台、未配置真实签名证书，也未发布正式包；门户技术栈、部署方式、企业身份系统和证书持有方仍待选择。

两类品牌分别交付：

- 运行时品牌：页面、窗口标题、菜单、托盘、可动态替换的 Dock / 窗口图标。
- 发行品牌：`.app` 名称、系统安装图标、Windows 产品属性与快捷方式、安装界面。由构建期替换并重新签名，不在员工运行中的已签名程序内修改。

## 2. 当前源码约束

| 当前锚点 | 已核验行为 | 企业发行需要改变的契约 |
| --- | --- | --- |
| [Makefile](../../Makefile): `app` | 固定生成 `magpie.app`，复制 `.icns`、`Assets.car`、固定 plist | 参数化包名称、展示字段和品牌资产；每个任务使用独立 staging，不能改共享工作区 |
| [Makefile](../../Makefile): `release-windows` | 使用 `go-winres` 生成多尺寸图标、版本、manifest 后编译 amd64 / arm64 EXE | 用企业资源生成器替代硬编码品牌，最终 EXE 再签名 |
| [macOS plist](../../build/darwin/Info.plist) | 展示名、bundle ID、可执行文件、图标和协议均为固定值 | 区分展示名、稳定企业身份、技术兼容标识；不能全文件替换 magpie |
| [Windows 资源](../../build/windows/winres.json) | ProductName / FileDescription / CompanyName、manifest 身份和图标固定 | 参数化企业展示与稳定身份；保留真实法律署名 |
| [发布工作流](../../.github/workflows/release.yml) | 将发布请求交给 `yetone/magpie-releases`；注释说明签名秘密保存在那里 | 企业流水线使用获授权的企业发布空间和签名服务，不调用上游发布账号或借用其证书 |
| [更新器](../../internal/update/update.go): `Feed`、`Stage`、`sameSigner` | 默认 `usemagpie.ai/api/latest`；macOS 解包固定找 `magpie.app`；验证同一签名 Team；Windows 二进制更新以摘要校验为主 | 企业更新源、包入口、企业身份及 Windows 签名验证必须配套改变，不能只换安装图标 |
| [更新镜像](../../internal/update/mirror.go) | 镜像只替换下载地址，版本和摘要仍来自原 feed | 镜像不是企业更新渠道，不能用 mirror 配置代替企业 feed |
| [目录管理](../../internal/appdir/appdir.go)、`singleInstance` | 原有配置 / 缓存目录是通用 magpie 身份；Windows 单实例 ID 依赖配置目录 | 企业发行身份隔离状态，不能仅改 bundle ID 后继续共享旧状态 |
| [启动项](../../internal/autostart/autostart_darwin.go)、[Windows 启动项](../../internal/autostart/autostart_windows.go) | LaunchAgent label 和 Run value 固定；Mac AssociatedBundleIdentifiers 从实际 bundle 读取 | 启动项使用企业稳定身份；继承系统中用户主动关闭的启动状态 |
| [Windows 协议](../../internal/gui/scheme_windows.go) | 每次启动注册 `magpie://` 到当前程序 | 企业与通用版共存时需定义协议归属，避免争抢或改写另一个发行版 |

项目使用 Wails `v3.0.0-beta.24`。该版本 `WebviewWindow.SetTitle`、托盘 `SetIcon` / `SetTooltip` 可用于动态更新；macOS App.SetIcon 使用 NSApp 图标接口，Windows 应用级 `setIcon` 是空实现，窗口初始图标优先读取 EXE 资源。Windows 运行时自定义窗口图标需要额外原生适配，重启不等于完成适配。这是锁定依赖源码核验结论，仍需逐平台实际验证。

## 3. 配置档案与身份

### 3.1 一份配置驱动所有品牌表面

平台保存版本化 `release-profile.json`。这是纯数据档案，不接受用户脚本、模板表达式、编译参数或任意文件路径。字段名称为拟议契约：

```json
{
  "schemaVersion": 1,
  "tenantId": "acme",
  "distributionId": "acme-assistant",
  "profileRevision": 1,
  "brand": {
    "displayName": "Acme Assistant",
    "companyName": "Acme",
    "logoAsset": "sha256:<normalized-master-png-hash>"
  },
  "identity": {
    "macBundleId": "com.acme.assistant",
    "windowsAppId": "com.acme.assistant",
    "artifactSlug": "acme-assistant",
    "urlScheme": "acme-assistant"
  },
  "enterpriseDefaults": {
    "visibleProviders": [],
    "showPluginProviderEntry": false,
    "showCustomProviderEntry": false,
    "publicPluginCatalog": false,
    "libraryEnabled": false
  },
  "updates": {
    "mode": "platform",
    "channel": "stable",
    "feedUrl": "https://downloads.example.com/acme-assistant/stable/manifest.json",
    "manifestKeyId": "acme-release-key-1"
  }
}
```

示例 URL、身份及摘要不是可用配置；构建时必须替换为已验证值。`enterpriseDefaults` 沿用客户端配置 schema，默认包含不可移除的 Copilot 和本机 Ollama；完整运行时默认名称和 logo 从 `brand` 取得，不另维护第二套不一致的字符串。

- `tenantId`：平台权限归属；员工下载包中的租户标识不是登录凭据。
- `distributionId`：产品发行身份，首次建立后固定。名称或 logo 改变不改变它。
- `profileRevision`：配置快照版本，每次发布使用不可变 revision。
- `artifactSlug`：受限 ASCII 文件标识，避免将任意名称拼入命令或路径；它不是应用显示名。
- `macBundleId` / `windowsAppId` / 自有 URL scheme：企业产品首次注册时确定，之后稳定。不同企业、通用版不能误用同一身份。
- 签名证书、notarization 凭据、存储密钥和私钥不进入档案、包或客户端 API。平台仅在内部任务记录中引用受控 `signingProfileId`。
- `manifestKeyId` 对应受信公钥：公钥在构建时嵌入，不能只相信 manifest 从同一个网络地址带来的新公钥。

企业产品可保留 bundle 内的 `magpie` 可执行文件名和 CLI 协议字节，外部 `.app` / 下载文件使用企业展示名或 artifact slug。稳定身份是明确新增的发行标识，不属于无差别替换技术标识。

### 图标双主题标准补充

上传及企业打包共用 `internal/brandicon`，禁止两套各自实现的缩放/着色规则。以可见图形生成 1024px 透明画布，保持比例并居中到约 80% 可用区域；浅色界面使用深色单色图形，深色界面使用浅色单色图形，保留透明间隙。原始色值仅保留在规范化源图中。纯色背景保守处理，透明 PNG 为推荐输入，不承诺任意照片抠图。

安装图标另生成浅/深色标准圆角底板及对应高对比图形，避免 Finder/Explorer 的浅色背景无法看见白色透明 Logo。默认安装资源采用中性浅色底板；应用内、Dock/窗口/About 随设置/系统主题选用对应资源，macOS 菜单栏始终使用系统着色的 alpha 模板。资源包同时保留浅/深两套 PNG/ICNS，不把普通 ICNS 等同于已支持 macOS 自适应 Assets.car。

选择图片后先处理并展示双主题预览，不直接保存企业设置；处理完成后才可保存，取消后过期上传响应不得写回草稿。旧版冻结 PNG 的可信源摘要保持不变，再生成新的主题派生资源。该标准用于当前编译式打包，也应复用于后续“基础程序＋发行配置”的免编译封装；后者已实现为可复用 runtime + 签名前的包内配置/资源封装，见发行操作文档。

### 3.2 版本与状态目录

记录 `sourceCommit`、上游基础版本、企业 `releaseVersion`、平台 `buildNumber`、profile revision、目标架构和构建工具链。改变 logo 或功能默认值也要产生新发行版本，不能复用旧版本号让更新器认为没有变化。

macOS bundle version、Windows 数字版本资源与企业发行版本之间需定义可验证映射，并检查系统允许的范围。应用内同时展示企业版本和基础源码来源，避免用企业版本冒充上游发布版本。

配置、缓存、备份、更新 staging、单实例 ID、启动项和相关 OS 凭据命名空间以稳定 distribution ID 隔离。不能把所有状态简单复制到一个新目录：已有 Agent 凭据仍按原读取规则读取，企业自身保存的记录按归属隔离。与通用版数据互通只通过用户确认的显式导入，不默认复制凭据。

还需处理多个发行版共同扫描 Agent 文件、使用同一网关端口及注册外部工具的冲突。第一版允许安装并存，但不承诺同时管理同一 Agent；检测冲突并提示选择管理实例，禁止静默覆盖。

## 4. 包内配置与本机配置的优先级

有效运行配置按以下来源计算：

```text
程序轻量默认值
  -> 已签名发行包内的企业默认值与 logo
  -> 本机显式覆盖（仅运行时可编辑字段）
```

- 包内档案作为受签名保护的只读资源嵌入或随 bundle 签名，不从任意可写旁置文件加载为可信默认。
- 第一次启动没有本机覆盖时直接使用包内档案，不把整套默认值复制为永久本机设置，否则后续发行默认值不能更新。
- `EnterpriseConfig` 扩展 `overrideFields`，记录用户明确覆盖的字段；未列入的字段从当前包继承。UI 的完整有效快照不是全字段覆盖声明，只有明确修改的字段进入覆盖集。
- 提供“恢复发行包默认”操作：删除对应覆盖，而不是把名称改回 magpie。通用构建无企业档案时才回退原品牌。
- 已有本机企业配置迁移为显式覆盖，保留用户选择，不能因为首次安装企业包而悄悄覆盖它。不同 distribution ID 的配置不相互读取。
- `KeepOwn` 继续保护本机覆盖；包内企业默认由安装 / 升级提供，不靠 WebDAV / S3 将其他电脑的默认档案写入。
- 品牌运行时覆盖只改变应用内和可变原生表面。修改品牌时 UI 提示“系统安装名称和图标仍来自发行包”；要求系统表面一致时在平台生成新包。
- 稳定身份、更新源、信任公钥和安装路径不是普通企业设置的可编辑字段。是否未来增加“企业托管锁定运行时字段”须单独批准；当前保持原需求中的可编辑能力，平台配置默认值不等于管理员强制策略。

保存来源、包内 profile revision、本机覆盖项和静态安装品牌应可在企业页查看，避免员工不清楚当前配置从哪里来。包内配置损坏或身份不符时拒绝企业激活 / 更新并给出恢复提示，不能静默退回全开放或上游更新渠道。

## 5. logo 生成与替换方法

平台上传图片沿用安全解码原则；发行主图建议 1024×1024 透明 PNG，并明确最小分辨率、文件大小和像素上限。原始资产保留不可变摘要；规范化去除无关元数据，生成阶段输出各平台资产及哈希。

| 目标 | 生成内容 | 替换方法 |
| --- | --- | --- |
| Web UI | 标准 PNG / favicon、浅深主题可读预览 | 由品牌 helper / boot 使用同一发行 logo，不保留旧小鸟回退 |
| macOS 旧系统 | 16 / 32 / 128 / 256 / 512 及 2x iconset、`.icns` | staging Resources 写入企业图标，并生成对应 plist 图标字段 |
| macOS 新图标机制 | `.icon` / `Assets.car` | 有匹配 Xcode 工具链时从企业资产生成；不能复用原 Magpie Assets.car。无该工具链时采用已验证的 `.icns` 回退并不宣称自适应图标支持 |
| macOS 托盘 | 小尺寸单色模板图及可选彩色图 | 不将完整 app 背景卡片直接当菜单栏模板；预览后使用，原小鸟振翅动画停用 |
| Windows | 16 / 32 / 48 / 64 / 128 / 256 图标资源及安装 / 快捷方式图标 | 在编译前生成企业 `winres.json` 和 `.syso`，不能在签名后修补 EXE |

生成器处理名称的空格、中文、长名称和路径字符，使用 JSON / plist 编码库和受控进程参数，不对 Makefile / shell 做任意字符串替换。每个构建任务在独立 checkout / staging 中生成资源；两企业并发任务不得共享 `.syso`、logo 或 manifest 文件。

### 构建输入到文件的映射

| 档案输入 | 生成 / 替换目标 | 实施规则 |
| --- | --- | --- |
| 整份已校验 profile | 拟议 `internal/distribution` 的生成资源，随二进制嵌入 | 客户端只读此受签名保护的档案；平台草稿和签名秘密不被复制 |
| displayName、企业 bundle ID、版本 | staging 的 `Contents/Info.plist` | 用 plist 编码器设置具体 key，保留可执行文件、兼容协议及版权字段的真实语义 |
| 企业 Mac 图标 | staging 的 `Contents/Resources` | 写入企业 `.icns` / Assets.car，并同步图标 key；不能只替换 PNG 但仍保留原 Assets.car |
| Windows 名称、图标、身份、版本 | 任务私有 `winres.json` 和编译输入 `.syso` | 用 `go-winres` 或等价生成器制作资源，再构建 EXE；不对签名后文件二次写入 |
| 标准化 logo、托盘图 | 拟议嵌入品牌资产，替代运行时对固定 `icon.png`、`icon-1024.png`、`tray.png` 的默认选择 | 页头、原生应用和托盘从同一发行品牌读取，不能让某一初始化 / 刷新路径再次使用原小鸟 |
| distribution ID、更新模式 / 公钥 | 拟议客户端发行元数据 | 启动时先确定发行身份，再打开配置目录、注册启动项及执行更新；不依赖后加载的用户设置确定身份 |

当前 `make icons` / `build/icon/gen.go` 为原小鸟生成资源，不能直接用于企业 logo 并宣称已经替换。新增企业图像生成路径应从校验后的用户 bitmap 缩放 / 转换，不覆盖通用版素材；企业 target 使用生成结果，通用 target 保持不变。

展示名称不直接作为目录或命令参数模板。构建前额外验证包文件名；名称含平台不允许的文件名字符时，使用受控包名并在预览中说明，不能产生路径穿越或静默改变企业展示文字。

## 6. macOS 发行流程

1. 校验 profile、资产摘要、企业 bundle ID 和目标架构，冻结源码 SHA 与构建工具链。
2. 在 macOS 构建节点生成企业 GUI 二进制；第一期分别输出 arm64 / amd64，Universal 包为后续可选项，不假设现有流水线已支持。
3. staging 组装 `<DisplayName>.app`；生成 CFBundleName / CFBundleDisplayName、企业 bundle ID、版本和图标，复制只读发行配置。可执行文件名与 plist 保持相符。
4. 正确签名嵌套可执行内容，再签名外层 `.app`；使用获授权 Developer ID Application、必要 entitlements、hardened runtime 和时间戳，不用 ad-hoc 签名冒充正式发布。
5. 提交 notarization 并等待接受；将可 staple 的公证票据写入 app / 容器后验证。最终 ZIP 摘要在签名 / staple / 压缩全部完成后计算。
6. 第一阶段交付 `.app` ZIP，可额外提供 DMG；若未来使用 PKG / MDM，另选安装器签名与安装策略，不能把它们视为现有能力。
7. 验证系统名称、Finder / Dock 图标、Login Items、菜单、通知、首次启动以及升级；审核后发布。

修改签名后的 plist、图标或档案会改变签名保护内容，必须重新签名 / 公证后发布。不得让员工客户端自改安装包。签名 Team / 身份需保持更新连续性；更换签名主体走显式迁移，不能绕过检查。

企业更新 ZIP 的入口由受验证 manifest 提供或从企业稳定元数据推导，不能继续写死 `magpie.app`。入口限定在解包根内的单个合法 `.app`，核验 bundle ID、distribution ID、版本和签名；不接受路径遍历或外链作为入口。

## 7. Windows 发行流程

1. 按 profile 生成企业多尺寸图标、ProductName、FileDescription、CompanyName、manifest 身份、版本资源和只读发行配置。
2. 每任务独立生成 `.syso`，构建 amd64 / arm64 GUI EXE；确认嵌入资源、运行时品牌与平台选择一致。
3. 首阶段提供已签名 EXE / ZIP；开始菜单、卸载项和安装路径需安装器或受管部署负责，裸 EXE 下载不等于已有安装能力。
4. 后续可选 MSI / MSIX，先决定安装器技术和企业部署策略，再实现稳定 UpgradeCode / package identity / Publisher 等相应契约，不混用不同安装类型的标识。
5. 完成所有 EXE 资源替换后执行 Authenticode 签名及可信时间戳；安装器如存在也签名。客户端更新除摘要外验证受信发行者和签名状态。
6. 验证 Explorer、程序属性、运行窗口 / Alt+Tab、托盘、启动项及快捷方式。已固定任务栏快捷方式可能缓存旧图标，升级时更新自有快捷方式并验证，不操作系统图标缓存强迫全局刷新。

Windows App.SetIcon 的空实现需要针对运行时覆盖增加适配；发行资源本身仍在编译期完成。证书签名不保证 SmartScreen 已获得信誉，分发平台不能承诺“签名后无任何警告”。

## 8. 未来分发平台工作流

```text
企业管理员登录平台
  -> 创建企业产品（固定发行身份）
  -> 配置名称、logo、供应商名单和功能默认值
  -> 查看应用内 / 系统图标预览，保存不可变 profile revision
  -> 选择受支持源码版本、架构、渠道和签名档案
  -> 提交构建任务
  -> 隔离构建 / 签名 / 公证 / 检查
  -> 审核并发布不可变产物与更新 manifest
  -> 员工在所属企业下载页选择 macOS / Windows 与架构
  -> 下载并安装，首次启动使用包内企业配置
```

平台职责分为 profile / 资产管理、任务队列、平台构建节点、受控签名服务、产物仓库、审核发布、企业下载页与更新渠道。配置页只提交数据，不能从浏览器直接调用签名密钥或执行构建命令。

建议状态机：`draft → validated → queued → building → signing → verifying → ready → published`。macOS 公证是 signing 的可观察子阶段；失败保持失败原因和日志，不自动进入 published。每 OS / 架构有单独结果，下载页只列已验证产物；配置未改变的重试复用冻结输入，不读取已被管理员修改的新草稿。

建议未来 API 能力：profile 创建 / 校验 / revision、logo 上传、构建创建 / 状态、产物列表、审核发布、下载授权与渠道 manifest。具体路径待平台实现时确定，不把示意 API 视为现有服务。

- 平台验证管理员与下载者的企业归属；任务、日志、资产和产物不可跨租户枚举或下载。
- 员工通过企业账号获得有时限的下载 URL，不在包里内置员工 token、企业 API key、Copilot 登录或私钥。
- 配置档案可以包含非敏感企业身份和默认目录，但敏感连接信息应通过员工登录或部署阶段注入，不能作为公开包资源。
- 短期下载 URL 与稳定更新入口分开；不把会过期的员工下载链接嵌入长期运行的客户端。
- 不信任用户输入的任意源码 URL、脚本、证书、feed 主机或外部 logo URL。来源版本和服务域名使用平台批准范围，避免签名任意程序、命令注入和 SSRF。

## 9. 企业更新、回滚与重配置

### 9.1 更新渠道

企业包必须显式选择以下一种模式：

- `platform`：嵌入企业 feed 和信任公钥，接收同 distribution ID 的企业更新。
- `manual`：关闭自更新，由员工重新下载或受管部署更新。

两种模式都不回退到上游 feed；企业发行包始终忽略 `MAGPIE_UPDATE_FEED`，该环境变量仅保留通用版既有测试用途。上游 Magpie 发布不触发企业版本升级；需要重新构建企业包并发布到该企业下载频道。

员工自助下载的标准模式为 `platform`。分发 API 按 `BaseURL`、tenant ID、distribution ID 和 channel 自动生成稳定 feed，拒绝公开 Magpie、其他租户或其他频道的 feed；独立构建则由发行方显式嵌入自有 HTTPS feed。短期下载链接不作为版本检测地址。设置“关于”展示发行频道和更新服务器，失败时只允许重试企业检测，不提供公开 GitHub / 镜像回退。

`manual` 仅用于明确关闭应用内检测的离线 / 受管部署。页面显示通过企业分发渠道下载新版的说明，不报“检测失败”，不提供无效检查按钮、自动更新开关、检测间隔或无源更新内容入口。当前 QA 测试包为该模式，不代表已接入生产分发平台。

更新 manifest 至少含 distribution ID、渠道、releaseVersion、单调发布序号、profile revision、平台 / 架构、入口、URL、大小、最终 SHA-256、签名 key ID 与签名。认证 canonical bytes，绑定所有安全字段；校验身份、渠道、架构、签名和版本后才允许下载及替换。

macOS 保留有效签名及受信 Team 校验，Windows 增加 Authenticode / 发行者校验；manifest 公钥签名不替代 OS 签名。信任 key 轮换需要旧受信 key 授权或安装新受信包，不接受网络端任意替换公钥。签名或证书读取失败不能当作相同发行者。

现有 `AppAsset` / `BinaryAsset` 命名、`Stage` 入口、裸 EXE 更新、GUI / CLI 更新入口和退出时安装更新路径都要核对。CLI `magpie update` 不得成为切回通用版的旁路。

品牌配置改变也发布新的企业发行版本。更换上游源码版本时，以原企业 profile 重新构建，不能直接拿通用上游二进制替换企业包。

### 9.2 回滚与重配置

- 配置草稿变更不会修改已发布包或已安装员工客户端。发布新 revision 后，员工通过升级获得静态品牌及未覆盖的运行默认值。
- 平台提供“新包已发布 / 需重新安装或升级”的状态，不显示“所有员工已实时生效”。远程运行时配置推送是后续独立功能，不在本期默认加入轮询或遥测。
- 下架阻止新的下载；它不能自动删除员工已下载文件，也不是供应商凭据撤销机制。
- 回滚由管理员审批，将已验证旧源码 / profile 重新发布为更高发布序号的受信版本，并校验配置 schema 兼容性，不能简单关闭防回退检查。
- 更新先 staging 校验，再安全替换，失败保留原可运行版本；本机显式覆盖、账号和库内容不因包回滚被清空。

## 10. 安全与发布证据

- 签名私钥集中保管在受控签名服务 / 证书仓库中；授权 scope、租户与签名主体绑定，不能把本企业任务转交其他企业签名。
- 构建器只接受批准的源码和工具链，生成资产与签名步骤分权；非可信用户内容不能在可访问私钥的环境执行。
- 保留源码 SHA、依赖锁定、profile / 原始及生成 logo 摘要、构建工具链、签名身份、验证输出和最终产物摘要；建议附 SBOM 与构建溯源。
- 产物及 profile revision 不可覆盖。发布 feed 在验证后原子指向已存在的产物，不允许页面显示完成但下载文件尚未就绪。
- 文件摘要证明下载一致性，受信签名证明发布身份，企业鉴权证明可下载权限；三者职责不同，不能只靠 SHA-256 宣称可信发布。

## 11. 分阶段交付和验收

### E1：本地可配置企业发行

已通过 `cmd/enterprise-release prepare|build` 实现 profile schema、logo 生成器、只读包内档案、本机覆盖和独立企业包输出。正式签名、公证和安装验证仍须使用授权证书及对应平台；不能把未签名开发包等同于生产发行。

### E2：企业构建服务与员工下载

平台接入同一构建接口，实现租户配置、不可变 revision、队列、签名 / 公证状态、审核、产物与下载授权。无需员工在首次启动前访问配置服务。

### E3：企业更新与受管部署扩展

实现受信更新 manifest、灰度 / 回滚及可选 MSI / MSIX / PKG / MDM 分发；远程托管策略、应用内企业登录和配置推送另立需求，不从下载平台能力推断其已具备。

| 编号 | 验收场景 | 必须证明 |
| --- | --- | --- |
| B01 | 同一 profile 生成两平台包 | 页面、窗口、托盘、系统包名 / 图标和程序属性一致，技术标识及法律来源保持真实 |
| B02 | 员工全新安装、无网络启动 | 使用包内品牌及默认功能，不需要手动配置或配置服务在线 |
| B03 | 系统安装名改名 / 中文 / 空格 / 相似名称 | 正确编码、不产生路径注入，发行 ID 和状态目录不随显示名变化 |
| B04 | 两企业并发构建，通用版并存 | 资产、`.syso`、配置、签名、产物不串租户；单实例 / 启动项 / 端口 / Agent 冲突有准确处理 |
| B05 | 首次启动、显式覆盖、恢复默认、升级 profile | 未覆盖项继承新版默认，覆盖项保留；恢复是包默认而不是原 Magpie 品牌 |
| B06 | 已签名包资源被修改、manifest 篡改、错误身份 / 架构 | 验证拒绝并保留原版本；无上游 fallback；未知证书 / key 不被信任 |
| B07 | 企业升级、新 profile 发布、GUI / CLI 更新、退出安装 | 不替换为通用包，不丢企业身份、本机数据或签名连续性 |
| B08 | Mac 公证失败、Windows 签名失败、某架构构建失败 | 不发布失败产物，平台显示真实状态；对应证据由真实平台验证而非只编译 |
| B09 | 员工下载权限、过期 URL、跨租户请求 | 未授权不可下载；下载 URL 过期不破坏已嵌入的稳定更新策略 |
| B10 | 下架、回滚、离线及安装失败 | 已安装应用可继续按既有能力运行；回滚有审批与兼容校验，不清空凭据或 Agent 文件 |

## 12. 待确定的运营选项

- 分发平台部署位置、企业登录方式和产物存储；当前设计不预设 Azure 或某一种平台。
- 签名主体由企业持有还是发布服务持有，以及每产品允许的签名者 / 公钥轮换流程。
- 首阶段 ZIP / EXE 是否满足部署需求，何时增加 DMG、MSI / MSIX、PKG / MDM。
- 是否允许员工覆盖包内运行时品牌，未来是否引入托管锁定；本期仍保留原可编辑需求。
- 企业专有 scheme 与兼容 `magpie://` 的安装归属、企业 CLI 与通用 CLI 的 PATH 共存方式。默认不争抢原协议 / 命令，由安装策略显式选择并告知。

平台方法和配置接口可以先落实为 schema 与设计；以上运营选项未确定前，不执行生产构建、证书配置或企业发布。

## 免编译封装与共存补充（2026-10-12）

基础 runtime 按源码/OS/架构编译一次。`build --base-runtime` 复用校验过的基础程序，
根据 profile 和 logo 生成双主题图标、安装品牌、企业版本和发布身份，封装后再签名。
macOS 使用包内 enterprise-runtime.json；Windows 使用 PE RCDATA，签名覆盖配置。
员工端缺失/损坏配置时拒绝启动，不回退公开版身份或更新源。

进程/CLI 使用不可变的 artifactSlug；安装配置与缓存目录使用同名企业根目录，
不再位于公开 magpie 根目录下。独立端口、锁、bundle/App ID、scheme、启动项和
快捷方式避免抢占公开客户端。旧企业目录迁移为显式操作；不替换公开 magpie 命令。
同时显式接管同一个外部 Agent 的配置不在隔离保证之内。
