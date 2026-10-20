"""Patch two pinned opening-activity UILabel Font PPtrs, without reserialization."""
import argparse
import hashlib
import json
from pathlib import Path
import struct
import zipfile

import UnityPy

ROOT = Path(__file__).resolve().parents[2]
APK_SHA = "9fec499af7473f5fd05da31670e27b3e9eb270ba9b0c9d52bd6f3238e1425285"
PREFIX = "assets/bin/Data/"
FIELD_OFFSET = 168
OLD = struct.pack("<iq", 2, 190)
NEW = struct.pack("<iq", 2, 191)
PINS = [
    dict(entry=PREFIX + "ebce6dd628f2446409bbc8bf961f59be", entrySHA="0fa072b3d90d4caa99ae3958ae47acfe0fb3f109832cc27ee867d7f5ca3981f2", pathId=22, start=5936, size=356,
         objectSHA="d0c1ff394fb6fafcf75b84d3b5cc2b3aabab62e82109dfc741ce6ba0c3ccf426", go="dayForLabel", owner=23, ownerFieldOffset=92),
    dict(entry=PREFIX + "604bece618febcd4db139a9b09dfa0dd", entrySHA="629e02dcf16a9d70e9edb4cdeb23c7026235f03442820a8f4aedf178f5fc9c1f", pathId=126, start=17976, size=344,
         objectSHA="1ee1cd6c8c9d0008fd8c46026e02d175f7ca62257699284693fb480f19bb03ac", go="Label", owner=102, ownerFieldOffset=92),
]


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def require(condition, message):
    if not condition:
        raise ValueError(message)


def objects(raw):
    return {obj.path_id: obj for obj in UnityPy.load(raw).objects}


def font_char(raw, code):
    """Read Unicode BMP cmap format4; return actual nonzero glyph coverage."""
    count = struct.unpack_from(">H", raw, 4)[0]
    cmap = next(struct.unpack_from(">I", raw, 12 + i * 16 + 8)[0]
                for i in range(count) if raw[12 + i * 16:16 + i * 16] == b"cmap")
    for i in range(struct.unpack_from(">H", raw, cmap + 2)[0]):
        sub = cmap + struct.unpack_from(">I", raw, cmap + 8 + i * 8)[0]
        if struct.unpack_from(">H", raw, sub)[0] != 4:
            continue
        n = struct.unpack_from(">H", raw, sub + 6)[0] // 2
        end = sub + 14
        start = end + 2 * n + 2
        delta = start + 2 * n
        ranges = delta + 2 * n
        for j in range(n):
            a = struct.unpack_from(">H", raw, start + 2 * j)[0]
            b = struct.unpack_from(">H", raw, end + 2 * j)[0]
            if a <= code <= b:
                shift = struct.unpack_from(">h", raw, delta + 2 * j)[0]
                distance = struct.unpack_from(">H", raw, ranges + 2 * j)[0]
                if distance == 0:
                    glyph = (code + shift) & 65535
                else:
                    glyph = struct.unpack_from(">H", raw, ranges + 2 * j + distance + 2 * (code - a))[0]
                    if glyph:
                        glyph = (glyph + shift) & 65535
                if glyph:
                    return True
    return False


def font_evidence(archive):
    data = b"".join(archive.read(PREFIX + "sharedassets0.assets.split" + str(i)) for i in range(11))
    refs = objects(data)
    result = []
    for path_id, name in [(190, "Numbers"), (191, "Words")]:
        obj = refs[path_id]
        require(obj.type.name == "Font", "Font reference target type differs")
        tree = obj.read_typetree()
        require(tree["m_Name"] == name, "Font name differs")
        font = bytes(tree["m_FontData"])
        coverage = {char: font_char(font, ord(char)) for char in "第一二三四五六七天领取充值0123456789"}
        require(all(coverage[char] == (path_id == 191) for char in "第一二三四五六七天领取充值"), "Chinese glyph coverage differs")
        require(all(coverage[char] for char in "0123456789"), "Numeric glyph coverage differs")
        result.append(dict(pathId=path_id, name=name, fontDataSHA256=sha(font), coverage=coverage))
    return result


def verify_and_patch(raw, pin):
    require(sha(raw) == pin["entrySHA"], "Resource entry SHA differs: " + pin["entry"])
    before = objects(raw)
    target = before[pin["pathId"]]
    original = target.get_raw_data()
    require(target.type.name == "MonoBehaviour", "Target object type differs")
    require(target.byte_start == pin["start"] and target.byte_size == pin["size"], "Object location/size differs")
    require(sha(original) == pin["objectSHA"], "Target object SHA differs")
    require(struct.unpack_from("<iq", original, 16) == (1, 2055), "Original UILabel script PPtr differs")
    require(original[FIELD_OFFSET:FIELD_OFFSET + 12] == OLD, "Original Font PPtr differs")
    go_id = struct.unpack_from("<q", original, 4)[0]
    require(before[go_id].read_typetree()["m_Name"] == pin["go"], "Target GameObject name differs")
    owner_raw = before[pin["owner"]].get_raw_data()
    require(struct.unpack_from("<iq", owner_raw, pin["ownerFieldOffset"]) == (0, pin["pathId"]), "Owner label field binding differs")
    external = target.assets_file.externals[1]
    require(external.path == "sharedassets0.assets", "FileID2 Font external mapping differs")
    at = target.byte_start + FIELD_OFFSET
    patched = raw[:at] + NEW + raw[at + 12:]
    require(len(raw) == len(patched), "Resource byte length changed")
    changed = [index for index, pair in enumerate(zip(raw, patched)) if pair[0] != pair[1]]
    require(changed == [at + 4], "Bytes outside pinned Font pathId changed")
    after = objects(patched)
    require(before.keys() == after.keys(), "Reloaded object inventory differs")
    unchanged = 0
    for path_id, old in before.items():
        new = after[path_id]
        require((old.type.name, old.byte_start, old.byte_size) == (new.type.name, new.byte_start, new.byte_size), "Object metadata changed")
        if path_id == pin["pathId"]:
            newraw = new.get_raw_data()
            require(newraw == original[:FIELD_OFFSET] + NEW + original[FIELD_OFFSET + 12:], "Target contains an extra field change")
        else:
            require(new.get_raw_data() == old.get_raw_data(), "Unrelated object changed")
            unchanged += 1
    require(after[pin["pathId"]].assets_file.externals[1].path == "sharedassets0.assets", "Reloaded Font reference mapping differs")
    return patched, dict(entry=pin["entry"], beforeSHA256=sha(raw), afterSHA256=sha(patched),
                         pathId=pin["pathId"], gameObject=pin["go"], objectByteStart=at - FIELD_OFFSET,
                         fieldOffset=FIELD_OFFSET, oldFont=[2, 190], newFont=[2, 191], changedByteOffsets=changed,
                         resourceLength=len(raw), unchangedOtherObjects=unchanged, reloadVerified=True,
                         nonTargetBytesIdentical=True, originalObjectMetadataPreserved=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, default=ROOT / "build/hyjs-local.apk")
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    require(sha(args.base_apk.read_bytes()) == APK_SHA, "Pinned APK identity differs")
    require(not args.output_directory.exists(), "Fresh output directory required")
    # Finish all validation before writing either resource.
    results = []
    with zipfile.ZipFile(args.base_apk) as archive:
        fonts = font_evidence(archive)
        for pin in PINS:
            results.append(verify_and_patch(archive.read(pin["entry"]), pin))
        original_entries = {item.filename: sha(archive.read(item.filename)) for item in archive.infolist() if not item.is_dir() and item.filename not in {pin["entry"] for pin in PINS}}
    out = args.output_directory.resolve()
    out.mkdir(parents=True)
    changes = []
    for patched, report in results:
        destination = out / report["entry"]
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(patched)
        changes.append(report)
    report = dict(schema="hyjs-opening-label-fonts-v1", status="STATIC_VERIFIED_NOT_PACKAGED", inputAPK_SHA256=APK_SHA,
                  sources={str(Path(__file__).relative_to(ROOT)): sha(Path(__file__).read_bytes())},
                  replacementEntries=[pin["entry"] for pin in PINS], changes=changes, fontEvidence=fonts,
                  otherAPKEntriesToPreserve=original_entries, modifiedDLLs=[], modifiedFontAssets=[],
                  originalFormatPreserved=True, deviceExecuted=False, visualVerified=False)
    (out / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(dict(status=report["status"], changes=changes), ensure_ascii=False))


if __name__ == "__main__":
    main()
