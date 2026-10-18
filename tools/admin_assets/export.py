"""Export original icons and bounded NGUI activity art for the local admin.

Resolve Resources through globalgamemanagers and inspect only selected prefab
dependencies. Never scan all Android assets, render invented pages, or modify APKs.
"""
import argparse
import collections
import csv
import hashlib
import io
import json
from pathlib import Path
import re
import struct
import zipfile

from PIL import Image, ImageDraw
import UnityPy

ROOT = Path(__file__).resolve().parents[2]
ORIGINAL_SHA = "b1d99496c7ffe17c45ab7646c72023be406924997d471e3f18db169fb0dfd641"
BASE_URL = "/admin/media/"
ICON_DIRS = {"itemicons", "equipicons", "treasureicons", "heroheader", "fishicon", "mineicon", "pet"}
SEED_PREFABS = {"prefab/commonicon", "prefab/equipicon", "prefab/fishicon", "prefab/heroicon", "prefab/itemicon",
                "prefab/mineicon", "prefab/mineiconbig", "prefab/petequipicon", "prefab/peticon",
                "pages/uiactivity", "pages/recyclepage", "pages/unionpage"}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def safe(name):
    return re.sub(r"[^A-Za-z0-9_.-]", "_", name)


def read_string(raw, at):
    size = struct.unpack_from("<i", raw, at)[0]
    if size < 0 or at + 4 + size > len(raw):
        raise ValueError("Invalid serialized UTF8 string")
    value = raw[at + 4:at + 4 + size].decode("utf-8")
    return value, (at + 4 + size + 3) & ~3


class Exporter:
    def __init__(self, apk, output):
        self.apk = apk
        self.output = output
        self.names = set(apk.namelist())
        self.cache = {}
        self.entry_reads = set()
        self.manifest = {"schema": "hyjs-admin-original-media-v1", "sourceAPK": {"file": "10056262.apk", "sha256": ORIGINAL_SHA},
                         "sprites": {}, "activityPrefabs": {}, "activityModes": {}, "displayNames": {}, "resourceIcons": {},
                         "atlases": [], "missing": [], "duplicates": [], "boundaries": [
                             "Original shipped pixels and serialized prefab metadata only; no reconstructed full UI screenshot",
                             "NGUI UISpriteData has no packed-rotation field in this original build; x/y are top-left pixel coordinates",
                             "UnityPy Texture2D.image performs the Unity bottom-up decoding flip; atlas crop uses original top-left x/y directly",
                             "Original reward quantities, probabilities and activity eligibility are not inferred from art"]}
        self.resources = {}
        env = self.load("assets/bin/Data/globalgamemanagers")
        manager = next(o for o in env.objects if o.type.name == "ResourceManager")
        for name, ref in manager.read_typetree()["m_Container"]:
            self.resources[name] = (self.ref_entry(manager, ref), ref["m_PathID"])
        script_env = self.load("assets/bin/Data/globalgamemanagers.assets")
        self.scripts = {o.path_id: o.read_typetree() for o in script_env.objects if o.type.name == "MonoScript"}

    def load(self, entry):
        if entry not in self.cache:
            if entry not in self.names:
                raise KeyError("Original APK resource absent: " + entry)
            self.entry_reads.add(entry)
            self.cache[entry] = UnityPy.load(self.apk.read(entry))
        return self.cache[entry]

    def ref_entry(self, obj, ref):
        if not ref["m_FileID"]:
            return next(k for k, e in self.cache.items() if obj.assets_file in e.files.values())
        path = obj.assets_file.externals[ref["m_FileID"] - 1].path
        if path.startswith("archive:/"):
            path = path.rsplit("/", 1)[-1]
        return "assets/bin/Data/" + path

    def dereference(self, obj, ref):
        if not ref["m_FileID"]:
            return obj.assets_file.objects[ref["m_PathID"]]
        return next(o for o in self.load(self.ref_entry(obj, ref)).objects if o.path_id == ref["m_PathID"])

    def source(self, entry, obj, kind, **extra):
        return {"apk": "10056262.apk", "apkSHA256": ORIGINAL_SHA, "entry": entry,
                "entrySHA256": sha(self.apk.read(entry)), "objectPathID": obj.path_id,
                "objectType": obj.type.name, "extraction": kind, **extra}

    def image(self, name, image, source, activity=False):
        if not name or image.width < 1 or image.height < 1:
            raise ValueError("Empty original sprite")
        filename = safe(name) + ".png"
        relative = ("activity/" if activity else "items/") + filename
        target = self.output / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        buffer = io.BytesIO()
        image.convert("RGBA").save(buffer, format="PNG", compress_level=5)
        encoded = buffer.getvalue()
        item = {"iconUrl": BASE_URL + relative, "previewUrl": BASE_URL + relative,
                "source": source, "sha256": sha(encoded), "sha": sha(encoded),
                "width": image.width, "height": image.height}
        old = self.manifest["sprites"].get(name)
        if old:
            self.manifest["duplicates"].append({"spriteName": name, "samePixels": old["sha256"] == item["sha256"], "kept": old["source"], "other": source})
            return
        if target.exists() and target.read_bytes() != encoded:
            raise ValueError("Sprite filename collision: " + name)
        target.write_bytes(encoded)
        self.manifest["sprites"][name] = item

    def independent_icons(self):
        wanted = {value.lower() for value in self.manifest["resourceIcons"].values() if value}
        entries = sorted(n for n in self.names if n.startswith("assets/android/") and len(n.split("/")) == 4 and n.split("/")[2] in ICON_DIRS
                         and (n.split("/")[2] != "equipicons" or n.rsplit("/", 1)[-1] in wanted or
                              n.rsplit("/", 1)[-1].startswith(("equip_icon_", "petequip_icon_", "jz01_jingyan_"))))
        for entry in entries:
            env = self.load(entry)
            textures = [o for o in env.objects if o.type.name == "Texture2D"]
            if len(textures) != 1:
                self.manifest["missing"].append({"entry": entry, "reason": "Expected one original icon Texture2D"})
                continue
            obj = textures[0]
            texture = obj.read()
            self.image(texture.m_Name, texture.image, self.source(entry, obj, "original standalone Texture2D"))
            # Icons are independent small files; retain no decoded Unity cache.
            self.cache.pop(entry)
        self.manifest["independentIconBundleCount"] = len(entries)

    def atlas(self, entry):
        env = self.load(entry)
        objects = {o.path_id: o for o in env.objects}
        for obj in env.objects:
            if obj.type.name != "MonoBehaviour":
                continue
            header = obj.read_typetree(check_read=False)
            if self.scripts.get(header.get("m_Script", {}).get("m_PathID"), {}).get("m_ClassName") != "UIAtlas":
                continue
            raw = obj.get_raw_data()
            _, cursor = read_string(raw, 28)
            material = {"m_FileID": struct.unpack_from("<i", raw, cursor)[0], "m_PathID": struct.unpack_from("<q", raw, cursor + 4)[0]}
            cursor += 12
            count = struct.unpack_from("<i", raw, cursor)[0]
            cursor += 4
            if count < 1 or count > 10000:
                raise ValueError("Unsupported original UIAtlas sprite count")
            sprites = []
            for _ in range(count):
                name, cursor = read_string(raw, cursor)
                values = struct.unpack_from("<12i", raw, cursor)
                cursor += 48
                sprites.append((name, values))
            # Original UIAtlas serial fields end with pixelSize, replacement,
            # coordinate enum and the unused old Sprite list. No rotation bit.
            if len(raw) - cursor != 24:
                raise ValueError("Original UIAtlas layout differs; no heuristic crop")
            pixel_size, replacement_file, replacement_path, coordinates, legacy_count = struct.unpack_from("<fiqii", raw, cursor)
            if replacement_file or replacement_path or coordinates != 0 or legacy_count:
                raise ValueError("Unsupported atlas replacement/legacy normalized coordinates")
            material_obj = self.dereference(obj, material)
            material_data = material_obj.read_typetree()
            main_tex = next(value["m_Texture"] for key, value in material_data["m_SavedProperties"]["m_TexEnvs"] if key["name"] == "_MainTex")
            texture_obj = self.dereference(material_obj, main_tex)
            texture = texture_obj.read()
            image = texture.image
            atlas_name = material_data["m_Name"]
            atlas_source = self.source(entry, obj, "original NGUI UIAtlas serialized mSprites", atlasName=atlas_name,
                                       materialPathID=material_obj.path_id, texturePathID=texture_obj.path_id,
                                       textureName=texture.m_Name, textureSize=[image.width, image.height],
                                       pixelSize=pixel_size, coordinateOrigin="top-left", packedRotation="not present in original UISpriteData")
            names = []
            for name, values in sprites:
                x, y, width, height = values[:4]
                if min(x, y) < 0 or min(width, height) <= 0 or x + width > image.width or y + height > image.height:
                    raise ValueError("Original atlas rectangle out of bounds: " + name)
                # Export actual item/pet/currency icons and the selected activity
                # atlases, without exporting every unrelated UI decoration.
                item = name in self.manifest["resourceIcons"].values() or bool(re.search(r"(^|_)(item|equip|trea|hero|pet|fish|mine|coin|diamond|gold|energy|power|light)(_|[0-9]|$)", name, re.I))
                activity = any(k in atlas_name.lower() for k in ["activity", "recycle", "festival", "turntable", "redweapon", "openserver", "christmas"])
                if not item and not activity:
                    continue
                source = dict(atlas_source, spriteRect={"x": x, "y": y, "width": width, "height": height},
                              border=dict(zip(["left", "right", "top", "bottom"], values[4:8])),
                              padding=dict(zip(["left", "right", "top", "bottom"], values[8:12])))
                self.image(name, image.crop((x, y, x + width, y + height)), source, activity=activity and not item)
                names.append(name)
            self.manifest["atlases"].append({"name": atlas_name, "source": atlas_source,
                                             "serializedSprites": count, "exportedSprites": names})

    def prefab(self, resource_name, entry, root_id):
        env = self.load(entry)
        objects = {o.path_id: o for o in env.objects}
        rows = []
        sprite_names = []
        for obj in env.objects:
            if obj.type.name != "MonoBehaviour":
                continue
            header = obj.read_typetree(check_read=False)
            script = self.scripts.get(header.get("m_Script", {}).get("m_PathID"), {})
            raw = obj.get_raw_data()
            owner = objects.get(header.get("m_GameObject", {}).get("m_PathID"))
            owner_name = owner.read_typetree()["m_Name"] if owner else ""
            if script.get("m_ClassName") == "UILabel":
                # The original inherited UIWidget occupies 164 bytes; UILabel
                # serializes keepCrisp(4), font pointers(24), then mText at192.
                # Short/unexpected layouts remain explicit instead of scanning.
                if len(raw) >= 196:
                    text, end = read_string(raw, 192)
                    if text and all(c.isprintable() or c in "\r\n\t" for c in text):
                        rows.append({"text": text, "gameObject": owner_name,
                                     "source": self.source(entry, obj, "original UILabel.mText", field="mText", byteOffset=192,
                                                           monoScriptPathID=header["m_Script"]["m_PathID"], monoScriptClass="UILabel")})
            elif script.get("m_ClassName") == "UISprite" and len(raw) >= 220:
                name, _ = read_string(raw, 216)
                if name:
                    sprite_names.append(name)
        if resource_name.startswith("activityinnerpage/") or resource_name in ["pages/uiactivity", "pages/recyclepage", "pages/unionpage"]:
            self.manifest["activityPrefabs"][resource_name] = {
                "resourceName": resource_name,
                "source": {"apk": "10056262.apk", "entry": entry, "entrySHA256": sha(self.apk.read(entry)), "rootGameObjectPathID": root_id},
                "spriteNames": sorted(set(sprite_names)), "labelTexts": rows, "previewUrls": [],
                "previewKind": "original referenced art components; not a reconstructed page screenshot"}

    def tables(self):
        for table, key, icon in [("Item", "itemId", "Icon"), ("Equip", "id", "iconName"), ("Treasure", "id", "iconName"),
                                 ("Hero", "id", "iconName"), ("Pet", "id", "iconName"), ("PetEquip", "id", "icon"),
                                 ("MineSkill", "MineItemID", None), ("FishSkill", "FishItemID", None),
                                 ("NPC", "npcId", None), ("Text", "id", None)]:
            entry = "assets/android/tabledata/table" + table.lower()
            env = self.load(entry)
            obj = next(o for o in env.objects if o.type.name == "TextAsset")
            text = obj.read().m_Script
            lines = list(csv.reader(io.StringIO(text), delimiter="\t"))
            rows = [dict(zip(lines[1], v)) for v in lines[5:] if v and v[0]]
            if table == "Text":
                for row in rows:
                    self.manifest["displayNames"][row["id"]] = {"text": row["text"], "source": {"apk": "10056262.apk", "entry": entry,
                                                                             "entrySHA256": sha(self.apk.read(entry)), "rowID": row["id"], "field": "text"}}
            elif table in ["MineSkill", "FishSkill"]:
                for row in rows:
                    if row["Type"] in ["mine", "dock"]:
                        self.manifest.setdefault("nonInventoryResources", {})[row[key]] = {"type": row["Type"],
                            "reason": "original gathering node, not a backpack item icon", "source": {"entry": entry, "rowID": row[key], "field": "Type"}}
                        continue
                    self.manifest["resourceIcons"][row[key]] = row[key]
            elif icon:
                # Use actual machine headers rather than assuming all tables
                # share the same id/icon spelling.
                fields = lines[1]
                key_field = next((f for f in [key, "id", "itemId", "ItemId", "MineItemId", "FishItemId", "PetId", "EquipId", "TreasureId"] if f in fields), None)
                icon_field = next((f for f in [icon, "Icon", "icon", "IconName", "iconName"] if f in fields), None)
                if not key_field or not icon_field:
                    self.manifest["missing"].append({"table": table, "reason": "Unmapped table id/icon header", "fields": fields})
                    continue
                for row in rows:
                    self.manifest["resourceIcons"][row[key_field]] = row[icon_field]
            else:
                self.manifest["npcRows"] = [{k: v for k, v in row.items() if any(x in k.lower() for x in ["id", "name", "hero", "model", "page", "desc"])} for row in rows]
        npc_by_hero = collections.defaultdict(list)
        for row in self.manifest.get("npcRows", []):
            if row.get("heroId") and row.get("nameId") in self.manifest["displayNames"]:
                npc_by_hero[row["heroId"]].append(row)
        for hero, rows in npc_by_hero.items():
            texts = {self.manifest["displayNames"][row["nameId"]]["text"] for row in rows}
            key = "name_" + hero
            if len(texts) == 1 and key not in self.manifest["displayNames"]:
                # Original NPC role/title is a valid descriptive catalogue label;
                # keep that scope distinct from a recovered hero's proper name.
                self.manifest["displayNames"][key] = {"text": next(iter(texts)),
                    "source": {"apk": "10056262.apk", "entry": "assets/android/tabledata/tablenpc", "field": "nameId",
                               "rowIDs": [row["npcId"] for row in rows], "textRowIDs": sorted({row["nameId"] for row in rows}),
                               "scope": "unique original NPC role label; not an invented character proper name"}}

    def run(self):
        self.tables()
        self.independent_icons()
        selected = {name: value for name, value in self.resources.items() if name in SEED_PREFABS or name.startswith("activityinnerpage/")}
        atlas_entries = set()
        for name, (entry, path_id) in selected.items():
            self.prefab(name, entry, path_id)
            file = next(iter(self.load(entry).objects)).assets_file
            atlas_entries.update("assets/bin/Data/" + ext.path for ext in file.externals if len(ext.path) == 32 and "assets/bin/Data/" + ext.path in self.names)
        # Only selected prefab's direct dependencies are inspected for UIAtlas;
        # materials/textures are then followed by exact serialized references.
        for entry in sorted(atlas_entries):
            self.atlas(entry)
        for prefab in self.manifest["activityPrefabs"].values():
            names = prefab["spriteNames"]
            assets = [(n, self.manifest["sprites"][n]) for n in names if n in self.manifest["sprites"]]
            assets.sort(key=lambda pair: -(pair[1]["width"] * pair[1]["height"]))
            prefab["previewUrls"] = [r["previewUrl"] for _, r in assets[:12]]
        mode_source = ROOT / "hyjs-server/service/adminActivityModesService.go"
        if mode_source.exists():
            mode_raw = mode_source.read_text()
            for line in mode_raw.splitlines():
                header = re.match(r'\s*\{\s*"([^\"]+)",\s*"([^\"]+)"', line)
                original = re.search(r'"(ActivityInnerPage/[^\"]+)"', line)
                if not header or not original:
                    continue
                mode, label = header.groups()
                name = original.group(1).lower()
                prefab = self.manifest["activityPrefabs"].get(name)
                self.manifest["activityModes"][mode] = {"label": label, "originalPrefab": original.group(1),
                    "resourceName": name, "prefabFound": prefab is not None,
                    "previewUrls": prefab["previewUrls"] if prefab else [],
                    "previewKind": "original art components; not a page screenshot",
                    "source": {"modeDefinition": str(mode_source.relative_to(ROOT)), "modeDefinitionSHA256": sha(mode_raw.encode()),
                               "prefab": prefab["source"] if prefab else None}}
        missing_icons = {rid: name for rid, name in self.manifest["resourceIcons"].items() if name and name not in self.manifest["sprites"]}
        self.manifest["unresolvedResourceIcons"] = missing_icons
        self.manifest["inventoryBoundary"] = {"independentIconDirectories": sorted(ICON_DIRS), "resourceManagerSeedPrefabs": sorted(selected),
                                               "directPrefabDependencyFiles": len(atlas_entries), "totalTargetEntriesRead": len(self.entry_reads),
                                               "allAndroidResourcesScanned": False}
        self.manifest["exporterSHA256"] = sha(Path(__file__).read_bytes())
        referenced = {self.output / "/".join(row["iconUrl"].split("/")[-2:]) for row in self.manifest["sprites"].values()}
        removed = []
        for file in self.output.rglob("*.png"):
            if file not in referenced:
                removed.append(str(file.relative_to(self.output)))
                file.unlink()
        self.manifest["ownedUnreferencedPNGsRemoved"] = removed
        self.output.mkdir(parents=True, exist_ok=True)
        (self.output / "content-manifest.json").write_text(json.dumps(self.manifest, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps({"sprites": len(self.manifest["sprites"]), "atlases": len(self.manifest["atlases"]),
                          "activityPrefabs": len(self.manifest["activityPrefabs"]), "unresolvedResourceIcons": len(missing_icons),
                          "bytesPNG": sum(p.stat().st_size for p in self.output.rglob("*.png")), "scannedEntries": len(self.entry_reads)}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apk", type=Path, default=ROOT / "apks/original/10056262.apk")
    parser.add_argument("--output", type=Path, default=ROOT / "admin-web/public/media")
    parser.add_argument("--resume", action="store_true", help="Verify/reuse this exporter's deterministic original PNGs after a saved runner failure")
    args = parser.parse_args()
    with args.apk.open("rb") as source:
        if hashlib.file_digest(source, "sha256").hexdigest() != ORIGINAL_SHA:
            raise ValueError("Original APK SHA differs")
    if args.output.exists() and any(args.output.iterdir()) and not args.resume:
        raise ValueError("Use an empty owned media output; no unrelated file is overwritten")
    with zipfile.ZipFile(args.apk) as apk:
        Exporter(apk, args.output).run()


if __name__ == "__main__":
    main()
