"""Append original bundled missing glyphs while preserving old glyphs and Unity objects."""
import argparse
import copy
import hashlib
from io import BytesIO
import json
from pathlib import Path
import struct
import zipfile

import fontTools
from fontTools.ttLib import TTFont
from fontTools.ttLib.scaleUpem import scale_upem
from fontTools.ttLib.tables._g_l_y_f import Glyph
import UnityPy
from UnityPy.files.ObjectReader import ObjectReader

APK_SHA = "9fec499af7473f5fd05da31670e27b3e9eb270ba9b0c9d52bd6f3238e1425285"
SHARED_SHA = "000d67a882772004f08bb9defc16e0cd9e42e290c4f3251a8ca4d90334dec6b8"
FONT_SHAS = {
    190: "d6c6319ead092fa606fe8942544d666c547d1469b6e23aa7abc26666a563859c",
    191: "c14a2c7606b46548f9a6af86a5c641541831a084d244f4c1533709ce87849371",
}
FONTTOOLS_VERSION = "4.67.0"
PREFIX = "assets/bin/Data/sharedassets0.assets.split"
ROOT = Path(__file__).resolve().parents[2]
LAYOUT_KEYS = ("ascent", "descent", "lineGap", "caretSlopeRise", "caretSlopeRun", "caretOffset")
HEAD_KEYS = ("unitsPerEm", "created", "modified", "macStyle", "flags", "lowestRecPPEM", "fontDirectionHint")


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def require(condition, message):
    if not condition:
        raise ValueError(message)


def font(raw):
    return TTFont(BytesIO(raw), recalcBBoxes=False, recalcTimestamp=False)


def glyph_bytes(ttf, index):
    offsets = ttf["loca"].locations
    return ttf.reader["glyf"][offsets[index]:offsets[index + 1]]


def fill_missing(target_raw, donor_raw, requested=None):
    """Keep target glyph indices/raw slices; append renamed, scaled, dehinted donor glyphs."""
    target, donor = font(target_raw), font(donor_raw)
    old_order = target.getGlyphOrder()[:]
    old_metrics = dict(target["hmtx"].metrics)
    old_raw = {name: glyph_bytes(target, i) for i, name in enumerate(old_order)}
    old_cmaps = [dict(table.cmap) for table in target["cmap"].tables]
    donor_cmap, target_cmap = dict(donor.getBestCmap()), dict(target.getBestCmap())
    missing = sorted(set(donor_cmap) - set(target_cmap) if requested is None else set(requested))
    require(all(cp in donor_cmap and cp not in target_cmap for cp in missing), "Requested codepoints must be donor-only")
    scale = target["head"].unitsPerEm / donor["head"].unitsPerEm
    scale_upem(donor, target["head"].unitsPerEm)
    appended = {}

    def append_source(source_name):
        if source_name in appended:
            return appended[source_name]
        name = "BundledFill." + str(len(appended))
        require(name not in target["glyf"].glyphs, "Appended glyph name collision")
        appended[source_name] = name
        glyph = copy.deepcopy(donor["glyf"][source_name])
        if glyph.isComposite():
            for component in glyph.components:
                component.glyphName = append_source(component.glyphName)
        glyph.removeHinting()
        target["glyf"].glyphs[name] = glyph
        target["hmtx"].metrics[name] = donor["hmtx"].metrics[source_name]
        return name

    for cp in missing:
        append_source(donor_cmap[cp])
    target.setGlyphOrder(old_order + list(appended.values()))
    # Raw original glyph bodies retain their exact instructions and component indices.
    for name, raw in old_raw.items():
        target["glyf"].glyphs[name] = Glyph(raw)
    for table in target["cmap"].tables:
        if table.isUnicode():
            for cp in missing:
                table.cmap[cp] = appended[donor_cmap[cp]]
    for name in appended.values():
        glyph = target["glyf"][name]
        if glyph.isComposite():
            points, contours, depth = glyph.getCompositeMaxpValues(target["glyf"])
            for field, value in (("maxCompositePoints", points), ("maxCompositeContours", contours),
                                 ("maxComponentElements", len(glyph.components)), ("maxComponentDepth", depth)):
                setattr(target["maxp"], field, max(getattr(target["maxp"], field), value))
        elif glyph.numberOfContours > 0:
            target["maxp"].maxPoints = max(target["maxp"].maxPoints, len(glyph.coordinates))
            target["maxp"].maxContours = max(target["maxp"].maxContours, glyph.numberOfContours)
        if not glyph.numberOfContours:
            continue
        # These whole-font bounding statistics must enclose the new outlines.
        for key in ("xMin", "yMin"):
            setattr(target["head"], key, min(getattr(target["head"], key), getattr(glyph, key)))
        for key in ("xMax", "yMax"):
            setattr(target["head"], key, max(getattr(target["head"], key), getattr(glyph, key)))
        advance, lsb = target["hmtx"].metrics[name]
        width = glyph.xMax - glyph.xMin
        for key, value, operation in (("advanceWidthMax", advance, max), ("minLeftSideBearing", lsb, min),
                                      ("minRightSideBearing", advance - lsb - width, min), ("xMaxExtent", lsb + width, max)):
            setattr(target["hhea"], key, operation(getattr(target["hhea"], key), value))
    target["maxp"].numGlyphs = len(target.getGlyphOrder())
    buffer = BytesIO()
    target.save(buffer, reorderTables=False)
    merged_raw = buffer.getvalue()
    merged, original = font(merged_raw), font(target_raw)
    merged_order = merged.getGlyphOrder()
    require(merged_order[:len(old_order)] == old_order, "Original glyph order/name changed")
    glyphs = []
    for index, name in enumerate(old_order):
        actual = glyph_bytes(merged, index)
        require(actual == old_raw[name], "Original glyph body changed: " + name)
        require(merged["hmtx"].metrics[name] == old_metrics[name], "Original glyph metrics changed: " + name)
        glyphs.append(dict(glyphID=index, name=name, rawSHA256=sha(actual), rawEqual=True, hmtxEqual=True))
    cmaps = []
    for i, old in enumerate(old_cmaps):
        require(all(merged["cmap"].tables[i].cmap.get(cp) == name for cp, name in old.items()), "Original cmap mapping changed")
        cmaps.append(dict(index=i, originalMappings=len(old), allOriginalMappingsEqual=True))
    donor_rows = []
    new_names = {name: merged_order[len(old_order) + i] for i, name in enumerate(appended.values())}
    for source_name, name in appended.items():
        actual_name = new_names[name]
        expected = donor["glyf"][source_name].getCoordinates(donor["glyf"])
        actual_glyph = merged["glyf"][actual_name]
        actual = actual_glyph.getCoordinates(merged["glyf"])
        require(list(actual[0]) == list(expected[0]) and list(actual[1]) == list(expected[1]) and list(actual[2]) == list(expected[2]), "Scaled donor geometry changed")
        require(merged["hmtx"].metrics[actual_name] == donor["hmtx"].metrics[source_name], "Scaled donor metrics changed")
        require(not hasattr(actual_glyph, "program") or not actual_glyph.program.getBytecode(), "Donor hint instructions remain")
        donor_rows.append(dict(sourceGlyph=source_name, targetGlyphID=merged.getGlyphID(actual_name), geometryEqual=True, hmtxEqual=True, hintInstructionsEmpty=True))
    for key in HEAD_KEYS:
        require(getattr(original["head"], key) == getattr(merged["head"], key), "Original head field changed: " + key)
    for key in LAYOUT_KEYS:
        require(getattr(original["hhea"], key) == getattr(merged["hhea"], key), "Original line metric changed: " + key)
    mutable_tables = {"head", "hhea", "maxp", "hmtx", "cmap", "loca", "glyf", "post"}
    require(set(original.reader.keys()) == set(merged.reader.keys()), "Font tables added or removed")
    changed_tables = []
    for tag in original.reader.keys():
        if original.reader[tag] != merged.reader[tag]:
            require(tag in mutable_tables, "Unexpected font table mutation: " + tag)
            changed_tables.append(tag)
    derived = {}
    for tag, keys in (("head", ("xMin", "yMin", "xMax", "yMax", "checkSumAdjustment", "indexToLocFormat")),
                      ("hhea", ("numberOfHMetrics", "advanceWidthMax", "minLeftSideBearing", "minRightSideBearing", "xMaxExtent")),
                      ("maxp", ("numGlyphs", "maxPoints", "maxContours", "maxCompositePoints", "maxCompositeContours", "maxComponentElements", "maxComponentDepth")),
                      ("post", ("formatType",))):
        for key in keys:
            old, new = getattr(original[tag], key), getattr(merged[tag], key)
            if old != new:
                derived[tag + "." + key] = dict(before=old, after=new)
    expected_coverage = set(target_cmap) | set(missing)
    require(set(merged.getBestCmap()) == expected_coverage, "Merged Unicode coverage differs")
    return merged_raw, dict(sourceSHA256=sha(target_raw), donorSHA256=sha(donor_raw), outputSHA256=sha(merged_raw),
                            originalGlyphs=len(old_order), appendedGlyphs=len(appended), addedCodepoints=missing,
                            originalUnicodeCoverage=len(target_cmap), outputUnicodeCoverage=len(expected_coverage),
                            donorScale=scale, sourceBytes=len(target_raw), outputBytes=len(merged_raw),
                            allOriginalGlyphsPreserved=True, allOriginalHmtxPreserved=True, originalCmaps=cmaps,
                            originalLineMetricsPreserved=True, originalNameAndHintTablesPreserved=True,
                            changedTables=changed_tables, changedDerivedFields=derived, glyphs=glyphs, donorGlyphs=donor_rows)


def object_metadata(obj):
    return {key: getattr(obj, key, None) for key in ("path_id", "byte_start", "byte_size", "type_id", "class_id", "is_destroyed", "is_stripped")}


def read_objects(raw, positions=None):
    if positions is None:
        return {obj.path_id: obj for obj in UnityPy.load(raw).objects}
    original = ObjectReader.from_reader.__func__

    def record(cls, assets_file, reader):
        start = reader.Position
        obj = original(cls, assets_file, reader)
        if obj.path_id in FONT_SHAS:
            version = assets_file.header.version
            require(version == 15, "Serialized-file version drift")
            if not assets_file.big_id_enabled:
                start = (start + 3) & ~3
            path_bytes = 8 if assets_file.big_id_enabled or version >= 14 else 4
            positions[obj.path_id] = dict(start=start, offsetField=start + path_bytes, sizeField=start + path_bytes + 4,
                                         endian=reader.endian, dataOffset=assets_file.header.data_offset,
                                         fileSize=assets_file.header.file_size)
        return obj

    ObjectReader.from_reader = classmethod(record)
    try:
        return {obj.path_id: obj for obj in UnityPy.load(raw).objects}
    finally:
        ObjectReader.from_reader = classmethod(original)


def build_overlay(base_apk):
    """Recompute a scoped overlay from pinned APK/font bytes; never trust a prior manifest."""
    require(fontTools.__version__ == FONTTOOLS_VERSION, "fontTools version must be " + FONTTOOLS_VERSION)
    base_apk = Path(base_apk)
    with base_apk.open("rb") as stream:
        require(hashlib.file_digest(stream, "sha256").hexdigest() == APK_SHA, "Base APK SHA256 drift")
    names = [PREFIX + str(i) for i in range(11)]
    with zipfile.ZipFile(base_apk) as archive:
        segments = [archive.read(name) for name in names]
    require([len(part) for part in segments] == [1048576] * 10 + [689188], "Shared asset split sizes drift")
    raw = b"".join(segments)
    require(sha(raw) == SHARED_SHA, "Shared asset SHA256 drift")
    positions = {}
    before = read_objects(raw, positions)
    require(len(before) == 515 and set(positions) == set(FONT_SHAS), "Object inventory drift")
    source_fonts, old_trees = {}, {}
    for pid, expected in FONT_SHAS.items():
        require(before[pid].type.name == "Font", "Font object type drift")
        tree = before[pid].read_typetree()
        require(tree["m_Name"] == ("Numbers" if pid == 190 else "Words"), "Font name drift")
        data = bytes(tree["m_FontData"])
        require(sha(data) == expected, "Font SHA256 drift")
        source_fonts[pid], old_trees[pid] = data, tree
    merged, font_proofs = {}, {}
    merged[190], font_proofs[190] = fill_missing(source_fonts[190], source_fonts[191])
    require(len(font_proofs[190]["addedCodepoints"]) == 7455, "Numbers missing-codepoint inventory drift")
    merged[191], font_proofs[191] = fill_missing(source_fonts[191], source_fonts[190], (0x00A9, 0x2022))
    edited = read_objects(raw)
    output = bytearray(raw)
    allowed = set(range(4, 8))
    payloads = []
    for pid in (190, 191):
        position = positions[pid]
        require(position["fileSize"] == len(raw), "Serialized file-size drift")
        offset, size, endian = position["offsetField"], position["sizeField"], position["endian"]
        require(struct.unpack_from(endian + "I", raw, offset)[0] == before[pid].byte_start - position["dataOffset"], "Object offset parse differs")
        require(struct.unpack_from(endian + "I", raw, size)[0] == before[pid].byte_size, "Object size parse differs")
        tree = edited[pid].read_typetree()
        tree["m_FontData"] = list(merged[pid])
        payload = edited[pid].save_typetree(tree)
        start = (len(output) + 7) & ~7
        output.extend(b"\0" * (start - len(output)))
        output.extend(payload)
        struct.pack_into(endian + "I", output, offset, start - position["dataOffset"])
        struct.pack_into(endian + "I", output, size, len(payload))
        allowed.update(range(offset, offset + 4))
        allowed.update(range(size, size + 4))
        payloads.append(dict(pathId=pid, absoluteStart=start, byteSize=len(payload), objectRecord=position))
    struct.pack_into(">I", output, 4, len(output))
    output = bytes(output)
    changed_offsets = [i for i in range(len(raw)) if raw[i] != output[i]]
    require(set(changed_offsets) <= allowed, "Unexpected original serialized bytes changed")
    after = read_objects(output)
    require(set(after) == set(before), "Reloaded object inventory differs")
    object_rows = []
    for pid, old in before.items():
        new = after[pid]
        raw_equal = old.get_raw_data() == new.get_raw_data()
        metadata_equal = object_metadata(old) == object_metadata(new)
        if pid not in FONT_SHAS:
            require(raw_equal and metadata_equal, "Non-target object changed: " + str(pid))
        else:
            new_tree = new.read_typetree()
            require(bytes(new_tree["m_FontData"]) == merged[pid], "Reloaded FontData differs")
            require(all(value == new_tree[key] for key, value in old_trees[pid].items() if key != "m_FontData"), "Other Unity Font field changed")
        object_rows.append(dict(pathId=pid, type=old.type.name, rawEqual=raw_equal, metadataEqual=metadata_equal,
                                beforeSHA256=sha(old.get_raw_data()), afterSHA256=sha(new.get_raw_data()),
                                before=object_metadata(old), after=object_metadata(new)))
    # Unity's split reader addresses parts by 1 MiB index. Extend the tail
    # with new fixed-size parts instead of creating an oversized final part.
    part_size = 1048576
    chunks = [output[start:start + part_size] for start in range(0, len(output), part_size)]
    require(b"".join(chunks) == output, "Split reassembly differs")
    require(all(len(part) == part_size for part in chunks[:-1]), "Non-final split size differs")
    output_names = [PREFIX + str(i) for i in range(len(chunks))]
    entries = {name: chunk for i, (name, chunk) in enumerate(zip(output_names, chunks))
               if i >= len(segments) or chunk != segments[i]}
    expected_changed = {names[0], names[10]} | set(output_names[len(segments):])
    require(set(entries) == expected_changed, "Unexpected changed APK entry")
    require(chunks[10][:len(segments[10])] == segments[10], "Original final-part bytes changed")
    proof = dict(schema="hyjs-bundled-font-fill-v1", status="STATIC_VERIFIED_NOT_PACKAGED", baseAPKSHA256=APK_SHA,
                 sourceSharedSHA256=SHARED_SHA, outputSharedSHA256=sha(output), sourceBytes=len(raw), outputBytes=len(output),
                 sourceSHA256={"tools/client_font_fallback/build.py": sha(Path(__file__).read_bytes())},
                 fontToolsVersion=fontTools.__version__, fontToolsVersionPin=FONTTOOLS_VERSION, unityPyVersion=UnityPy.__version__,
                 fonts={str(pid): value for pid, value in font_proofs.items()}, appendedPayloads=payloads,
                 changedOriginalByteOffsets=changed_offsets, onlyFileSizeAndTargetRecordsChanged=True,
                 nonTargetObjectCount=513, allNonTargetObjectsRawAndMetadataEqual=True,
                 allOtherTargetFontFieldsEqual=True, objects=object_rows,
                 originalSplitBoundariesPreserved=True,
                 fixedSplitPartSize=part_size, newSplitEntries=output_names[len(segments):],
                 segments=[dict(entry=name, beforeSize=len(segments[i]) if i < len(segments) else 0,
                                afterSize=len(new), beforeSHA256=sha(segments[i]) if i < len(segments) else None,
                                afterSHA256=sha(new), changed=i >= len(segments) or segments[i] != new)
                           for i, (name, new) in enumerate(zip(output_names, chunks))],
                 replacementEntries=list(entries), modifiedManagedAssemblies=[], externalFontsUsed=[], deviceExecuted=False)
    return entries, proof


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, default=ROOT / "build/hyjs-local.apk")
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    require(not args.output_directory.exists(), "Output directory already exists; refusing overwrite")
    entries, proof = build_overlay(args.base_apk)
    args.output_directory.mkdir(parents=True, exist_ok=False)
    for name, raw in entries.items():
        destination = args.output_directory / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(raw)
    (args.output_directory / "manifest.json").write_text(json.dumps(proof, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": proof["status"], "replacementEntries": proof["replacementEntries"],
                      "outputSharedSHA256": proof["outputSharedSHA256"], "outputDirectory": str(args.output_directory)}, ensure_ascii=False))


if __name__ == "__main__":
    main()
