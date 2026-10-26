"""Reuse the existing binary-AXML parser; change only the frozen input's package."""
import struct
from tools import build_multiclient as inherited


def patch_package(original):
    pool, flags, strings, serialized = inherited.read_pool(original)
    offset, _, header, size = pool
    attrs = inherited.literal_attributes(original, strings)
    packages = [(raw, kind, value) for tag, ns, name, raw, kind, value in attrs
                if tag == "manifest" and ns == "" and name == "package"]
    if len(packages) != 1:
        raise ValueError("Expected exactly one manifest package")
    raw, kind, index = packages[0]
    if kind != 3 or raw != index or strings[index] != inherited.SOURCE_PACKAGE:
        raise ValueError("Frozen QA input must retain the reviewed .local package")
    for tag, ns, name, raw, kind, value in attrs:
        if raw == index or kind == 3 and value == index:
            if (tag, ns, name) != ("manifest", "", "package"):
                raise ValueError("Package string is shared by an unrelated attribute")
    labels = [strings[v] for tag, ns, name, raw, kind, v in attrs
              if tag == "application" and ns == inherited.ANDROID_NS and name == "label" and kind == 3]
    if len(labels) != 1:
        raise ValueError("Expected one inherited literal application label")
    text = inherited.TARGET_PACKAGE
    utf16 = text.encode("utf-16le")
    if flags & 256:
        utf8 = text.encode()
        if len(utf8) >= 128 or len(utf16) // 2 >= 128:
            raise ValueError("Package exceeds the finite short-string scope")
        replacement = bytes([len(utf16) // 2, len(utf8)]) + utf8 + b"\0"
    else:
        replacement = struct.pack("<H", len(utf16) // 2) + utf16 + b"\0\0"
    encoded = list(serialized)
    encoded[index] = replacement
    offsets, payload = [], bytearray()
    for entry in encoded:
        offsets.append(len(payload))
        payload.extend(entry)
    payload.extend(b"\0" * (-len(payload) % 4))
    new_pool = bytearray(original[offset:offset + header])
    new_pool.extend(struct.pack("<" + "I" * len(offsets), *offsets))
    new_pool.extend(payload)
    struct.pack_into("<I", new_pool, 4, len(new_pool))
    patched = bytearray(original[:offset] + new_pool + original[offset + size:])
    struct.pack_into("<I", patched, 4, len(patched))
    _, _, new_strings, new_serialized = inherited.read_pool(patched)
    if any(new_serialized[i] != serialized[i] for i in range(len(strings)) if i != index):
        raise ValueError("Unrelated label or string changed")
    if [original[o:o+n] for o,k,h,n in inherited.chunks(original) if k != 1] != [patched[o:o+n] for o,k,h,n in inherited.chunks(patched) if k != 1]:
        raise ValueError("Unrelated XML node changed")
    if new_strings[index] != inherited.TARGET_PACKAGE:
        raise ValueError("Package did not round trip")
    return bytes(patched), [{"element":"manifest","attribute":"package","before":inherited.SOURCE_PACKAGE,"after":inherited.TARGET_PACKAGE}], labels[0]
