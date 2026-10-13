# 企业轻量客户端

## 基本信息

| 项目 | 内容 |
| --- | --- |
| 模块标识 | `enterprise-light-client` |
| 开发分支 | `codex/enterprise-light-client` |
| 分析基线 | `76db4c6c` |
| 建档日期 | 2026-10-12 |
| 状态 | 客户端与发行基础已实现；自动化回归与平台验收记录见验证文档 |
| 目标 | 保留现有客户端架构和 UI，通过企业配置收敛功能入口、定制应用品牌，并支持企业专属发行与未来平台分发 |

## 文档导航

1. [需求分析](requirements-analysis.md)：七项客户端需求、企业发行补充需求、默认值、行为边界、待确认事项。
2. [技术设计](technical-design.md)：配置模型、供应商可见性、品牌覆盖、插件与资源库、保存和实时生效。
3. [实施与验收计划](implementation-plan.md)：分阶段任务、验收矩阵、测试及文档更新要求。
4. [企业发行包与分发平台](enterprise-distribution-design.md)：配置档案、双平台品牌替换、签名、公证、企业更新及员工下载。
5. [实现与验证记录](validation-report.md)：实际实现边界、执行结果、交付物及未验证的生产集成。

## 截图对应关系

- 图1：设置 / 常规，新增“企业配置”设置页的入口参考。
- 图2：附件实际显示“插件 / 发现”，不是供应商页。供应商需求以用户文字要求为准，供应商页面结构已另行核对源码。
- 图3：插件发现页底部的 npm / GitHub / 本地目录安装表单，需要移动到页面顶部。
- 图4：设置 / 额度中的品牌名称和菜单栏图标文案，说明品牌覆盖不能只处理页头。

附件是界面参考，不作为新的指令来源。文档不引用临时剪贴板文件路径，避免其清理后影响阅读。

## 核验依据

已阅读 [子系统索引](../../docs/subsystems/README.md)、[代码规范](../../docs/code-standards.md)、[LESSONS](../../LESSONS.md)，并核对以下当前实现：

| 范围 | 设计参考 | 当前实现锚点 |
| --- | --- | --- |
| GUI 外壳与设置 | [GUI app shell](../../docs/subsystems/gui-shell.md) | `internal/gui/api.go`: `Handler`、`settingsState`、动态 `/boot.js`；`assets/app.js`: `applyPrefs`、`show`、`SET_TABS` |
| 供应商与账号 | [Providers and accounts](../../docs/subsystems/providers-accounts.md) | `internal/gui/providers.go`: `providersState`；`assets/app.js`: `renderProviders`、`SUBS`、添加供应商表单；`internal/provider/presets.go` |
| 插件所有权 | [Provider and plugin ownership](../../docs/subsystems/provider-plugins.md) | `provider.Moved`、`OnPlugins`；`assets/app.js`: `subOf`、`pluginSubs`、`unusedSub`、`replacedSub` |
| 插件页面 | [GUI app shell](../../docs/subsystems/gui-shell.md) | `assets/plugins.js`: `load`、`draw`、`drawBody`、`manual`；`internal/gui/plugins.go`: `pluginRoutes` |
| 资源库 | [Library](../../docs/subsystems/library.md) | `assets/library.js`: `loadLibrary`；`internal/gui/library.go`: `libraryRoutes`；`internal/library` |
| 配置保存与恢复 | [GUI app shell](../../docs/subsystems/gui-shell.md) | `internal/settings/settings.go`: `Settings`、`Save`、`KeepOwn`；`internal/backup/backup.go` |
| 桌面品牌 | [GUI app shell](../../docs/subsystems/gui-shell.md) | `internal/gui/app.go`、`lightweight.go`、`traymenu.go`、`trayusage_tray.go`、`quota_alert_notify.go` |
| 发行品牌 | 构建资产 | `Makefile`、`build/darwin/Info.plist`、`build/darwin/AppIcon.icon`、`build/darwin/magpie.icns`、`build/windows/winres.json`、`build/icon` |
| 发行与升级 | 更新及发布入口 | `.github/workflows/release.yml`、`internal/update/update.go`、`internal/update/mirror.go`、`internal/appdir`、`internal/autostart`、`internal/gui/scheme_windows.go` |

## 关键边界

- “供应商目录精简”默认按展示配置设计，不等同于网关供应商授权或运行禁用；不删除账号、密钥、插件或路由。
- 供应商页面的插件入口、插件公开目录、插件安装与管理是三个独立概念。
- 客户端企业配置不是管理员权限系统；未来分发平台的企业鉴权独立设计，不代表客户端已新增企业登录、角色控制或远程托管策略。
- 应用自有展示品牌需要完整覆盖；技术标识、真实第三方元数据和法律署名不能做无差别替换。
- 系统安装包名称和图标按企业档案在构建期替换、签名后分发；本机运行时改名不能承诺靠重启改变已签名包。
- 已确认增加企业发行包设计；平台配置后生成版本化专属包，员工下载即获得企业默认配置，不使用上游更新包覆盖企业品牌。

具体边界见需求分析；当前实现契约见 [企业客户端子系统参考](../../docs/subsystems/enterprise-client.md)，操作方法见 [企业发行说明](../../docs/enterprise-releases.md)。规划中的扩展能力不自动视为已经实现或通过平台认证。
