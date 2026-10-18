"""Verify the wallet-only dex and packaging proposal without touching an APK."""
import hashlib
from pathlib import Path
import struct
import xml.etree.ElementTree as ET

ANDROID = '{http://schemas.android.com/apk/res/android}'


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def dex_classes(raw):
    if not raw.startswith(b'dex\n') or len(raw) < 112:
        raise ValueError('Invalid wallet dex')
    string_count, string_offset, type_count, type_offset = struct.unpack_from('<4I', raw, 56)
    strings = []
    for index in range(string_count):
        at = struct.unpack_from('<I', raw, string_offset + index * 4)[0]
        while raw[at] & 0x80:
            at += 1
        at += 1
        strings.append(raw[at:raw.index(b'\0', at)].decode('utf-8', errors='replace'))
    types = [strings[struct.unpack_from('<I', raw, type_offset + index * 4)[0]] for index in range(type_count)]
    count, offset = struct.unpack_from('<2I', raw, 96)
    return [types[struct.unpack_from('<I', raw, offset + index * 32)[0]] for index in range(count)]


def verify(dex, manifest):
    classes = dex_classes(Path(dex).read_bytes())
    if not classes or any(not name.startswith('Lcom/hyjs/wallet/') for name in classes):
        raise ValueError('Wallet dex includes original Unity or unrelated class definitions')
    if 'Lcom/hyjs/wallet/WalletActivity;' not in classes:
        raise ValueError('Wallet activity absent')
    root = ET.parse(manifest).getroot()
    activities = root.findall('./application/activity')
    launch = [activity for activity in activities if activity.find('./intent-filter/action[@' + ANDROID + 'name="android.intent.action.MAIN"]') is not None]
    if len(launch) != 1 or launch[0].get(ANDROID + 'name') != 'com.hyjs.wallet.WalletActivity':
        raise ValueError('Wallet must extend the sole original launcher activity')
    if root.find('./uses-permission[@' + ANDROID + 'name="android.permission.SYSTEM_ALERT_WINDOW"]') is not None:
        raise ValueError('In-activity wallet must not request overlay permission')
    return {'definedClasses': classes, 'definedClassCount': len(classes),
            'unityStubAbsent': True, 'unrelatedClassesAbsent': True,
            'launcher': 'com.hyjs.wallet.WalletActivity', 'systemOverlayPermission': False,
            'dexSHA256': digest(dex), 'manifestProposalSHA256': digest(manifest),
            'phoneUI': False, 'apiExecuted': False, 'originalAPKTouched': False}
