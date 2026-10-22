"""Package generated GGO subjects with the original game's rarity art and tables.

Creative image work is performed by built-in imagegen. This pipeline only scales
the supplied alpha images, composites the requested original frames, and writes
legacy Unity bundles. Original APK entries and gameplay tables are preserved.
"""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import zipfile

from PIL import Image, ImageDraw
import UnityPy

ROOT = Path(__file__).resolve().parents[2]
SOURCE = Path(__file__).resolve().parent
COLORS = {1: "white", 2: "green", 3: "blue", 4: "zi", 5: "yellow", 6: "red"}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def resize_alpha(image, size):
    # Associated alpha avoids a light/dark matte when reducing transparent art.
    return image.convert("RGBa").resize(size, Image.Resampling.LANCZOS).convert("RGBA")


def subject(path, size, bound):
    image = Image.open(path).convert("RGBA")
    alpha = image.getchannel("A")
    assert alpha.getextrema() == (0, 255), f"True alpha required: {path}"
    box = alpha.getbbox()
    assert box, f"Empty generated subject: {path}"
    cropped = image.crop(box)
    scale = min(bound / cropped.width, bound / cropped.height)
    cropped = resize_alpha(cropped, (max(1, round(cropped.width * scale)), max(1, round(cropped.height * scale))))
    output = Image.new("RGBA", (size, size))
    output.alpha_composite(cropped, ((size - cropped.width) // 2, (size - cropped.height) // 2))
    return output


def icon_bundle(template, image, name):
    env = UnityPy.load(template)
    texture_object = next(o for o in env.objects if o.type.name == "Texture2D")
    texture = texture_object.read()
    texture.m_Name = name
    texture.set_image(image, target_format=4, mipmap_count=1)
    texture.save()
    bundle = next(o for o in env.objects if o.type.name == "AssetBundle")
    tree = bundle.read_typetree()
    assert len(tree["m_Container"]) == 1
    _, info = tree["m_Container"][0]
    tree["m_Container"] = [("assets/outputbundle/itemicons/" + name + ".png", info)]
    tree["m_AssetBundleName"] = "itemicons/" + name
    bundle.save_typetree(tree)
    # Unity 5.3 identifies serialized files by CAB name across loaded bundles.
    # A cloned template must not reuse its CAB or collide with another icon.
    assert len(env.file.files) == 1, "Expected one self-contained icon serialized file"
    old_cab, serialized = next(iter(env.file.files.items()))
    cab = "CAB-" + hashlib.sha256(("hyjs-ggo-icon:" + name).encode() + image.tobytes()).hexdigest()[:32]
    assert cab != old_cab
    serialized.name = cab
    env.file.files = {cab: serialized}
    raw = env.file.save(packer="lz4")
    check = UnityPy.load(raw)
    tex = next(o.read() for o in check.objects if o.type.name == "Texture2D")
    assert tex.m_Name == name and tex.image.convert("RGBA").tobytes() == image.tobytes()
    assert not next(o for o in check.objects if o.type.name == "Texture2D").assets_file.externals
    assert list(check.file.files) == [cab]
    verified_bundle = next(o for o in check.objects if o.type.name == "AssetBundle").read_typetree()
    assert verified_bundle["m_AssetBundleName"] == "itemicons/" + name
    assert list(verified_bundle["m_Container"])[0][0] == "assets/outputbundle/itemicons/" + name + ".png"
    return raw


def append_items(raw, items):
    env = UnityPy.load(raw)
    obj = next(o for o in env.objects if o.type.name == "TextAsset")
    tree = obj.read_typetree()
    text = tree["m_Script"]
    if isinstance(text, bytes):
        text = text.decode("utf-8")
    lines = text.splitlines()
    keys = lines[1].split("\t")
    for item in items:
        assert not any(line.split("\t")[0] == item["id"] for line in lines[5:]), item["id"]
        row = {"itemId": item["id"], "name": item["name"], "star": str(item["quality"]),
               "intro": item["description"], "desc": item["description"], "category": "11",
               "selloutPrice": "0", "icon": item["icon"]}
        lines.append("\t".join(row.get(key, "") for key in keys))
    encoded = "\n".join(lines) + "\n"
    tree["m_Script"] = encoded
    obj.save_typetree(tree)
    return env.file.save(packer="lz4"), encoded


def append_item_text(raw, items):
    # CommonMethod and the original tooltip resolve name_<id>/desc_<id> keys.
    env = UnityPy.load(raw)
    obj = next(o for o in env.objects if o.type.name == "TextAsset")
    tree = obj.read_typetree()
    text = tree["m_Script"]
    if isinstance(text, bytes):
        text = text.decode("utf-8")
    lines = text.splitlines()
    existing = {line.split("\t")[0] for line in lines[5:]}
    for item in items:
        for prefix, value in [("name_", item["name"]), ("desc_", item["description"])]:
            key = prefix + item["id"]
            assert key not in existing and "\t" not in value and "\n" not in value
            lines.append(key + "\t" + value)
            existing.add(key)
    encoded = "\n".join(lines) + "\n"
    tree["m_Script"] = encoded
    obj.save_typetree(tree)
    return env.file.save(packer="lz4"), encoded


def update_bundle_index(raw, changes):
    env = UnityPy.load(raw)
    obj = next(o for o in env.objects if o.type.name == "AssetBundleManifest")
    tree = obj.read_typetree()
    original_names = dict(tree["AssetBundleNames"])
    original_infos = dict(tree["AssetBundleInfos"])
    index = {name: key for key, name in original_names.items()}
    next_id = max(original_names) + 1
    for entry, value in changes.items():
        name = entry.removeprefix("assets/android/")
        if name not in index:
            index[name] = next_id
            tree["AssetBundleNames"].append((next_id, name))
            original_infos[next_id] = {"AssetBundleHash": {}, "AssetBundleDependencies": []}
            next_id += 1
        info = copy.deepcopy(original_infos[index[name]])
        digest = hashlib.md5(value).digest()
        info["AssetBundleHash"] = {f"bytes[{i}]": byte for i, byte in enumerate(digest)}
        original_infos[index[name]] = info
    tree["AssetBundleInfos"] = list(original_infos.items())
    obj.save_typetree(tree)
    return env.file.save(packer="lz4")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    assert not args.output.exists(), "Use a fresh task-owned output"
    args.output.mkdir(parents=True)
    items = json.loads((SOURCE / "items.json").read_text())["items"]
    generated = json.loads((SOURCE / "generated-sources.json").read_text())
    frame_sources = json.loads((SOURCE / "frames/provenance.json").read_text())
    images, proofs = {}, []
    for item in items:
        name = item["icon"]
        color = COLORS[item["quality"]]
        background = SOURCE / "frames/items" / f"UI_kuang_big_di_{color}.png"
        border = SOURCE / "frames/items" / f"UI_kuang_big_bian_{color}.png"
        image = resize_alpha(Image.open(background).convert("RGBA"), (128, 128))
        image.alpha_composite(subject(SOURCE / "assets" / (name + "-source.png"), 128, 102))
        image.alpha_composite(resize_alpha(Image.open(border).convert("RGBA"), (128, 128)))
        images[name] = image
        proofs.append({"id": item["id"], "icon": name, "quality": item["quality"],
                       "background": frame_sources[background.stem], "frame": frame_sources[border.stem],
                       "generatedSubjectSHA256": sha((SOURCE / "assets" / (name + "-source.png")).read_bytes())})
    for name in ["ggo_chip_marker", "ggo_sprout"]:
        images[name] = subject(SOURCE / "assets" / (name + "-source.png"), 64, 46)
        assert images[name].getpixel((0, 0))[3] == 0
    media = ROOT / "admin-web/public/media"
    manifest_path = media / "content-manifest.json"
    manifest = json.loads(manifest_path.read_text())
    changes = {}
    with zipfile.ZipFile(args.base_apk) as archive:
        template = archive.read("assets/android/itemicons/item_icon_005")
        for name, image in images.items():
            path = media / "items" / (name + ".png")
            image.save(path, compress_level=9)
            image.save(SOURCE / "assets" / (name + ".png"), compress_level=9)
            raw = path.read_bytes()
            manifest["sprites"][name] = {"iconUrl": "/admin/media/items/" + path.name,
                "previewUrl": "/admin/media/items/" + path.name, "sha256": sha(raw), "sha": sha(raw),
                "width": image.width, "height": image.height,
                "source": {"kind": "built-in imagegen; original quality frame reused for inventory icons",
                           "promptFile": "tools/ggo_items/prompts.json", "frameProvenance": "tools/ggo_items/frames/provenance.json"}}
            changes["assets/android/itemicons/" + name] = icon_bundle(template, image, name)
            if name in ["ggo_chip_marker", "ggo_sprout"]:
                resize_alpha(image, (24, 24)).save(SOURCE / "assets" / (name + "-24.png"))
        for item in items:
            manifest["resourceIcons"][item["id"]] = item["icon"]
        changes["assets/android/tabledata/tableitem"], table = append_items(archive.read("assets/android/tabledata/tableitem"), items)
        (args.output / "TableItem.tsv").write_text(table)
        changes["assets/android/tabledata/tabletext"], text = append_item_text(archive.read("assets/android/tabledata/tabletext"), items)
        (args.output / "TableText.tsv").write_text(text)
        changes["assets/android/android"] = update_bundle_index(archive.read("assets/android/android"), changes)
        raw = archive.read("assets/android/smallassetmanifest").decode()
        lines = raw.splitlines()
        entries = {line.split("\t")[0]: line.split("\t")[1:] for line in lines[1:] if line}
        for entry, value in changes.items():
            entries[entry.removeprefix("assets/android/")] = [hashlib.md5(value).hexdigest(), str(len(value))]
        changes["assets/android/smallassetmanifest"] = (lines[0] + "\n" + "\n".join("\t".join([key, *value]) for key, value in sorted(entries.items())) + "\n").encode()
        for name, raw in changes.items():
            path = args.output / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(raw)
    icon_rows = [(name.removeprefix("assets/android/"), hashlib.md5(raw).hexdigest(), len(raw))
                 for name, raw in sorted(changes.items()) if name.startswith("assets/android/itemicons/ggo_")]
    assert len(icon_rows) == 10
    icon_manifest = "1\n" + "".join("%s\t%s\t%d\n" % row for row in icon_rows)
    generated_cs = ROOT / "tools/ggo_items/client/GGOIconManifest.cs"
    generated_cs.write_text("// Generated by build_assets.py from the exact bundle bytes.\nnamespace HYJS.GGO { internal static class GGOIconManifest { internal const string Text = "
                            + json.dumps(icon_manifest) + "; } }\n")
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    report = {"schema": "hyjs-ggo-image-assets-v1", "baseAPK": str(args.base_apk.resolve()),
              "baseAPKSHA256": sha(args.base_apk.read_bytes()), "imagegenMode": "built-in",
              "generatedSources": generated, "originalFrames": proofs,
              "files": [{"entry": name, "sha256": sha(raw), "bytes": len(raw)} for name, raw in changes.items()],
              "payloadFiles": {name: str((args.output / name).resolve()) for name in changes},
              "generatedIconManifestSHA256": sha(generated_cs.read_bytes()), "generatedIconManifest": str(generated_cs),
              "newItems": items, "runtimeVerified": False, "creativeRetouchByCode": False}
    (args.output / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    preview = Image.new("RGB", (768, 350), (34, 38, 45))
    draw = ImageDraw.Draw(preview)
    for i, (name, im) in enumerate(images.items()):
        x, y = (i % 6) * 128, (i // 6) * 175
        preview.paste(im, (x + (128 - im.width) // 2, y), im)
        draw.text((x + 5, y + 132), name.removeprefix("ggo_"), fill="white")
    preview.save(args.output / "icons-review.png")
    print(json.dumps({"images": len(images), "items": len(items), "payloadEntries": len(changes), "output": str(args.output)}, ensure_ascii=False))


if __name__ == "__main__":
    main()
