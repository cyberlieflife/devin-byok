# devin-byok

让 **Devin**（原 Windsurf）使用你自己的模型服务（BYOK）。

- OpenAI 兼容 API / Anthropic Messages
- 免登录（pure_local）
- 图形界面一键启停、监控、模型管理
- 协议：**AGPL-3.0**（见 [LICENSE](LICENSE)）

仓库：https://github.com/cyberlieflife/devin-byok

## 用户怎么用（发布包）

发布包 **只包含 3 个文件**：

```
devin-byok-gui.exe
config.example.yaml
START.txt
```

1. 解压 zip  
2. 复制 `config.example.yaml` → `config.yaml`，填写供应商  
3. 双击 `devin-byok-gui.exe`  
4. 在 GUI 点 **启动服务**（会自动 apply 到 Devin）  
5. **完全退出并重开 Devin**，选择 BYOK 模型  

停止：GUI 点 **停止服务**（会 restore Devin 设置）。

> 发布包内服务跑在 GUI 进程中，无需单独的 CLI。

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

## 许可证

GNU Affero General Public License v3.0 — 见 [LICENSE](LICENSE)。
