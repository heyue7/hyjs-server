"""Build one controlled parallel APK without extracting or recompiling its payload."""
from __future__ import annotations

import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
SOURCE_PACKAGE = "com.yunchang.djsy.ewan.local"
TARGET_PACKAGE = "com.yunchang.djsy.ewan.multi"
SOURCE_LABEL = "黑衣剑士·自建服"
TARGET_LABEL = "黑衣剑士·自建服·双端"
ANDROID_NS = "http://schemas.android.com/apk/res/android"
SDK = Path.home() / "Library/Android/sdk/build-tools/33.0.0"
JAVA = Path.home() / "Library/Java/JavaVirtualMachines/openjdk-18.0.1.1/Contents/Home"
MANIFEST = "AndroidManifest.xml"


def sha256(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def run(*args: object) -> str:
    env = dict(os.environ, JAVA_HOME=str(JAVA))
    result = subprocess.run(["rtk", "proxy", *(str(a) for a in args)], cwd=ROOT,
                            env=env, text=True, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE)
    if result.returncode:
        # These SDK commands contain only the well-known local debug-key password.
        raise RuntimeError(f"{Path(str(args[0])).name} failed: {result.stderr[-2000:]}")
    return result.stdout


def chunks(data: bytes):
    if len(data) < 8 or struct.unpack_from("<HHI", data) != (3, 8, len(data)):
        raise ValueError("unexpected binary XML header")
    offset = 8
    while offset < len(data):
        kind, header, size = struct.unpack_from("<HHI", data, offset)
        if header < 8 or size < header or offset + size > len(data):
            raise ValueError("invalid binary XML chunk")
        yield offset, kind, header, size
        offset += size
    if offset != len(data):
        raise ValueError("truncated binary XML")


def read_pool(data: bytes):
    pools = [c for c in chunks(data) if c[1] == 1]
    if len(pools) != 1:
        raise ValueError("expected one manifest string pool")
    offset, _, header, size = pools[0]
    count, styles, flags, start, style_start = struct.unpack_from("<IIIII", data, offset + 8)
    if header != 28 or styles or style_start or start != header + 4 * count:
        raise ValueError("unsupported styled or nonstandard manifest string pool")
    strings, serialized = [], []

    def length8(at):
        first = data[at]
        return (((first & 127) << 8) | data[at + 1], at + 2) if first & 128 else (first, at + 1)

    def length16(at):
        first = struct.unpack_from("<H", data, at)[0]
        return (((first & 32767) << 16) | struct.unpack_from("<H", data, at + 2)[0], at + 4) if first & 32768 else (first, at + 2)

    for index in range(count):
        begin = offset + start + struct.unpack_from("<I", data, offset + header + 4 * index)[0]
        if flags & 256:
            units, at = length8(begin)
            length, at = length8(at)
            text = data[at:at + length].decode("utf-8")
            end = at + length + 1
            if data[end - 1:end] != b"\0" or len(text.encode("utf-16le")) // 2 != units:
                raise ValueError("invalid UTF-8 string pool entry")
        else:
            units, at = length16(begin)
            end = at + units * 2 + 2
            text = data[at:end - 2].decode("utf-16le")
            if data[end - 2:end] != b"\0\0":
                raise ValueError("invalid UTF-16 string pool entry")
        if end > offset + size:
            raise ValueError("string exceeds pool")
        strings.append(text)
        serialized.append(data[begin:end])
    return pools[0], flags, strings, serialized


def literal_attributes(data: bytes, strings: list[str]):
    attributes = []
    for offset, kind, header, size in chunks(data):
        if kind != 0x102:
            continue
        if header != 16:
            raise ValueError("unexpected XML element header")
        tag = strings[struct.unpack_from("<I", data, offset + 20)[0]]
        start, width, count = struct.unpack_from("<HHH", data, offset + 24)
        if width != 20 or offset + 16 + start + width * count > offset + size:
            raise ValueError("unexpected XML attribute layout")
        for index in range(count):
            at = offset + 16 + start + width * index
            ns, name, raw, value_size, zero, value_type, value = struct.unpack_from("<IIIHBBI", data, at)
            if value_size != 8 or zero != 0:
                raise ValueError("unexpected XML typed value")
            attributes.append((tag, "" if ns == 0xffffffff else strings[ns], strings[name],
                               raw, value_type, value))
    return attributes


def patch_manifest(original: bytes) -> tuple[bytes, list[dict]]:
    pool, flags, strings, serialized = read_pool(original)
    offset, _, header, size = pool
    changes, replacements = [], {}
    for tag, ns, name, raw, kind, value in literal_attributes(original, strings):
        wanted = (tag == "manifest" and ns == "" and name == "package") or (
            tag in ("application", "activity") and ns == ANDROID_NS and name == "label")
        if not wanted:
            continue
        before, after = (SOURCE_PACKAGE, TARGET_PACKAGE) if name == "package" else (SOURCE_LABEL, TARGET_LABEL)
        if kind != 3 or raw != value or strings[value] != before:
            raise ValueError(f"unexpected controlled manifest attribute: {tag}/{name}")
        replacements[value] = after
        changes.append({"element": tag, "attribute": name, "before": before, "after": after})
    if sorted((c["element"], c["attribute"]) for c in changes) != [
            ("activity", "label"), ("application", "label"), ("manifest", "package")]:
        raise ValueError("expected exactly one package and two original labels")
    # A shared string index must not change an unrelated attribute through aliasing.
    for tag, ns, name, raw, kind, value in literal_attributes(original, strings):
        if raw in replacements or (kind == 3 and value in replacements):
            if not ((tag == "manifest" and ns == "" and name == "package") or
                    (tag in ("application", "activity") and ns == ANDROID_NS and name == "label")):
                raise ValueError("changed string index is shared by an unrelated attribute")

    def len8(value):
        if value > 32767:
            raise ValueError("string too long")
        return bytes([value]) if value < 128 else bytes([(value >> 8) | 128, value & 255])

    def len16(value):
        return struct.pack("<H", value) if value < 32768 else struct.pack("<HH", (value >> 16) | 32768, value & 65535)

    encoded = list(serialized)
    for index, text in replacements.items():
        utf16 = text.encode("utf-16le")
        if flags & 256:
            utf8 = text.encode("utf-8")
            encoded[index] = len8(len(utf16) // 2) + len8(len(utf8)) + utf8 + b"\0"
        else:
            encoded[index] = len16(len(utf16) // 2) + utf16 + b"\0\0"
    offsets, payload = [], bytearray()
    for entry in encoded:
        offsets.append(len(payload))
        payload.extend(entry)
    payload.extend(b"\0" * (-len(payload) % 4))
    new_pool = bytearray(original[offset:offset + header])
    new_pool.extend(struct.pack(f"<{len(offsets)}I", *offsets))
    new_pool.extend(payload)
    struct.pack_into("<I", new_pool, 4, len(new_pool))
    patched = bytearray(original[:offset] + new_pool + original[offset + size:])
    struct.pack_into("<I", patched, 4, len(patched))
    _, _, new_strings, new_serialized = read_pool(patched)
    if any(new_serialized[i] != serialized[i] for i in range(len(strings)) if i not in replacements):
        raise ValueError("unrelated pool strings changed")
    if [original[o:o+n] for o,k,h,n in chunks(original) if k != 1] != [
            patched[o:o+n] for o,k,h,n in chunks(patched) if k != 1]:
        raise ValueError("XML nodes changed")
    for index, replacement in replacements.items():
        if new_strings[index] != replacement:
            raise ValueError("manifest replacement did not round trip")
    return bytes(patched), changes


def signature_entry(name: str) -> bool:
    upper = name.upper()
    return upper == "META-INF/MANIFEST.MF" or bool(re.fullmatch(r"META-INF/[^/]+\.(SF|RSA|DSA|EC)", upper))


def payload_inventory(path: Path):
    inventory = {}
    with path.open("rb") as stream, zipfile.ZipFile(path) as archive:
        for entry in archive.infolist():
            if entry.filename in inventory:
                raise ValueError("duplicate ZIP entry")
            stream.seek(entry.header_offset)
            local = stream.read(30)
            if len(local) != 30 or local[:4] != b"PK\x03\x04":
                raise ValueError("invalid ZIP local record")
            name_length, extra_length = struct.unpack_from("<HH", local, 26)
            stream.seek(name_length + extra_length, 1)
            digest, remaining = hashlib.sha256(), entry.compress_size
            while remaining:
                block = stream.read(min(1024 * 1024, remaining))
                if not block:
                    raise ValueError("truncated compressed payload")
                digest.update(block)
                remaining -= len(block)
            inventory[entry.filename] = {
                "sha256Compressed": digest.hexdigest(), "compression": entry.compress_type,
                "compressedBytes": entry.compress_size, "bytes": entry.file_size, "crc32": entry.CRC,
            }
    return inventory


def cert_digest(path: Path) -> str:
    verified = run(SDK / "apksigner", "verify", "--verbose", "--print-certs", path)
    digests = re.findall(r"^Signer #\d+ certificate SHA-256 digest: ([0-9a-fA-F]{64})$", verified, re.M)
    if len(digests) != 1:
        raise ValueError("expected one APK signing certificate")
    return digests[0].lower()


def build(expected: str) -> Path:
    build_dir = ROOT / "build"
    source = build_dir / "hyjs-local.apk"
    output_dir = build_dir / "multi-client"
    key = ROOT / ".local-signing/debug.keystore"
    for path in (build_dir, source, output_dir, key):
        if path.is_symlink():
            raise ValueError(f"symlink path refused: {path.name}")
    if not source.is_file() or not key.is_file():
        raise ValueError("controlled APK or existing debug keystore missing")
    # Own lock serializes this tool. The source-builder lock binds one immutable input
    # throughout copying and signing, without promoting or removing any root APK.
    with (build_dir / ".multi-client-build.lock").open("a") as own_lock, (build_dir / ".client-build.lock").open("a") as source_lock:
        fcntl.flock(own_lock, fcntl.LOCK_EX)
        fcntl.flock(source_lock, fcntl.LOCK_EX)
        source_sha = sha256(source)
        if source_sha != expected:
            raise ValueError(f"source SHA mismatch: {source_sha}")
        source_cert = cert_digest(source)
        before = payload_inventory(source)
        with zipfile.ZipFile(source) as archive:
            original_manifest = archive.read(MANIFEST)
        patched, changes = patch_manifest(original_manifest)
        output_dir.mkdir(exist_ok=True)
        if not output_dir.is_dir() or output_dir.is_symlink():
            raise ValueError("output directory is not a real directory")
        with tempfile.TemporaryDirectory(prefix=".multi-client-tmp-", dir=build_dir) as temporary:
            work = Path(temporary)
            unsigned, aligned, signed = (work / name for name in ("unsigned.apk", "aligned.apk", "signed.apk"))
            shutil.copyfile(source, unsigned)
            # Reuse original local ZIP records and their exact compressed streams;
            # replace the central-directory manifest entry and obsolete signatures.
            with zipfile.ZipFile(unsigned, "a") as archive:
                archive.filelist = [entry for entry in archive.filelist
                                    if entry.filename != MANIFEST and not signature_entry(entry.filename)]
                archive.NameToInfo = {entry.filename: entry for entry in archive.filelist}
                archive.writestr(MANIFEST, patched, compress_type=zipfile.ZIP_DEFLATED, compresslevel=1)
            run(SDK / "zipalign", "-f", "-p", "4", unsigned, aligned)
            run(SDK / "apksigner", "sign", "--ks", key, "--ks-key-alias", "androiddebugkey",
                "--ks-pass", "pass:android", "--key-pass", "pass:android", "--v4-signing-enabled", "false", "--out", signed, aligned)
            run(SDK / "zipalign", "-c", "-p", "4", signed)
            target_cert = cert_digest(signed)
            if target_cert != source_cert:
                raise ValueError("input and output APK signing certificates differ")
            after = payload_inventory(signed)
            payload_before = {n:v for n,v in before.items() if n != MANIFEST and not signature_entry(n)}
            payload_after = {n:v for n,v in after.items() if n != MANIFEST and not signature_entry(n)}
            if payload_before != payload_after:
                raise ValueError("non-manifest ZIP payload changed")
            with zipfile.ZipFile(signed) as archive:
                if archive.read(MANIFEST) != patched:
                    raise ValueError("signed manifest changed")
            badging = run(SDK / "aapt2", "dump", "badging", signed)
            if f"package: name='{TARGET_PACKAGE}'" not in badging or f"application-label:'{TARGET_LABEL}'" not in badging:
                raise ValueError("SDK did not confirm the expected package and label")
            if sha256(source) != source_sha:
                raise ValueError("source APK changed during build")
            target = output_dir / "hyjs-multiclient.apk"
            proof_path = output_dir / "proof.json"
            if target.is_symlink() or proof_path.is_symlink():
                raise ValueError("output symlink refused")
            canonical = json.dumps(payload_before, sort_keys=True, separators=(",", ":")).encode()
            important = {name: value for name, value in payload_before.items()
                         if name.endswith(("/Assembly-CSharp.dll", "/LocalVoicePCM.dll")) or "NativeBatch" in name}
            proof = {
                "source": str(source.relative_to(ROOT)), "sourceSHA256": source_sha,
                "output": str(target.relative_to(ROOT)), "outputSHA256": sha256(signed),
                "sourceBytes": source.stat().st_size, "outputBytes": signed.stat().st_size,
                "package": TARGET_PACKAGE, "label": TARGET_LABEL, "manifestChanges": changes,
                "manifestBeforeSHA256": hashlib.sha256(original_manifest).hexdigest(),
                "manifestAfterSHA256": hashlib.sha256(patched).hexdigest(),
                "unchangedPayloadEntries": len(payload_before), "compressedPayloadInventorySHA256": hashlib.sha256(canonical).hexdigest(),
                "importantUnchangedPayload": important, "signingCertificateSHA256": target_cert,
                "checks": ["all non-manifest/non-signature compressed payload bytes identical",
                           "all non-pool XML chunks and unrelated string entries identical",
                           "same debug certificate", "zipalign -c -p 4", "apksigner verify", "aapt2 package and label"],
                "scope": "parallel APK only; no install, launch, account or database operations",
            }
            temporary_proof = work / "proof.json"
            temporary_proof.write_text(json.dumps(proof, ensure_ascii=False, indent=2) + "\n")
            signed.replace(target)
            temporary_proof.replace(proof_path)
    return target


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--expected-source-sha256", required=True,
                        help="SHA-256 of the currently reviewed build/hyjs-local.apk")
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{64}", args.expected_source_sha256):
        parser.error("expected source SHA must be 64 lowercase hexadecimal characters")
    print(build(args.expected_source_sha256))


if __name__ == "__main__":
    main()
