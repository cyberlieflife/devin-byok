package payload

import _ "embed"

// ConfigExample 内置默认配置模板（GUI 单文件首次启动可写出 config.yaml）。
//
//go:embed config.example.yaml
var ConfigExample []byte

// LSWrapper 内置 language_server 包装器（无需旁路 devin-byok-ls-wrapper.exe）。
//
//go:embed ls-wrapper.exe
var LSWrapper []byte
