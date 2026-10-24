"""Pinned feedback65 overlay. Explicit execution compiles; never packages or uses a device."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
from tools.client_arena_state.build import methods, exact_body
from tools.client_one_key59.build import verify as verify_inventory

MANAGED = 'assets/bin/Data/Managed/'
MAIN = MANAGED + 'Assembly-CSharp.dll'
BASE = 'f3ce73cc1f2e41bb45873c64f01e9a6ec81d84d1e41f18482c27d62c42ace3f9'
MAIN_SHA = '57f12e1c1aaa0ee46bc00d12be6b06bba5fda16533b47cc50a02c19c5e5e4f22'
# Full raw method bodies, including headers, are independently pinned.
PINS = {
    0x3bb8: ('PlaybackKit::Start', 'fe4bb19351224fbb51c07e65b3e33e4807e7ba68ff79a4ac3394b68502d7440a', 0),
    0x5284: ('UI.SelectReward::DoOpen', 'cf6c8828a3c3122cb3e84e461a3c1fa9f381e9b758c9fccc21f70756fee38513', 0),
    0x5e8b: ('UI.RushPage::OnFightBtnClick', 'd2555df298ed543ab76e0081003639974527f9af9f0903b1d09610b23557ee3f', 8),
    0x5e71: ('UI.RushFightReadyItem::OnFightBtnClick', 'fcfd042bc4de062c21ad73ee20b71e7ed1af6ea4ec0e510e642d470aaf61929c', 7),
    0x5e66: ('UI.RushEliteBossPage::OnFightBtnClick', '319b4d87de7de727321ced642713ce794b9162ae97ac540b51a8d57c2a191b05', 170),
}
WANTED = {
    0x504e: ('UI.WantedMission::SendEnterBattle', '237b3a4f08687cfc90f8b12acc2b4a80e6c5ecdb39b00a85c20f914c2ff29e4b', 'WantedRequest', 3),
    0x504f: ('UI.WantedMission::EnterBattleCallback', '980404d4a1b6210e9b7a12565fb689e6a4fc63577404d4e8934a6a81439a2beb', 'WantedStart', 6),
    0x2b6b: ('Battle.WantedBattleOver::AskWantedFightResult', '164a7fbacf02410dfafb6d8716f6d5d927662f852385ae6dee9da61d72c4ebec', 'WantedSettlement', 2),
    0x2b6d: ('Battle.WantedBattleOver::<AskWantedFightResult>m__E', '92bc6e6af08a32a0cc52f7f3b508bc74bb3175a7ecdf640fcc09af1f29c4763c', 'WantedResult', 6),
    0x504d: ('UI.WantedMission::OnNormalHit', '5c9b10678e5873bae5216983c1d45d68dd0fe63cc09e180e77721ded371ce742', 'WantedResume', 5),
    0x5050: ('UI.WantedMission::OnDeadlyHit', 'd220ac43ad55de0d0c5da6cbe8782e190d519db98fb3dcbb826d92d644143763', 'WantedResume', 5),
}

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def parsed(pe, rid):
    return read_method_body_from_bytes(exact_body(pe, pe.net.mdtables.MethodDef.rows[rid - 1]))

def normalized(body, rows):
    offsets = {i.offset: n for n, i in enumerate(rows)}
    offsets[body.header_size + body.code_size] = len(rows)
    def value(i):
        v = i.operand
        if i.opcode.flow_control.name in {'Branch', 'Cond_Branch'}:
            return [offsets[x] for x in v] if isinstance(v, list) else offsets[v]
        return v.value if hasattr(v, 'value') else v.index if hasattr(v, 'index') else v
    return [(i.opcode.name, value(i)) for i in rows]

def input_proof(raw):
    assert sha(raw) == MAIN_SHA, 'Formal64 Main drift'
    pe = dnfile.dnPE(data=raw)
    lookup = {r: n for n, r, _ in methods(pe)}
    for rid, (name, digest, count) in PINS.items():
        assert lookup[rid] == name and sha(exact_body(pe, pe.net.mdtables.MethodDef.rows[rid - 1])) == digest, name
        body = parsed(pe, rid)
        assert not body.exception_handlers
        if count:
            prefix = body.instructions[:count]
            assert [i.opcode.name for i in prefix[-4:]] == ['ldstr', 'ldc.r4', 'call', 'ret']
            assert pe.net.user_strings.get(prefix[-4].operand.value & 0xffffff).value.startswith('暂未开放')
            assert prefix[-3].operand == 0 and prefix[-2].operand.value == 0x06004adb
            if rid != 0x5e66:
                assert prefix[-6].opcode.name == 'ldc.i4' and prefix[-6].operand == 6
                assert prefix[-5].opcode.name == 'ble' and prefix[-5].operand == body.instructions[count].offset
            else:
                assert [pe.net.user_strings.get(prefix[i * 5 + 2].operand.value & 0xffffff).value for i in range(33)] == [f'{i:02}' for i in range(2, 35)]
                for i in range(33):
                    group = prefix[i * 5:i * 5 + 5]
                    assert [x.opcode.name for x in group] == ['ldarg.0', 'ldfld', 'ldstr', 'call', 'brtrue']
                    assert group[1].operand.value == 0x04005a44 and group[3].operand.value == 0x0a000047 and group[4].operand == prefix[-4].offset
                assert prefix[-5].opcode.name == 'br' and prefix[-5].operand == body.instructions[count].offset
    for token, name in [(0x4b7f, 'goList'), (0x4b80, 'heroIdList'), (0x4b81, 'm_click'), (0x4b82, 'selectId')]:
        assert str(pe.net.mdtables.Field.rows[token - 1].Name) == name
    for rid, (name, digest, _, _) in WANTED.items():
        assert lookup[rid] == name and sha(exact_body(pe, pe.net.mdtables.MethodDef.rows[rid - 1])) == digest, name
        assert not parsed(pe, rid).exception_handlers
    return pe

def output_proof(before, after):
    a, b = input_proof(before), dnfile.dnPE(data=after)
    proof = verify_inventory(before, after, [{'token': hex(0x06000000 + rid)} for rid in [*PINS, *WANTED]])
    for rid, (name, digest, count) in PINS.items():
        old, new = parsed(a, rid), parsed(b, rid)
        assert not new.exception_handlers
        assert getattr(old.local_var_sig_tok, 'value', None) == getattr(new.local_var_sig_tok, 'value', None)
        if count:
            assert normalized(old, old.instructions[count:]) == normalized(new, new.instructions), name
        elif rid == 0x5284:
            assert [i.opcode.name for i in new.instructions[:2]] == ['ldarg.0', 'call']
            ref = b.net.mdtables.MemberRef.rows[(new.instructions[1].operand.value & 0xffffff) - 1]
            assert str(ref.Name) == 'ResetSelection' and str(ref.Class.row.TypeNamespace) == 'HYJS.Feedback65' and ref.Signature.value == bytes.fromhex('0001011c')
            assert normalized(old, old.instructions) == normalized(new, new.instructions[2:])
        else:
            expected = bytearray(exact_body(a, a.net.mdtables.MethodDef.rows[rid - 1])); assert expected[7] == 0x17; expected[7] = 0x16
            assert bytes(expected) == exact_body(b, b.net.mdtables.MethodDef.rows[rid - 1])
    for rid, (name, _, helper, count) in WANTED.items():
        old, new = parsed(a, rid), parsed(b, rid)
        prefix, suffix = new.instructions[:count], new.instructions[count:]
        assert normalized(old, old.instructions) == normalized(new, suffix), name
        assert not new.exception_handlers and getattr(old.local_var_sig_tok, 'value', None) == getattr(new.local_var_sig_tok, 'value', None)
        calls = [i for i in prefix if i.opcode.name == 'call']; assert len(calls) == 1
        ref = b.net.mdtables.MemberRef.rows[(calls[0].operand.value & 0xffffff) - 1]
        assert str(ref.Name) == helper and str(ref.Class.row.TypeNamespace) == 'HYJS.Feedback65'
        expected = {'WantedRequest': '0002011c08', 'WantedStart': '0003021c1c02', 'WantedSettlement': '0001011c', 'WantedResult': '0003021c1c02', 'WantedResume': '0002021c08'}
        assert ref.Signature.value.hex() == expected[helper]
        if helper in ['WantedResult', 'WantedResume', 'WantedStart']:
            assert prefix[-2].opcode.name == 'brfalse' and prefix[-2].operand == suffix[0].offset and prefix[-1].opcode.name == 'ret'
        if helper == 'WantedResume':
            assert prefix[1].opcode.name == 'ldc.i4' and prefix[1].operand == (1 if rid == 0x504d else 2)
    for table in ['AssemblyRef', 'MemberRef', 'TypeRef']:
        left, right = getattr(a.net.mdtables, table).rows, getattr(b.net.mdtables, table).rows
        assert len(right) >= len(left)
        for x, y in zip(left, right):
            # Metadata heap positions may move; compare decoded identities.
            for field in ['Name', 'TypeName', 'TypeNamespace', 'Signature', 'PublicKey', 'PublicKeyOrToken', 'Culture']:
                if hasattr(x, field):
                    xv, yv = getattr(x, field), getattr(y, field)
                    assert (xv.value if hasattr(xv, 'value') else str(xv)) == (yv.value if hasattr(yv, 'value') else str(yv)), (table, field)
            for field in ['Class', 'ResolutionScope']:
                if hasattr(x, field):
                    xv, yv = getattr(x, field), getattr(y, field)
                    assert (xv.table.name, xv.row_index) == (yv.table.name, yv.row_index)
    proof['originalEngineMethodsExact'] = 547
    proof['removedGuardOriginalSuffixBranchAndLocalsExact'] = True
    proof['originalSelectionBodyExactAfterPrefix'] = True
    proof['originalMetadataReferencesAppendOnly'] = True
    proof['wantedOriginalBodiesBranchesAndLocalsExactAfterPrefixes'] = True
    return proof

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    assert sha(args.base_apk.read_bytes()) == BASE
    out = args.output_directory.resolve(); assert out.is_relative_to(ROOT) and not out.exists()
    sources = [HERE / name for name in ['Runtime.cs', 'Injector.cs', 'build.py', 'README.md']] + [ROOT / 'tools/client_bulk_purchase/Runtime.cs']
    pins = {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in sources}
    runtime = ROOT / '.tools/native-batch'; dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    def run(label, cmd):
        result = subprocess.run(['rtk', 'proxy', *map(str, cmd)], cwd=ROOT, capture_output=True, text=True, timeout=180)
        (out / (label + '.log')).write_text(result.stdout + result.stderr)
        assert result.returncode == 0, label + ' failed; inspect output log'
    with zipfile.ZipFile(args.base_apk) as apk, tempfile.TemporaryDirectory(prefix='hyjs-feedback65-') as tmp:
        assert len(apk.namelist()) == len(set(apk.namelist()))
        before = apk.read(MAIN); input_proof(before)
        out.mkdir(parents=True)
        work = Path(tmp); refs = work / 'refs'; refs.mkdir()
        for entry in apk.namelist():
            if entry.startswith(MANAGED) and entry.endswith('.dll'):
                (refs / Path(entry).name).write_bytes(apk.read(entry))
        for name, source in [('HYJS.Feedback65.dll', HERE / 'Runtime.cs'), ('HYJS.BulkPurchase.dll', ROOT / 'tools/client_bulk_purchase/Runtime.cs')]:
            run('compile-' + name, [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/codepage:65001', '/out:' + str(out / name), *['/r:' + str(p) for p in sorted(refs.glob('*.dll')) if p.name != name], source])
        shutil.copy2(HERE / 'Injector.cs', work / 'Program.cs')
        project = work / 'Inject.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(runtime / 'injector/bin/Debug/net8.0/dnlib.dll') + '</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        result = out / 'Assembly-CSharp.dll'
        run('inject', [dotnet, work / 'bin/Debug/net8.0/Inject.dll', refs / 'Assembly-CSharp.dll', out / 'HYJS.Feedback65.dll', result])
        proof = output_proof(before, result.read_bytes())
        payload = {MANAGED + n: str((out / n).relative_to(ROOT)) for n in ['Assembly-CSharp.dll', 'HYJS.Feedback65.dll', 'HYJS.BulkPurchase.dll']}
        all_targets = {**PINS, **WANTED}
        report = dict(schema='hyjs-feedback65-managed-v1', baseAPKSHA256=BASE, inputManagedSHA256=sha(before), outputManagedSHA256=sha(result.read_bytes()), inputDexSHA256=sha(apk.read('classes2.dex')), payloadFiles=payload, replacementEntries={n: sha((ROOT / f).read_bytes()) for n, f in payload.items()}, sourcePins=pins, managedOnly=True, inheritedJavaByteExact=True, inheritedJavaPublicProtectedABIExact=True, allowedChangedMethods=[v[0] for v in all_targets.values()], targetPins={hex(0x06000000 + r): v[1] for r, v in all_targets.items()}, apkBuilt=False, deviceTested=False, **proof)
        assert pins == {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in sources}
        (out / 'manifest.json').write_text(json.dumps(report, indent=2) + '\n')
        print(json.dumps(proof))

if __name__ == '__main__':
    main()
