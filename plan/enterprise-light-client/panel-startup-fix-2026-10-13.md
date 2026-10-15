# 菜单栏面板 Agent 与额度初始化修复

## 原因与修复

合并 main 时，为修复中文顶部导航尺寸变化，头部 ResizeObserver 增加了 `#nav`。
菜单栏面板会在初始化中移除 `#nav` 与侧栏，却仍将空节点传给观察器。
这会抛出 `ResizeObserver.observe` TypeError，中止后续初始化，导致 Agent 状态
没有读取、额度页无法正常加载。不是用户 Agent 配置被清空。

修改边界：

- 只观察仍存在的 DOM 元素。主窗口继续观察导航，菜单栏面板跳过不存在的导航。
- 将尺寸适配放在下一动画帧，避免在 ResizeObserver 回调中改变被观察宽度而触发
  `ResizeObserver loop completed with undelivered notifications`。
- 不放宽企业可见 Agent 名单，不更改网关、配置目录、Agent 接入文件或账号。
- 不关闭原生面板的失焦自动隐藏；面板内部切换标签无需打开主窗口或隐藏面板。

## 验证与隔离

- 新浏览器测试在修复前 22/22 真实失败，错误明确指向对空节点调用观察器。
  修复后 Chromium/WebKit、企业/公共面板、五种语言全部通过。
- 新 macOS 测试使用真实 WKWebView、原生面板选项、125% page zoom 和合成 API。
  修复前真实失败，修复后能够显示配置的 WorkBuddy，切换额度、返回 Agent，窗口仍打开。
- 原生测试通过 `runAppKit` 隔离 HOME、AppKit/WebKit 目录，没有启动业务网关或读取
  员工账号。测试在交互前等待 AppKit 确认窗口获得焦点，避免将初始化中的 Show/Focus
  异步切换误判成额度点击引发失焦。最终源代码连续运行 5 次通过。
- 扩展回归同时覆盖菜单栏、顶部布局、侧栏、缩放、额度打开及企业页面。侧栏测试的
  旧设置列表缺少企业配置，已更新为实际的九个标签，而不是忽略断言。
  最终重新运行该组测试，320/320 通过，记录在 `ui-regressions-final.log`。
- GUI Go 包测试以及 macOS/Linux/Windows 交叉 vet 通过。未测试 Windows 原生应用，
  未重跑全仓库网关长耗时测试或全部 GUI 测试文件。

日志：`/tmp/magpie-enterprise-tools/panel-fix/`。

## macOS 测试包

- `msai-client 1.0.11`，build 13，ARM64，未签名、未公证。
- 目录：`/Users/edwin/.codex/artifacts/msai-client-1.0.11-macos-arm64`。
- 包含 1.0.10 的 ZIP 插件名称、可见供应商入口及缓存刷新修复。
- 维持 profile revision 2、企业身份、Logo、CLI 和配置路径，不重置员工设置。
- 来自当前企业分支未提交工作区，不是生产发布包。
- 已核对 Info.plist、企业资源、程序哈希、ZIP 每个应用文件，并验证包内嵌的
  `app.js`、`enterprise.js`、`plugins.js`、`plugins.css` 与当前测试源逐字节一致。
- 出包程序 `--version` 输出 `msai-client 1.0.11`；没有替换或重启已安装应用。
