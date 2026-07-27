# devin-byok

让 **Devin**（原 Windsurf）使用你自己的模型服务（BYOK）。

- OpenAI 兼容 API / Anthropic Messages
- 免登录（pure_local）
- 图形界面一键启停、监控、模型管理
- 协议：**AGPL-3.0**（见 [LICENSE](LICENSE)）

仓库：https://github.com/cyberlieflife/devin-byok

## 用户怎么用（发布包）

发布包**只包含 2 个文件**：

```
devin-byok-gui.exe
START.txt
```

1. 解压 zip  
2. 双击 `devin-byok-gui.exe`  
3. 在 GUI 配置模型/供应商（保存到 `%USERPROFILE%\.devin-byok\config.yaml`）  
4. 点 **启动服务**（自动 apply + LS wrapper）  
5. **完全退出并重开 Devin**，选择 BYOK 模型  

退出 GUI 会自动关闭服务并 restore。配置由 GUI 管理，包内不再附带 config 模板。

## 配置最少要写什么

```yaml
upstream:
  families:
    - uid: my-model
      label: My Model
      provider: openai
      base_url: https://api.example.com/v1
      api_key: "sk-xxx"
      upstream_model: gpt-4.1-mini
      context_window: 128000
      max_tokens: 8192
  models:
    - id: my-model-medium
      label: My Model Medium
      upstream_model: gpt-4.1-mini
      thinking: medium
      family_uid: my-model

features:
  pure_local: true
  enable_stream: true
  enable_cascade_tools: true

update:
  enabled: true
  repo: cyberlieflife/devin-byok
  asset_contains: windows-amd64.zip
  auto_apply: false
```

完整字段见 `config.example.yaml`。

## GUI 页面

| 页面 | 功能 |
|------|------|
| 监控 | 成功/失败、Token、缓存、DeepWiki/CodeMap、日志 |
| 模型 | Family 卡片；DeepWiki / CodeMap Fast·Smart 绑定 |
| 设置 | 工具/流式/pure_local、托盘自启、在线更新 |

## 开发者

```powershell
git clone https://github.com/cyberlieflife/devin-byok.git
cd devin-byok
go build -o devin-byok.exe ./cmd/devin-byok
go build -ldflags "-H windowsgui" -o devin-byok-gui.exe ./cmd/devin-byok-gui
```

本地仍可用完整 CLI：`install` / `start` / `stop` / `doctor` / `update`。

### 打发布包（仅 GUI）

```powershell
.\scripts\pack-release.ps1
# dist/devin-byok-<ver>-windows-amd64.zip
# 仅: devin-byok-gui.exe + config.example.yaml + START.txt
```

### 发版

```powershell
.\scripts\pack-release.ps1
git add -A && git commit -m "release: v1.2.0"
git tag v1.2.0
git push origin master --tags
gh release create v1.2.0 dist/devin-byok-1.2.0-windows-amd64.zip dist/devin-byok-1.2.0-windows-amd64.zip.sha256 --title v1.2.0
```

### 在线更新

GUI：设置 → 检查更新 / 下载并更新  

从 GitHub Releases 下载 `*windows-amd64.zip`，有 `.sha256` 则校验后替换 GUI。

## 数据路径

| 用途 | 路径 |
|------|------|
| 配置 | 程序目录 `config.yaml` |
| Devin 设置 | `%APPDATA%\Devin\User\settings.json` |
| apply 元数据 | `%APPDATA%\devin-byok\last-apply.json` |
| 指标 | `%APPDATA%\devin-byok\metrics.json` |
| GUI 偏好 | `%APPDATA%\devin-byok\gui.json` |

## 单文件 GUI

发布包只需 `devin-byok-gui.exe`（+ `START.txt`）：内嵌 local-api、默认配置模板、LS wrapper。

- 配置：`%USERPROFILE%\.devin-byok\config.yaml`（首次从内置模板生成，可在 GUI 修改）
- 日志：`%APPDATA%\devin-byok\gui.log`
- 退出 GUI：自动停止服务并 restore
- 检测到 Devin 安装目录时自动植入 language_server 包装器

## 许可证

GNU Affero General Public License v3.0 — 见 [LICENSE](LICENSE)。

## 赞赏支持

如果觉得项目好可以赞赏我支持一下谢谢喵

![赞赏码](Support.jpg)
