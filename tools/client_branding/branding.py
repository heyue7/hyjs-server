"""Produce a manifest/icon overlay while retaining every existing manifest contract.

The original resources.arsc already declares drawable/supersdk_icon at
0x7f0200bc. Only its PNG and the application/launcher labels/icon change.
This tool does not sign, install or promote an APK.
"""
import argparse
import hashlib
import json
from pathlib import Path
import struct
import zipfile

NAME = "刀剑神域黑衣剑士·灵魂之花"
ICON_ENTRY = "res/drawable/supersdk_icon.png"
ICON_ID = 0x7F0200BC


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def length(raw, at, utf8):
    if utf8:
        n = raw[at]
        return ((n & 0x7f) << 8 | raw[at + 1], at + 2) if n & 0x80 else (n, at + 1)
    n = struct.unpack_from("<H", raw, at)[0]
    return (((n & 0x7fff) << 16) | struct.unpack_from("<H", raw, at + 2)[0], at + 4) if n & 0x8000 else (n, at + 2)


def encode_length(n, utf8):
    if utf8:
        if n >= 32768:
            raise ValueError("Manifest UTF8 string too long")
        return bytes([0x80 | (n >> 8), n & 255]) if n >= 128 else bytes([n])
    return struct.pack("<H", n) if n < 32768 else struct.pack("<HH", 0x8000 | (n >> 16), n & 65535)


def chunks(raw):
    if struct.unpack_from("<HHI", raw) != (3, 8, len(raw)):
        raise ValueError("Expected complete binary Android manifest")
    result = []
    at = 8
    while at < len(raw):
        kind, header, size = struct.unpack_from("<HHI", raw, at)
        if header < 8 or size < header or at + size > len(raw):
            raise ValueError("Invalid manifest chunk")
        result.append((kind, header, raw[at:at + size]))
        at += size
    if at != len(raw):
        raise ValueError("Truncated manifest")
    return result


def strings_from_pool(chunk):
    kind, header, size = struct.unpack_from("<HHI", chunk)
    count, styles, flags, start, style_start = struct.unpack_from("<5I", chunk, 8)
    if kind != 1 or header != 28 or styles or style_start:
        raise ValueError("Only the current unstyled manifest string pool is supported")
    values = []
    utf8 = bool(flags & 0x100)
    for i in range(count):
        pos = start + struct.unpack_from("<I", chunk, header + 4 * i)[0]
        units, pos = length(chunk, pos, utf8)
        n, pos = length(chunk, pos, True) if utf8 else (units * 2, pos)
        value = chunk[pos:pos + n].decode("utf-8" if utf8 else "utf-16le")
        if len(value.encode("utf-16le")) // 2 != units:
            raise ValueError("Manifest string units differ")
        values.append(value)
    return values, flags


def make_pool(values, flags):
    utf8 = bool(flags & 0x100)
    data = bytearray()
    offsets = []
    for value in values:
        offsets.append(len(data))
        encoded = value.encode("utf-8" if utf8 else "utf-16le")
        data += encode_length(len(value.encode("utf-16le")) // 2, utf8)
        if utf8:
            data += encode_length(len(encoded), True)
        data += encoded + (b"\0" if utf8 else b"\0\0")
    data += b"\0" * (-len(data) % 4)
    start = 28 + 4 * len(values)
    # The caller preserves indices; the original sorted flag is no longer valid.
    return struct.pack("<HHI5I", 1, 28, start + len(data), len(values), 0,
                       flags & ~1, start, 0) + struct.pack("<" + "I" * len(values), *offsets) + data


def elements(parts, values):
    result = []
    for kind, header, chunk in parts:
        if kind != 0x102:
            continue
        if header != 16:
            raise ValueError("Unexpected manifest XML node")
        name = values[struct.unpack_from("<I", chunk, 20)[0]]
        start, width, count = struct.unpack_from("<3H", chunk, 24)
        if start != 20 or width != 20:
            raise ValueError("Unexpected manifest attribute representation")
        attrs = []
        for i in range(count):
            at = 16 + start + width * i
            ns, key, raw = struct.unpack_from("<3I", chunk, at)
            size, zero, kind_value, value = struct.unpack_from("<HBBI", chunk, at + 12)
            if size != 8 or zero:
                raise ValueError("Malformed manifest value")
            attrs.append((None if ns == 0xffffffff else values[ns], values[key], kind_value,
                          values[value] if kind_value == 3 else value))
        result.append((name, attrs))
    return result


def patch_manifest(raw, icon_resource_id=ICON_ID):
    parts = chunks(raw)
    pool_positions = [i for i, p in enumerate(parts) if p[0] == 1]
    maps = [i for i, p in enumerate(parts) if p[0] == 0x180]
    if len(pool_positions) != 1 or len(maps) != 1:
        raise ValueError("Expected one string pool and one resource map")
    pool_at = pool_positions[0]
    values, flags = strings_from_pool(parts[pool_at][2])
    old_values = list(values)
    old_elements = elements(parts, values)
    namespace = values.index("http://schemas.android.com/apk/res/android")
    if NAME in values:
        raise ValueError("Branding is already applied")
    new_label = len(values)
    values.append(NAME)
    if "icon" not in values:
        values.append("icon")
    icon_key = values.index("icon")
    resource_map = list(struct.unpack("<" + "I" * ((len(parts[maps[0]][2]) - 8) // 4), parts[maps[0]][2][8:]))
    resource_map += [0] * max(0, icon_key + 1 - len(resource_map))
    if resource_map[icon_key] not in (0, 0x01010002):
        raise ValueError("Existing icon attribute resource identity differs")
    resource_map[icon_key] = 0x01010002
    changed = []
    rebuilt = []
    for index, (kind, header, chunk) in enumerate(parts):
        if index == pool_at:
            chunk = make_pool(values, flags)
        elif index == maps[0]:
            chunk = struct.pack("<HHI", kind, header, 8 + 4 * len(resource_map)) + struct.pack("<" + "I" * len(resource_map), *resource_map)
        elif kind == 0x102:
            chunk = bytearray(chunk)
            name = old_values[struct.unpack_from("<I", chunk, 20)[0]]
            start, width, count = struct.unpack_from("<3H", chunk, 24)
            attrs = []
            is_launcher = False
            for i in range(count):
                pos = 16 + start + i * width
                entry = bytearray(chunk[pos:pos + width])
                ns, key, raw_value = struct.unpack_from("<3I", entry)
                if old_values[key] == "name" and entry[15] == 3:
                    is_launcher = old_values[struct.unpack_from("<I", entry, 16)[0]] == "com.hyjs.wallet.WalletActivity"
                attrs.append(entry)
            target = name == "application" or (name == "activity" and is_launcher)
            if target:
                label_count = 0
                for entry in attrs:
                    ns, key, raw_value = struct.unpack_from("<3I", entry)
                    if old_values[key] == "label":
                        if ns != namespace:
                            raise ValueError("Label namespace changed")
                        struct.pack_into("<I", entry, 8, new_label)
                        struct.pack_into("<HBBI", entry, 12, 8, 0, 3, new_label)
                        label_count += 1
                if label_count != 1:
                    raise ValueError("Expected one explicit application and launcher label")
                changed.append(name + ":label")
            if name == "application":
                if any(old_values[struct.unpack_from("<I", entry, 4)[0]] == "icon" for entry in attrs):
                    raise ValueError("Current controlled application unexpectedly has an icon")
                entry = struct.pack("<IIIHBBI", namespace, icon_key, 0xffffffff, 8, 0, 1, icon_resource_id)
                # Android's typed-attribute parser requires resource-ID order.
                # Appending icon(0x01010002) after debuggable/allowBackup lets
                # generic XML readers decode it while Android ignores it.
                attribute_ids = [resource_map[struct.unpack_from("<I", attribute, 4)[0]] for attribute in attrs]
                if attribute_ids != sorted(attribute_ids):
                    raise ValueError("Inherited application attributes are not resource-ID sorted")
                insert_at = next((i for i, ident in enumerate(attribute_ids) if ident > 0x01010002), len(attrs))
                attrs.insert(insert_at, bytearray(entry))
                # Preserve the optional one-based id/class/style indices when
                # inserting before them. The present controlled app uses zero.
                for position in [30, 32, 34]:
                    selected = struct.unpack_from("<H", chunk, position)[0]
                    if selected and selected > insert_at:
                        struct.pack_into("<H", chunk, position, selected + 1)
                changed.append("application:icon")
                struct.pack_into("<H", chunk, 28, len(attrs))
            chunk = chunk[:16 + start] + b"".join(attrs)
            struct.pack_into("<I", chunk, 4, len(chunk))
            chunk = bytes(chunk)
        rebuilt.append(chunk)
    if sorted(changed) != ["activity:label", "application:icon", "application:label"]:
        raise ValueError("Expected exactly application icon and two labels")
    result = struct.pack("<HHI", 3, 8, 8 + sum(map(len, rebuilt))) + b"".join(rebuilt)
    # Compare every decoded element and attribute, including package, launcher,
    # permissions, SDK, activities and network/IME contracts.
    actual = elements(chunks(result), values)
    expected = []
    for name, attrs in old_elements:
        attrs = list(attrs)
        target = name == "application" or (name == "activity" and any(key == "name" and val == "com.hyjs.wallet.WalletActivity" for _, key, _, val in attrs))
        if target:
            attrs = [(ns, key, 3, NAME) if key == "label" else (ns, key, k, v) for ns, key, k, v in attrs]
        if name == "application":
            insert_at = next((i for i, (_, key, _, _) in enumerate(attrs) if resource_map[values.index(key)] > 0x01010002), len(attrs))
            attrs.insert(insert_at, ("http://schemas.android.com/apk/res/android", "icon", 1, icon_resource_id))
        expected.append((name, attrs))
    if actual != expected:
        raise ValueError("Non-branding manifest contract changed")
    return result, {"changedAttributes": changed, "allOtherDecodedElementAttributesExact": True,
                    "originalStringIndicesPreserved": True, "resourcesArscUnchanged": True,
                    "iconResourceID": hex(icon_resource_id), "iconEntry": ICON_ENTRY, "applicationName": NAME,
                    "applicationAttributeResourceIDOrderVerified": True}


def resolve_icon_resource(path):
    """Resolve the drawable ID from this exact APK's actual ARSC table."""
    from loguru import logger
    from androguard.core.apk import APK
    logger.remove()
    package = APK(str(path))
    resources = package.get_android_resources()
    if resources is None:
        raise ValueError("Current APK resource table is absent")
    found = []
    for name in resources.get_packages_names():
        ident = resources.get_res_id_by_key(name, "drawable", "supersdk_icon")
        if ident is None:
            continue
        resolved = resources.get_resolved_res_configs(ident)
        if not resolved or any(value != ICON_ENTRY for _, value in resolved):
            raise ValueError("Current APK drawable resolves to a different file")
        found.append((name, ident))
    if len(found) != 1:
        raise ValueError("Current APK must resolve one supersdk_icon drawable")
    with zipfile.ZipFile(path) as archive:
        resource_sha = digest(archive.read("resources.arsc"))
    name, ident = found[0]
    return ident, {"currentARSCResourcePackage": name, "currentARSCResourceSHA256": resource_sha,
                   "resolvedDrawableKey": "drawable/supersdk_icon", "resolvedResourceID": hex(ident),
                   "resolvedEntry": ICON_ENTRY}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--base-sha256", required=True)
    parser.add_argument("--icon", type=Path, required=True)
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    if hashlib.file_digest(args.base_apk.open("rb"), "sha256").hexdigest() != args.base_sha256:
        raise ValueError("Input APK SHA differs")
    out = args.output_directory.resolve()
    if out.exists():
        raise ValueError("Use a fresh output directory")
    icon = args.icon.read_bytes()
    if not icon.startswith(b"\x89PNG\r\n\x1a\n"):
        raise ValueError("Launcher icon must be a PNG")
    with zipfile.ZipFile(args.base_apk) as apk:
        if ICON_ENTRY not in apk.namelist():
            raise ValueError("Original drawable resource is absent")
        icon_id, resolution = resolve_icon_resource(args.base_apk)
        manifest, proof = patch_manifest(apk.read("AndroidManifest.xml"), icon_id)
        proof.update(resolution)
    out.mkdir(parents=True)
    (out / "AndroidManifest.xml").write_bytes(manifest)
    (out / "icon.png").write_bytes(icon)
    proof.update(schema="hyjs-client-branding-overlay-v1", status="STATIC_OVERLAY_NOT_PACKAGED",
                 baseAPK=str(args.base_apk.resolve()), baseAPKSHA256=args.base_sha256,
                 replacements={"AndroidManifest.xml": {"file": "AndroidManifest.xml", "sha256": digest(manifest)},
                               ICON_ENTRY: {"file": "icon.png", "sha256": digest(icon)}},
                 sourceSHA256=digest(Path(__file__).read_bytes()))
    (out / "manifest.json").write_text(json.dumps(proof, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": proof["status"], "changedAttributes": proof["changedAttributes"]}, ensure_ascii=False))


if __name__ == "__main__":
    main()
