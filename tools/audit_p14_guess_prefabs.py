"""Read exact original Unity serialized guess-state fields; never modify APKs."""
import argparse
import hashlib
import json
from pathlib import Path
import struct
import zipfile

import UnityPy

ROOT = Path(__file__).resolve().parents[1]
ENTRIES = {
    "assets/bin/Data/08490f6ba8877b045b59c963d5733dc3": None,
    "assets/bin/Data/30a4339c32452d847b7a7944088141a1": None,
    "assets/bin/Data/f0235347c66f7014da6d2ece93df909d": None,
}


def audit(apk, output):
    apk = apk.resolve()
    output = output.resolve()
    if apk != ROOT / "apks/original/10056262.apk" or not output.is_relative_to(ROOT / "analysis"):
        raise AssertionError("original input and analysis-only output required")
    result = []
    with zipfile.ZipFile(apk) as archive:
        scripts = UnityPy.load(archive.read("assets/bin/Data/globalgamemanagers.assets"))
        script = next(o for o in scripts.objects if o.path_id == 723)
        meta = script.read_typetree()
        if meta["m_ClassName"] != "ServerBattlePlayerUIItem" or meta["m_Namespace"] != "UI":
            raise AssertionError("original script identity changed")
        for entry in ENTRIES:
            encoded = archive.read(entry)
            environment = UnityPy.load(encoded)
            rows = []
            objects = {o.path_id: o for o in environment.objects}
            for obj in environment.objects:
                if obj.type.name != "MonoBehaviour":
                    continue
                header = obj.read_typetree(check_read=False)
                if header.get("m_Script") != {"m_FileID": 1, "m_PathID": 723}:
                    continue
                raw = obj.get_raw_data()
                # The original class has13 public Unity references (12B each),
                # followed by two empty serialized strings and its final2ints.
                # Check the whole204B layout before interpreting any state value.
                if len(raw) != 204 or raw[188:196] != b"\0" * 8 or header["m_Name"] != "":
                    raise AssertionError("unsupported original component layout")
                state, shown = struct.unpack_from("<ii", raw, 196)
                if state not in (0, 7, 9, 11, 13, 15, 17) or shown not in (0, 1):
                    raise AssertionError("unexpected original guess enum")
                owner = objects[header["m_GameObject"]["m_PathID"]].read_typetree()
                rows.append({"componentPathID": obj.path_id, "byteSize": len(raw), "gameObject": owner["m_Name"], "inServerBattleStateOffset": 196, "inServerBattleState": state, "showGuessOffset": 200, "showGuess": shown, "rawSHA256": hashlib.sha256(raw).hexdigest()})
            result.append({"entry": entry, "entrySHA256": hashlib.sha256(encoded).hexdigest(), "rows": rows})
    with apk.open("rb") as source:
        digest = hashlib.file_digest(source, "sha256").hexdigest()
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps({"originalAPK_SHA256": digest, "script": {"pathID": 723, "class": "UI.ServerBattlePlayerUIItem"}, "layoutProof": "original CIL class public field order:13refs+2strings+enum+int; exact204B/rawzero string lengths checked", "entries": result, "scope": "serialized source values only; no server settlement or APK mutation"}, ensure_ascii=False, indent=2) + "\n")
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apk", type=Path, default=ROOT / "apks/original/10056262.apk")
    parser.add_argument("--output", type=Path, default=ROOT / "analysis/p14/guess-source-pending-20261015/original-ordinary-state-proof.json")
    args = parser.parse_args()
    values = audit(args.apk, args.output)
    print(json.dumps({item["entry"]: sorted({row["inServerBattleState"] for row in item["rows"]}) for item in values}, indent=2))
