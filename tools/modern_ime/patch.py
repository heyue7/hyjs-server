"""Remove exactly the four shipped Unity keyboard class definitions, preserving all code/EH bytes.

The replacement ABI is defined in a separate dex. This does not reassemble or
reindex the original dex. Remaining class_data records are copied byte-for-byte
within their original metadata section and their class_def offsets relocated.
Original instructions, code items, EH and all ID tables stay byte-identical.
"""
import hashlib
import json
from pathlib import Path
import struct
import zlib

ORIGINAL_DEX_SHA256 = '04ba9c67e74e363a1d9bb8c50494245288d04086f7fd5576b983215c3bb4f567'
REMOVED = {'Lcom/unity3d/player/s;', 'Lcom/unity3d/player/s$1;',
           'Lcom/unity3d/player/s$2;', 'Lcom/unity3d/player/s$3;'}


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def strings_and_types(raw):
    count, offset, type_count, type_offset = struct.unpack_from('<4I', raw, 56)
    strings = []
    for i in range(count):
        at = struct.unpack_from('<I', raw, offset + i * 4)[0]
        while raw[at] & 0x80:
            at += 1
        at += 1
        # DEX stores Java modified UTF-8 (encoded NUL and optional UTF-16 surrogate code units).
        encoded = raw[at:raw.index(b'\0', at)].replace(b'\xc0\x80', b'\x00')
        strings.append(encoded.decode('utf-8', errors='surrogatepass'))
    types = [strings[struct.unpack_from('<I', raw, type_offset + i * 4)[0]] for i in range(type_count)]
    return strings, types


def definitions(raw):
    _, types = strings_and_types(raw)
    count, offset = struct.unpack_from('<2I', raw, 96)
    return [(types[struct.unpack_from('<I', raw, offset + i * 32)[0]],
             raw[offset + i * 32:offset + (i + 1) * 32]) for i in range(count)]


def uleb(raw, at):
    value = 0
    for shift in range(0, 35, 7):
        part = raw[at]; at += 1
        value |= (part & 127) << shift
        if not part & 128: return value, at
    raise ValueError('Invalid original class_data ULEB')


def class_data_end(raw, at):
    counts = []
    for _ in range(4):
        value, at = uleb(raw, at); counts.append(value)
    for _ in range((counts[0] + counts[1]) * 2 + (counts[2] + counts[3]) * 3):
        _, at = uleb(raw, at)
    return at


def patch(raw):
    if digest(raw) != ORIGINAL_DEX_SHA256:
        raise ValueError('Original classes.dex SHA mismatch; unknown or already patched input rejected')
    rows = definitions(raw)
    if {name for name, _ in rows if name in REMOVED} != REMOVED:
        raise ValueError('Exact original keyboard class family missing')
    count, offset = struct.unpack_from('<2I', raw, 96)
    kept_original = [(name, row) for name, row in rows if name not in REMOVED]
    if len(kept_original) != count - 4:
        raise ValueError('Exactly four class definitions must be removed')
    revised = bytearray(raw)
    map_offset = struct.unpack_from('<I', raw, 52)[0]
    map_count = struct.unpack_from('<I', raw, map_offset)[0]
    map_size_offset = None
    class_data_map = None
    for i in range(map_count):
        at = map_offset + 4 + i * 12
        kind, unused, size, section = struct.unpack_from('<HHII', raw, at)
        if kind == 0x0006:
            if map_size_offset is not None or section != offset or size != count or unused != 0:
                raise ValueError('Original class_def map item differs')
            map_size_offset = at + 4
        elif kind == 0x2000:
            if class_data_map is not None: raise ValueError('Duplicate class_data map')
            class_data_map = (at + 4, size, section)
    if map_size_offset is None:
        raise ValueError('Class definition map absent')
    if class_data_map is None: raise ValueError('Class data map absent')
    data_count_offset, data_count, data_start = class_data_map
    data_rows = sorted((struct.unpack_from('<I', row, 24)[0], name) for name, row in rows if struct.unpack_from('<I', row, 24)[0])
    if len(data_rows) != data_count or data_rows[0][0] != data_start:
        raise ValueError('Original class_data ownership/map count differs')
    cursor, records, relocated = data_start, [], {}
    for index, (start, name) in enumerate(data_rows):
        if start != cursor: raise ValueError('Original class_data is not a contiguous exact metadata section')
        end = class_data_end(raw, start)
        records.append((name, start, raw[start:end])); cursor = end
    data_end, target = cursor, data_start
    kept_records = []
    for name, start, record in records:
        if name in REMOVED: continue
        relocated[start] = target
        kept_records.append(record); target += len(record)
    revised[data_start:data_end] = b''.join(kept_records) + bytes(data_end - target)
    kept = []
    for name, original_row in kept_original:
        row = bytearray(original_row)
        data_offset = struct.unpack_from('<I', row, 24)[0]
        if data_offset: struct.pack_into('<I', row, 24, relocated[data_offset])
        kept.append((name, bytes(row)))
    revised[offset:offset + count * 32] = b''.join(row for _, row in kept) + bytes(4 * 32)
    struct.pack_into('<I', revised, 96, len(kept))
    struct.pack_into('<I', revised, map_size_offset, len(kept))
    struct.pack_into('<I', revised, data_count_offset, len(kept_records))
    revised[12:32] = hashlib.sha1(revised[32:]).digest()
    struct.pack_into('<I', revised, 8, zlib.adler32(revised[12:]) & 0xffffffff)
    revised = bytes(revised)
    allowed = [(8, 32), (96, 100), (offset, offset + count * 32),
               (map_size_offset, map_size_offset + 4),
               (data_count_offset, data_count_offset + 4), (data_start, data_end)]
    cursor = 0
    for start, end in sorted(allowed):
        if raw[cursor:start] != revised[cursor:start]:
            raise ValueError('Bytes outside the exact metadata whitelist changed')
        cursor = end
    if raw[cursor:] != revised[cursor:] or len(raw) != len(revised):
        raise ValueError('Original DEX code/data bytes changed')
    if definitions(revised) != kept:
        raise ValueError('Non-keyboard class definitions were changed or reordered')
    proof = {'adapter': 'unity-current-ime-no-dialog-v1',
             'originalDexSHA256': digest(raw), 'patchedDexSHA256': digest(revised),
             'removedClassDefinitions': sorted(REMOVED), 'removedCount': 4,
             'originalClassCount': count, 'retainedClassCount': len(kept),
             'allowedByteRanges': allowed, 'dexByteLengthUnchanged': True,
             'allNonKeyboardClassDefinitionsUnchangedExceptClassDataOffset': True,
             'allRetainedClassDataRecordsByteIdentical': True,
             'removedClassDataCount': data_count - len(kept_records),
             'allOriginalCodeItemsAndExceptionHandlersByteIdentical': True,
             'allOriginalStringTypeProtoFieldMethodIdsByteIdentical': True,
             'oldKeyboardCodeItemsRetainedAsUnreferencedData': True,
             'newKeyboardDefinitionEntry': 'classes3.dex',
             'apkPackaged': False, 'deviceExecuted': False}
    return revised, proof


if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--input-dex', required=True)
    parser.add_argument('--output-dex', required=True)
    parser.add_argument('--proof', required=True)
    args = parser.parse_args()
    destination, proof_path = Path(args.output_dex), Path(args.proof)
    if destination.exists() or proof_path.exists():
        raise ValueError('Use fresh output paths; preserve prior evidence')
    output, proof = patch(Path(args.input_dex).read_bytes())
    destination.parent.mkdir(parents=True, exist_ok=True)
    proof_path.parent.mkdir(parents=True, exist_ok=True)
    destination.write_bytes(output)
    proof_path.write_text(json.dumps(proof, indent=2) + '\n')
