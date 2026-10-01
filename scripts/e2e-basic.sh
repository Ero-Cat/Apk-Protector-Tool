#!/usr/bin/env bash
# e2e-basic.sh — 无外部依赖的端到端冒烟测试（CI 可跑）：
#   构建 protector -> 构造含扫描特征的测试 APK -> scan 断言 -> 错误路径断言
# 需要：go、zip、python3。可选：clang（跑 C 解释器单测）。
set -euo pipefail
cd "$(dirname "$0")/.."

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

echo "==> build protector"
GOCACHE="$PWD/.gocache" go build -o "$WORK/protector" ./cmd/protector

echo "==> fabricate test APK"
mkdir -p "$WORK/pkg/lib/arm64-v8a"
printf '\x03\x00\x08\x00com.example.e2e' > "$WORK/pkg/AndroidManifest.xml"
printf 'dex\n035\x00frida gadget\nxposed bridge\nmagisk su\n' > "$WORK/pkg/classes.dex"
printf '\x7fELF\x02\x01\x01\x00libdemo.so\x00' > "$WORK/pkg/lib/arm64-v8a/libdemo.so"
(cd "$WORK/pkg" && zip -q -r -X "$WORK/test.apk" AndroidManifest.xml classes.dex lib)

echo "==> scan subcommand assertions (isolated cwd)"
mkdir -p "$WORK/run"
(cd "$WORK/run" && "$WORK/protector" scan -input "$WORK/test.apk" -report "$WORK/report.json")
python3 - "$WORK/report.json" <<'PY'
import json, sys
r = json.load(open(sys.argv[1]))
steps = [(s["name"], s["status"]) for s in r["steps"]]
assert ("scan", "completed") in steps, f"scan step missing: {steps}"
assert not any(s[0] in ("protections", "sign", "zipalign") for s in steps), steps
findings = r["scan"]["anti_environment"]
assert "frida" in findings and "xposed" in findings and "magisk" in findings, findings
assert r["final_apk"] == "", "scan-only must not produce an output artifact"
print("scan assertions: PASS")
PY

echo "==> scan-only produces no APK"
if [ -d "$WORK/run/dist" ] && [ -n "$(ls -A "$WORK/run/dist" 2>/dev/null)" ]; then
  echo "scan-only unexpectedly wrote artifacts:"; ls -A "$WORK/run/dist"; exit 1
fi
echo "no artifact: PASS"

echo "==> error paths"
out=$( ("$WORK/protector" run -profile turbo -input "$WORK/test.apk" 2>&1 || true) )
if echo "$out" | grep -q 'unknown profile'; then
  echo "invalid profile: PASS"
else
  echo "invalid profile rejection missing: $out"; exit 1
fi
out=$( ("$WORK/protector" run -store-pass-env E2E_DEFINITELY_UNSET -input "$WORK/test.apk" 2>&1 || true) )
if echo "$out" | grep -q 'E2E_DEFINITELY_UNSET'; then
  echo "unset env guard: PASS"
else
  echo "unset env guard missing: $out"; exit 1
fi

echo "==> ui non-TTY guard"
if "$WORK/protector" ui < /dev/null > /dev/null 2>&1; then
  echo "ui should fail without a TTY"; exit 1
fi
out=$( ("$WORK/protector" ui < /dev/null 2>&1 || true) )
if echo "$out" | grep -q 'interactive terminal'; then
  echo "ui guard message: PASS"
else
  echo "ui guard message missing: $out"; exit 1
fi

echo "==> config init"
(cd "$WORK" && "$WORK/protector" config init -o cfg.yml && test -s cfg.yml)

echo "==> key externalization (P3.3, ADR-0001)"
mkdir -p "$WORK/plainpkg"
printf '\x03\x00\x08\x00com.example.keytest' > "$WORK/plainpkg/AndroidManifest.xml"
printf 'dex\n035\x00e2e key payload' > "$WORK/plainpkg/classes.dex"
(cd "$WORK/plainpkg" && zip -q -r -X "$WORK/plain.apk" AndroidManifest.xml classes.dex)
cat > "$WORK/keytest.yml" <<YML
input_apk: $WORK/plain.apk
final_output: $WORK/out/keytest-protected.apk
work_dir: $WORK/wd
protections:
  enabled: true
  dex_encrypt: true
  encryption_secret: e2e-secret
YML
(cd "$WORK" && "$WORK/protector" run -config "$WORK/keytest.yml" > /dev/null)
test -s "$WORK/out/keytest-protected.apk" || { echo "protected apk missing"; exit 1; }
test -s "$WORK/out/keytest-protected.apk.key" || { echo "external key file missing"; exit 1; }
python3 - "$WORK/out/keytest-protected.apk" "$WORK/out/keytest-protected.apk.key" <<'PY'
import base64, json, sys, zipfile
apk_path, key_path = sys.argv[1], sys.argv[2]
raw = open(apk_path, "rb").read()
keymat = json.load(open(key_path))
aes_b64 = keymat["aes_key"]
aes_raw = base64.b64decode(aes_b64)
assert b"encryption_key" not in raw, "metadata embeds key field"
for needle in (aes_b64.encode(), aes_raw.hex().encode(), aes_raw):
    assert needle not in raw, f"artifact leaks key material ({len(needle)}B form)"
meta = json.load(zipfile.ZipFile(apk_path).open("assets/protector/metadata.json"))
assert meta.get("encryption_key", "") == "", "encryption_key must be empty by default"
assert len(meta["dex_encrypted"]) == 1 and meta["dex_encrypted"][0]["nonce"], meta
print("key externalization assertions: PASS")
PY

echo "==> optional: C interpreter unit tests"
if command -v clang >/dev/null 2>&1; then
  clang -std=c11 -Wall -Wextra -Werror -I runtime/include \
    runtime/tests/test_vm.c -o "$WORK/test_vm" && "$WORK/test_vm"
  clang -std=c11 -Wall -Wextra -Werror -I runtime/include \
    runtime/tests/test_hooks.c -o "$WORK/test_hooks" && "$WORK/test_hooks"
  clang -std=c11 -Wall -Wextra -Werror -I runtime/include \
    runtime/tests/test_integrity.c -o "$WORK/test_integrity" && "$WORK/test_integrity"
  clang -std=c11 -Wall -Wextra -Werror -I runtime/include \
    runtime/tests/test_antidebug.c -o "$WORK/test_antidebug" && "$WORK/test_antidebug"
  clang -std=c11 -Wall -Wextra -Werror -I runtime/android \
    runtime/android/tests/test_aes_gcm.c runtime/android/crypto/aes_gcm.c \
    -o "$WORK/test_aes_gcm" && "$WORK/test_aes_gcm"
  clang -std=c11 -Wall -Wextra -Werror -I runtime/android \
    runtime/android/tests/test_dex_decrypt.c runtime/android/dex_decrypt.c \
    runtime/android/crypto/aes_gcm.c -o "$WORK/test_dex_decrypt" && "$WORK/test_dex_decrypt"
else
  echo "clang not found, skipping"
fi

echo "ALL BASIC E2E CHECKS PASSED"
