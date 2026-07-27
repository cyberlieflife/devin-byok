# Cascade tool checklist (P0)

## Pre
- enable_cascade_tools: true
- tools.mode: readonly | standard | full
- start serve + restart Devin

## Cases
1. read README.md
2. list_dir
3. grep/code_search
4. write_to_file
5. edit
6. run_command (full)

## Logs
toolsIn/toolsOut/mode ; tool_calls names=...
## Note: built-in search workspace limit

grep_search/code_search only work **inside the open Devin workspace**. Searching D:\\Devin-byok while workspace is D:\\Code\\DevinTest will fail. Open that folder in Devin, or use run_command (mode=full).
