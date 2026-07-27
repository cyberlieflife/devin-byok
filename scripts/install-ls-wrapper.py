
import hashlib, json, os, shutil, subprocess, time
from pathlib import Path

BIN = Path(r"D:\Devin\resources\app\extensions\windsurf\bin")
TARGET = BIN / "language_server_windows_x64.exe"
REAL = BIN / "language_server_windows_x64.real.exe"
WRAPPER_SRC = Path(r"D:\Devin-byok\devin-byok-ls-wrapper.exe")
META = Path(os.environ["APPDATA"]) / "devin-byok" / "ls-wrapper-install.json"

def sha256(p: Path) -> str:
    h = hashlib.sha256()
    with p.open("rb") as f:
        for chunk in iter(lambda: f.read(1024*1024), b""):
            h.update(chunk)
    return h.hexdigest()

def main():
    if not TARGET.exists():
        raise SystemExit(f"missing {TARGET}")
    if not WRAPPER_SRC.exists():
        raise SystemExit(f"missing wrapper build {WRAPPER_SRC}")

    # if already wrapped, just refresh wrapper binary
    already = REAL.exists()
    ts = time.strftime("%Y%m%d_%H%M%S")
    backups = []

    if not already:
        # backup original first (hard rule)
        bak = BIN / f"language_server_windows_x64.exe.bak_{ts}"
        shutil.copy2(TARGET, bak)
        backups.append(str(bak))
        # verify backup size
        if bak.stat().st_size != TARGET.stat().st_size:
            raise SystemExit("backup size mismatch")
        # move original to .real
        if REAL.exists():
            REAL.unlink()
        TARGET.rename(REAL)
        # delete bak after successful rename+copy? keep bak until wrapper verified
    else:
        # backup current wrapper/target
        bak = BIN / f"language_server_windows_x64.exe.wrapperbak_{ts}"
        shutil.copy2(TARGET, bak)
        backups.append(str(bak))

    shutil.copy2(WRAPPER_SRC, TARGET)
    meta = {
        "installed_at": time.strftime("%Y-%m-%dT%H:%M:%S"),
        "target": str(TARGET),
        "real": str(REAL),
        "wrapper_sha256": sha256(TARGET),
        "real_sha256": sha256(REAL),
        "backups": backups,
        "api": "http://127.0.0.1:8787/_route/api_server",
    }
    META.parent.mkdir(parents=True, exist_ok=True)
    META.write_text(json.dumps(meta, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(meta, ensure_ascii=False, indent=2))
    # cleanup old bak if sizes ok - keep one bak for safety until user confirms
    print("INSTALL_OK")

if __name__ == "__main__":
    main()
