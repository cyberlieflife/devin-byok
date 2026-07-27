
import json, os, shutil
from pathlib import Path
BIN = Path(r"D:\Devin\resources\app\extensions\windsurf\bin")
TARGET = BIN / "language_server_windows_x64.exe"
REAL = BIN / "language_server_windows_x64.real.exe"
META = Path(os.environ["APPDATA"]) / "devin-byok" / "ls-wrapper-install.json"

if not REAL.exists():
    raise SystemExit("real binary missing; nothing to uninstall or already restored")
# backup current wrapper
import time
bak = BIN / f"language_server_windows_x64.exe.wrapper_before_restore_{time.strftime('%Y%m%d_%H%M%S')}"
if TARGET.exists():
    shutil.copy2(TARGET, bak)
TARGET.unlink(missing_ok=True)
REAL.rename(TARGET)
print("restored", TARGET)
if META.exists():
    print("meta", META)
print("UNINSTALL_OK")
