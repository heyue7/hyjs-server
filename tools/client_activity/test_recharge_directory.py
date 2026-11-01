"""Exercise the real recharge helper against a bounded native-layout model."""
import hashlib
import io
import json
import math
from pathlib import Path
import struct
import subprocess
import tempfile
import zipfile

import UnityPy
from PIL import ImageFont
from fontTools.ttLib import TTFont

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'tools/client_activity/InstanceSelector.cs'
HARNESS = ROOT / 'tools/client_activity/RechargeDirectoryHarness.cs'
APK = Path('/Users/lirui/LR/hyjs/deliveries/client-update-1.1.5/黑衣剑士灵魂之花v1.1.5-采集恢复候选.apk')
TOOLS = Path('/Users/lirui/LR/hyjs/.tools/native-batch')
APK_SHA = 'bd3e4caefe47545de7b2272681a363f79df5d75838b96bc752e882cdbb07137e'
PREFAB = 'assets/bin/Data/9b5b1df9f9349064bafc32ca504c5f94'
PREFAB_SHA = '3d43f9d036c33602258439390deee56817a09a7a73341f83f1fc5f2dcbddca25'
FONT_SHA = '980b989c1d5b3a30e10e23a6ddd9fd77baafc1f18d1c3d5fa21b5b8b221c4755'


def digest(data):
    return hashlib.sha256(data).hexdigest()


def section(source, name):
    start = '// ' + name + '_TEST_BEGIN'
    end = '// ' + name + '_TEST_END'
    assert source.count(start) == source.count(end) == 1
    return source.split(start, 1)[1].split(end, 1)[0]


def run_model():
    code = SOURCE.read_text()
    code = ('using System; using System.Collections.Generic; using System.Reflection; '
            'using UnityEngine; using UI; using ProtoBuf.Message;\n'
            'namespace HYJS.ActivitySafety {\n' + section(code, 'RECHARGE_DISPLAY') +
            section(code, 'RECHARGE_DIRECTORY') + '\n}\n')
    dotnet = TOOLS / 'dotnet/dotnet'
    compiler = next((TOOLS / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    references = next((TOOLS / 'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    with tempfile.TemporaryDirectory(prefix='hyjs-recharge-model-') as tmp:
        tmp = Path(tmp)
        extracted = tmp / 'ProductionRecharge.cs'
        extracted.write_text(code)
        program = tmp / 'RechargeDirectoryHarness.dll'
        args = ['rtk', 'proxy', str(dotnet), str(compiler), '/nologo', '/target:exe', '/nostdlib+',
                '/langversion:3', '/codepage:65001', '/out:' + str(program)]
        args += ['/reference:' + str(p) for p in sorted(references.glob('*.dll'))]
        args += [str(HARNESS), str(extracted)]
        built = subprocess.run(args, capture_output=True, text=True, check=False)
        if built.returncode:
            raise RuntimeError(built.stdout + built.stderr)
        (tmp / 'RechargeDirectoryHarness.runtimeconfig.json').write_text(json.dumps({
            'runtimeOptions': {'tfm': 'net8.0', 'framework': {'name': 'Microsoft.NETCore.App', 'version': '8.0.0'}}
        }))
        result = subprocess.run(['rtk', 'proxy', str(dotnet), str(program)], capture_output=True, text=True, check=False)
        if result.returncode:
            raise RuntimeError(result.stdout + result.stderr)
        return result.stdout.strip()


def inspect_prefab():
    with APK.open('rb') as src:
        assert hashlib.file_digest(src, 'sha256').hexdigest() == APK_SHA
    with zipfile.ZipFile(APK) as apk:
        raw = apk.read(PREFAB)
        assert digest(raw) == PREFAB_SHA
        objects = {obj.path_id: obj for obj in UnityPy.load(raw).objects}
        # Verified serialized NGUI fields in the fixed original prefab.
        arrangement, columns, width, height, _, _ = struct.unpack_from('<iiffff', objects[134].get_raw_data(), 32)
        assert (arrangement, columns, width, height) == (1, 1, 518.0, 148.0)
        assert struct.unpack_from('<4f', objects[145].get_raw_data(), 172) == (0.0, 0.0, 518.0, 404.0)
        assert struct.unpack_from('<4f', objects[102].get_raw_data(), 172) == (0.0, 0.0, 345.0, 110.0)
        assert struct.unpack_from('<iq', objects[136].get_raw_data(), 168) == (2, 191)
        font_file = b''.join(apk.read('assets/bin/Data/sharedassets0.assets.split' + str(i)) for i in range(16))
        font = next(obj.read() for obj in UnityPy.load(font_file).objects if obj.type.name == 'Font' and obj.path_id == 191)
        font_data = bytes(font.m_FontData)
        assert font.m_Name == 'Words' and digest(font_data) == FONT_SHA
    return font_data


def inspect_geometry(font_data):
    # Layout values also appear in the verbatim managed helper/model assertions.
    font = ImageFont.truetype(io.BytesIO(font_data), 18)
    cmap = TTFont(io.BytesIO(font_data)).getBestCmap()
    sample = ['测' * 80, '活动截止 2026-11-04 00:00:00', '领奖截止 2026-11-05 23:59:59']
    assert all(ord(ch) in cmap for line in sample for ch in line)
    lines = []
    for part in sample:
        line = ''
        for ch in part:
            if line and font.getlength(line + ch) > 472:
                lines.append(line)
                line = ''
            line += ch
        lines.append(line)
    line_height = sum(font.getmetrics())
    assert len(lines) == 6 and line_height * len(lines) <= 130
    assert max(font.getlength(line) for line in lines) <= 472
    # At row origin: title bottom=33; caption y=27..-103; the original
    # reward clip's upper edge is 40 and shifts down by 152.
    assert 27 < 33 and 27 - 130 > 40 - 152
    # Original background bottom is -74; next title reaches +73. The new
    # 300-unit cell leaves a one-unit gap between consecutive rows.
    assert -74 - 152 > 73 - 300
    content_top = 73
    last_bottom = -300 - 74 - 152
    assert content_top - last_bottom > 404
    assert math.isclose((content_top - last_bottom) - 404, 195.0)
    return {'sampleLines': len(lines), 'maxLineWidth': max(font.getlength(line) for line in lines),
            'fontLineHeight': line_height, 'captionHeight': 130, 'rowSpacing': 300,
            'viewportHeight': 404, 'twoRowScrollRangeModel': 195}


if __name__ == '__main__':
    report = {'model': run_model(), 'originalPrefabAndFont': inspect_geometry(inspect_prefab()),
              'apkSHA256': APK_SHA, 'prefabSHA256': PREFAB_SHA, 'fontSHA256': FONT_SHA,
              'limits': 'Model and fixed prefab/font inspection; no Unity runtime or device execution.'}
    print(json.dumps(report, ensure_ascii=False, indent=2))
