"""Build a verified managed-only 1.1.8 Boss reward display overlay."""
import argparse
import ast
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from tools.awake_diff_rows.build import methods, operand
from tools.client_relations85.verify_existing import verify_metadata
from tools.local_asset_ready.remove_gate import exact_body, require

HERE = Path(__file__).resolve().parent
DESERT = ROOT / 'tools/desert_extraction'
BASE_APK = Path('/Users/lirui/LR/hyjs/deliveries/client-update-1.1.8/黑衣剑士灵魂之花v1.1.8.apk')
DEFAULT_RUNTIME = Path('/Users/lirui/LR/hyjs/.tools/native-batch')
DEFAULT_OUTPUT = ROOT / 'build/rank-desert-fixes-20261103/client'
BASE_REV = 'b0967e83a2327feab5c4510c13f040ba06dd8835'
BASE_APK_SHA = 'ba60ffb21debdde6e3730357195f32a9efb5f2a49ba129d3003edcc244547f2d'
BASE_MAIN_SHA = 'f1266583f7da475c80ef1fe55dda5662c13ae402ccad13b9ef70d8fe31650d57'
BASE_HELPER_SHA = 'ccdfab94f784ab9bb636dbfa7b924441c1e58ca90aaefb73834f6054a1ad3fba'
BASE_TARGET_SHA = '2571ebc25079b923dbaa3b0480c9c5a4bc2cb69ba85d122abf16d0d8f7f2a9b4'
MAIN = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER = 'assets/bin/Data/Managed/SoulFlowerDesertV4.dll'
TARGET = 'Battle.CopyBalance::Refresh'
TARGET_TOKEN = 0x06005B6E
MONSTER_GETTER = 0x06001452
BOSS_GETTER = 0x060014FA
RESULT_CALL = 0x06005B6F
HOOK_TOKEN = 0x0A0021C7
REFS = ('Assembly-CSharp.dll', 'Assembly-CSharp-firstpass.dll', 'UnityEngine.dll', 'mscorlib.dll',
        'System.dll', 'System.Core.dll', 'LitJson.dll', 'HYJS.Field63.dll', 'LocalFieldSettlement.dll')
EXTRAS = ('EncounterStartRecovery.cs', 'AwakeStartRecovery.cs', 'CopyExitRecovery.cs')
GIT_DESERT = Path('tools/desert_extraction')


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def file_sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def git_blob(path):
    return subprocess.check_output(['rtk', 'proxy', 'git', 'show', BASE_REV + ':' + path.as_posix()], cwd=ROOT)


def source_names(raw):
    tree = ast.parse(raw)
    assignments = [n for n in tree.body if isinstance(n, ast.Assign) and
                   any(isinstance(t, ast.Name) and t.id == 'SOURCES' for t in n.targets)]
    require(len(assignments) == 1, 'Expected one literal SOURCES declaration')
    names = ast.literal_eval(assignments[0].value)
    require(isinstance(names, (tuple, list)) and all(isinstance(n, str) and Path(n).name == n and n.endswith('.cs') for n in names)
            and len(names) == len(set(names)), 'Invalid helper SOURCES')
    return tuple(names)


def table_rows(pe):
    return {name: table for name, table in vars(pe.net.mdtables).items() if hasattr(table, 'rows')}


def verify_metadata_tokens(before, after):
    old, new = (dnfile.dnPE(data=raw) for raw in (before, after))
    for pe in (old, new):
        require(not getattr(pe.net.mdtables, 'FieldPtr', None) and not getattr(pe.net.mdtables, 'MethodPtr', None),
                'Unity-incompatible pointer table')
    for field in ('Flags', 'MajorRuntimeVersion', 'MinorRuntimeVersion', 'EntryPointTokenOrRva', 'StrongNameSignatureSize'):
        require(getattr(old.net.struct, field) == getattr(new.net.struct, field), 'CLI field changed: ' + field)
    require(old.net.struct.StrongNameSignatureSize == 0, 'Unexpected signed CLI image')
    old_tables, new_tables = table_rows(old), table_rows(new)
    require(set(old_tables) == set(new_tables), 'Metadata table inventory changed')
    counts = {}
    for table in sorted(old_tables):
        left, right = old_tables[table].rows, new_tables[table].rows
        require(len(left) == len(right), 'Metadata row count changed: ' + table)
        counts[table] = len(left)
        for row_number, (a, b) in enumerate(zip(left, right), 1):
            names = [name for group in a.struct.__keys__ for name in group]
            require(names == [name for group in b.struct.__keys__ for name in group],
                    'Metadata row shape changed: ' + table)
            for name in names:
                # CIL and field data can move in the PE; all row identities,
                # heap indexes, signatures, flags and coded references stay exact.
                if table in ('MethodDef', 'FieldRva') and name.lower() == 'rva':
                    continue
                require(getattr(a.struct, name) == getattr(b.struct, name),
                        'Metadata row changed: %s[%d].%s' % (table, row_number, name))
    proof = verify_metadata(before, after)
    proof.update({'allTableRowsExceptRelocatedRVAsExact': True, 'metadataTableRows': counts,
                  'cliFlagsAndIdentityExact': True, 'noUnityPointerTables': True})
    return proof


def instruction_token(instruction):
    return operand(instruction.operand)


def is_call(instruction, opcode, token):
    return instruction.opcode.name == opcode and instruction_token(instruction) == token


def method_indices(body):
    return {instruction.offset: i for i, instruction in enumerate(body.instructions)}


def original_instruction_match(old, new, old_index, new_index, new_to_old):
    require(old.opcode_bytes == new.opcode_bytes and old.size == new.size, 'Original opcode or encoding size changed')
    if old.opcode.name == 'switch':
        require(tuple(old_index[x] for x in old.operand) == tuple(new_to_old[new_index[x]] for x in new.operand),
                'Original switch targets changed')
    elif old.opcode.flow_control.name in ('Branch', 'Cond_Branch'):
        require(old_index[old.operand] == new_to_old[new_index[new.operand]], 'Original branch target changed')
    else:
        require(old.operand_bytes == new.operand_bytes, 'Original non-branch IL operand bytes changed')


def boundary_indices(body, index_map):
    result = {instruction.offset - body.header_size: index_map[i]
              for i, instruction in enumerate(body.instructions) if i in index_map}
    result[body.code_size] = len(index_map)
    return result


def verify_target(left, right, old_row, new_row):
    old_raw, new_raw = exact_body(left, old_row), exact_body(right, new_row)
    require(digest(old_raw) == BASE_TARGET_SHA, 'Published Refresh body differs')
    before, after = (read_method_body_from_bytes(raw) for raw in (old_raw, new_raw))
    require(before.flags.value == after.flags.value and before.header_size == after.header_size and
            before.max_stack == after.max_stack and operand(before.local_var_sig_tok) == operand(after.local_var_sig_tok),
            'Refresh header, locals, init flags or maxstack changed')
    old, new = before.instructions, after.instructions
    require(len(new) == len(old) + 1 and after.code_size == before.code_size + 5,
            'Refresh must gain exactly one five-byte call')
    old_hooks = [i for i, ins in enumerate(old) if is_call(ins, 'call', HOOK_TOKEN)]
    require(len(old_hooks) == 1 and old_hooks[0] > 0 and old_hooks[0] + 1 < len(old) and
            is_call(old[old_hooks[0] - 1], 'callvirt', MONSTER_GETTER) and
            is_call(old[old_hooks[0] + 1], 'call', RESULT_CALL), 'Published monster display hook differs')
    sites = [i for i in range(len(old) - 1) if is_call(old[i], 'callvirt', BOSS_GETTER) and
             is_call(old[i + 1], 'call', RESULT_CALL)]
    require(len(sites) == 1, 'Published Boss display path differs')
    inserted = sites[0] + 1
    require(is_call(new[inserted], 'call', HOOK_TOKEN) and
            is_call(new[inserted - 1], 'callvirt', BOSS_GETTER) and
            is_call(new[inserted + 1], 'call', RESULT_CALL), 'New call is not on the Boss display path')
    require(sum(is_call(ins, 'call', HOOK_TOKEN) for ins in new) == 2, 'Unexpected display hook count')
    ref = left.net.mdtables.MemberRef.rows[(HOOK_TOKEN & 0xFFFFFF) - 1]
    owner = ref.Class.row
    scope = getattr(getattr(owner, 'ResolutionScope', None), 'row', None)
    require(str(ref.Name) == 'MonsterRewardDisplay' and str(owner.TypeNamespace) == 'HYJS.DesertV4' and
            str(owner.TypeName) == 'FieldHooks' and scope is not None and str(scope.Name) == 'SoulFlowerDesertV4' and
            ref.Signature.value == bytes.fromhex('00011282dc1282dc'), 'Existing display MemberRef differs')
    old_index, new_index = method_indices(before), method_indices(after)
    new_to_old = {i: i if i < inserted else i - 1 for i in range(len(new)) if i != inserted}
    for i, old_instruction in enumerate(old):
        new_pos = i if i < inserted else i + 1
        original_instruction_match(old_instruction, new[new_pos], old_index, new_index, new_to_old)
    require(len(before.exception_handlers) == len(after.exception_handlers), 'Refresh exception count changed')
    before_boundaries = boundary_indices(before, {i: i for i in range(len(old))})
    after_boundaries = boundary_indices(after, new_to_old)
    for old_eh, new_eh in zip(before.exception_handlers, after.exception_handlers):
        require(old_eh.exception_type == new_eh.exception_type and operand(old_eh.catch_type) == operand(new_eh.catch_type),
                'Refresh exception type changed')
        for field in ('try_start', 'try_end', 'handler_start', 'handler_end', 'filter_start'):
            old_value, new_value = getattr(old_eh, field), getattr(new_eh, field)
            if old_value < 0 or new_value < 0:
                require(old_value == new_value, 'Refresh exception filter changed')
            else:
                require(old_value in before_boundaries and new_value in after_boundaries and
                        before_boundaries[old_value] == after_boundaries[new_value],
                        'Refresh exception boundary changed: ' + field)
    return {'method': TARGET, 'methodToken': hex(TARGET_TOKEN), 'inputBodySHA256': digest(old_raw),
            'outputBodySHA256': digest(new_raw), 'reusedMemberRefToken': hex(HOOK_TOKEN),
            'inputInstructions': len(old), 'outputInstructions': len(new),
            'originalILOperandsAndBranchTargetsExact': True, 'originalExceptionBoundariesExact': True,
            'originalLocalsAndMaxStackExact': True, 'originalExceptionRegions': len(before.exception_handlers)}


def verify_main(before, after):
    require(digest(before) == BASE_MAIN_SHA, 'Published main assembly differs')
    old, new = (dnfile.dnPE(data=raw) for raw in (before, after))
    left, right = methods(old), methods(new)
    require(len(left) == len(right), 'Method inventory changed')
    unchanged = engine = 0
    target_proof = None
    for (name, rid, a), (other_name, other_rid, b) in zip(left, right):
        require((name, rid, a.Signature.value, a.struct.Flags, a.struct.ImplFlags) ==
                (other_name, other_rid, b.Signature.value, b.struct.Flags, b.struct.ImplFlags),
                'Method token or ABI changed: ' + name)
        if not a.Rva:
            require(not b.Rva, 'Unexpected new method body: ' + name)
            continue
        require(bool(b.Rva), 'Method body disappeared: ' + name)
        if name == TARGET:
            require(rid == (TARGET_TOKEN & 0xFFFFFF) and target_proof is None, 'Target token or overload changed')
            target_proof = verify_target(old, new, a, b)
        else:
            require(exact_body(old, a) == exact_body(new, b), 'Unrelated method body changed: ' + name)
            unchanged += 1
            engine += name.startswith('Battle.Engine')
    require(target_proof is not None and engine == 547, 'Target or Battle.Engine inventory differs')
    return {'changedMethods': [TARGET], 'unchangedRawMethodBodies': unchanged,
            'originalEngineMethodsExact': engine, 'originalMethodTokensAndABIExact': True,
            'target': target_proof, 'metadata': verify_metadata_tokens(before, after)}


def unique_path(paths, label):
    found = list(paths)
    require(len(found) == 1, 'Expected one local ' + label + '; found ' + str(len(found)))
    return found[0]


def run(label, args):
    result = subprocess.run(['rtk', 'proxy', *map(str, args)], cwd=ROOT, capture_output=True, text=True, timeout=180)
    require(result.returncode == 0, '%s failed (%d):\n%s\n%s' % (label, result.returncode, result.stdout, result.stderr))
    return result.stdout.strip()


def read_unique(archive, name):
    require(archive.namelist().count(name) == 1, 'Missing or duplicate APK entry: ' + name)
    return archive.read(name)


def source_snapshots():
    field_path = DESERT / 'build_field_patch.py'
    baseline_names = source_names(git_blob(GIT_DESERT / field_path.name))
    current_raw = field_path.read_bytes()
    current_names = source_names(current_raw)
    require(current_names.count('BossRewardView.cs') == 1 and
            tuple(n for n in current_names if n != 'BossRewardView.cs') == baseline_names,
            'Helper SOURCES must only add BossRewardView.cs in the published order')
    require(not set(EXTRAS) & set(current_names), 'Recovery extras must be appended once')
    baseline, candidate = {}, {}
    for name in baseline_names + EXTRAS:
        previous = git_blob(GIT_DESERT / name)
        current = (DESERT / name).read_bytes()
        if name != 'BattleRecovery.cs':
            require(current == previous, 'Unscoped helper source change: ' + name)
        baseline[name] = previous
        candidate[name] = current
    candidate['BossRewardView.cs'] = (DESERT / 'BossRewardView.cs').read_bytes()
    return baseline_names, current_names, current_raw, baseline, candidate


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, default=BASE_APK)
    parser.add_argument('--output-directory', type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument('--runtime', type=Path, default=DEFAULT_RUNTIME)
    args = parser.parse_args()
    base, out, runtime = args.base_apk.resolve(), args.output_directory.resolve(), args.runtime.resolve()
    require(out.is_relative_to(ROOT) and not out.exists(), 'Output must be a fresh project directory')
    require(file_sha(base) == BASE_APK_SHA, 'Published 1.1.8 APK SHA-256 differs')
    baseline_names, current_names, field_raw, old_sources, new_sources = source_snapshots()
    baseline_icon = git_blob(GIT_DESERT / 'assets/evacuation-npc.png')
    local_icon = DESERT / 'assets/evacuation-npc.png'
    fallback_icon = Path('/Users/lirui/LR/hyjs/tools/desert_extraction/assets/evacuation-npc.png')
    icon = local_icon if local_icon.is_file() else fallback_icon
    require(icon.is_file() and icon.read_bytes() == baseline_icon, 'Evacuation icon differs from the published source')
    dotnet = runtime / 'dotnet/dotnet'
    csc = unique_path((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'), 'SDK compiler')
    framework = unique_path((runtime / 'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'), 'net8 reference pack')
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'
    require(dotnet.is_file() and dnlib.is_file(), 'Local runtime or dnlib is missing')
    runtime_version = framework.parents[1].name
    require((runtime / 'dotnet/shared/Microsoft.NETCore.App' / runtime_version).is_dir(), 'Matching net8 runtime missing')
    modern_refs = ['/reference:' + str(path) for path in sorted(framework.glob('*.dll'))]
    require(modern_refs, 'Empty net8 reference pack')
    pinned = [Path(__file__).resolve(), HERE / 'Injector.cs', HERE / 'README.md', DESERT / 'build_field_patch.py',
              *[DESERT / name for name in current_names + EXTRAS],
              ROOT / 'tools/awake_diff_rows/build.py', ROOT / 'tools/client_relations85/verify_existing.py',
              ROOT / 'tools/client_relations85/build.py', ROOT / 'tools/client_relations85/target-pins.json',
              ROOT / 'tools/client_activity/build.py', ROOT / 'tools/client_achievements/build.py',
              ROOT / 'tools/local_asset_ready/remove_gate.py']
    if icon.is_relative_to(ROOT):
        pinned.append(icon)
    source_pins = {str(path.relative_to(ROOT)): file_sha(path) for path in pinned}
    require(source_pins[str((DESERT / 'build_field_patch.py').relative_to(ROOT))] == digest(field_raw),
            'Helper source list changed during snapshot')
    for name, raw in new_sources.items():
        require(source_pins[str((DESERT / name).relative_to(ROOT))] == digest(raw), 'Helper source changed during snapshot: ' + name)
    tool_pins = {str(path): file_sha(path) for path in (dotnet, csc, dnlib)}
    out.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.hyjs-rank-desert-', dir=out.parent) as temp:
        work = Path(temp)
        old_dir, new_dir, refs_dir = (work / name for name in ('old-src', 'new-src', 'refs'))
        for path in (old_dir, new_dir, refs_dir):
            path.mkdir()
        for name, raw in old_sources.items():
            (old_dir / name).write_bytes(raw)
        for name, raw in new_sources.items():
            (new_dir / name).write_bytes(raw)
        icon_file = work / 'evacuation-npc.png'
        icon_file.write_bytes(baseline_icon)
        with zipfile.ZipFile(base) as archive:
            old_main, old_helper = read_unique(archive, MAIN), read_unique(archive, HELPER)
            dex = read_unique(archive, 'classes2.dex')
            require(digest(old_main) == BASE_MAIN_SHA and digest(old_helper) == BASE_HELPER_SHA, 'Published managed inputs differ')
            references = []
            for name in REFS:
                path = refs_dir / name
                path.write_bytes(read_unique(archive, 'assets/bin/Data/Managed/' + name))
                references.append('/reference:' + str(path))
        old_out, stage = work / 'old', work / 'output'
        old_out.mkdir()
        stage.mkdir()

        def compile_helper(destination, directory, names):
            return run('compile helper', [dotnet, csc, '/nologo', '/target:library', '/nostdlib+', '/langversion:3',
                       '/deterministic+', '/out:' + str(destination),
                       '/resource:' + str(icon_file) + ',HYJS.DesertV4.Evacuation.png',
                       *references, *[directory / name for name in names]])

        baseline_helper = old_out / 'SoulFlowerDesertV4.dll'
        compile_helper(baseline_helper, old_dir, baseline_names + EXTRAS)
        actual_baseline = baseline_helper.read_bytes()
        if actual_baseline != old_helper:
            first = next((i for i, (a, b) in enumerate(zip(actual_baseline, old_helper)) if a != b),
                         min(len(actual_baseline), len(old_helper)))
            raise ValueError('Published helper reproduction failed: expected=%s actual=%s sizes=%d/%d firstDifference=%d' %
                             (BASE_HELPER_SHA, digest(actual_baseline), len(old_helper), len(actual_baseline), first))
        candidate_helper = stage / 'SoulFlowerDesertV4.dll'
        compile_helper(candidate_helper, new_dir, current_names + EXTRAS)
        injector = work / 'Injector.dll'
        shutil.copy2(dnlib, work / 'dnlib.dll')
        (work / 'Injector.runtimeconfig.json').write_text(json.dumps({'runtimeOptions': {'tfm': 'net8.0',
            'framework': {'name': 'Microsoft.NETCore.App', 'version': runtime_version}}}) + '\n')
        run('compile injector', [dotnet, csc, '/nologo', '/target:exe', '/nostdlib+', '/langversion:latest',
            '/out:' + str(injector), *modern_refs, '/reference:' + str(work / 'dnlib.dll'), HERE / 'Injector.cs'])
        helper_proof = run('verify helper ABI', [dotnet, injector, '--verify-helper', baseline_helper, candidate_helper])
        candidate_main = stage / 'Assembly-CSharp.dll'
        run('inject Boss display hook', [dotnet, injector, refs_dir / 'Assembly-CSharp.dll', candidate_main])
        checks = verify_main(old_main, candidate_main.read_bytes())
        require(file_sha(base) == BASE_APK_SHA, 'APK changed while building')
        for name, sha in source_pins.items():
            require(file_sha(ROOT / name) == sha, 'Source changed while building: ' + name)
        require(icon.is_file() and file_sha(icon) == digest(baseline_icon), 'Resource changed while building')
        for name, sha in tool_pins.items():
            require(file_sha(Path(name)) == sha, 'Local build tool changed: ' + name)
        payload = {MAIN: str((out / 'Assembly-CSharp.dll').relative_to(ROOT)),
                   HELPER: str((out / 'SoulFlowerDesertV4.dll').relative_to(ROOT))}
        proof = {'schema': 'hyjs-rank-desert-118-managed-v1', 'status': 'STATIC_VERIFIED_NOT_INSTALLED',
                 'baseAPKSHA256': BASE_APK_SHA, 'inputManagedSHA256': BASE_MAIN_SHA,
                 'inputDexSHA256': digest(dex), 'managedOnly': True, 'inheritedJavaByteExact': True,
                 'inheritedJavaPublicProtectedABIExact': True, 'managedAssemblyByteExact': False,
                 'allowedChangedMethods': [TARGET], 'payloadFiles': payload,
                 'replacementEntries': {MAIN: file_sha(candidate_main), HELPER: file_sha(candidate_helper)},
                 'sourcePins': source_pins, 'toolPins': tool_pins, 'baselineRevision': BASE_REV,
                 'publishedHelperSHA256': BASE_HELPER_SHA, 'baselineReproducedByteForByte': True,
                 'baselineCompilerOrder': list(baseline_names + EXTRAS), 'candidateCompilerOrder': list(current_names + EXTRAS),
                 'evacuationIconSHA256': digest(baseline_icon), 'evacuationIconPath': str(icon),
                 'existingHelperPublicABIExact': True, 'helperVerification': helper_proof,
                 'mainProof': checks, 'deviceExecuted': False}
        (stage / 'manifest.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n')
        require(not out.exists(), 'Output appeared while building')
        os.rename(stage, out)
    print(json.dumps({'status': proof['status'], 'output': str(out), 'replacementEntries': proof['replacementEntries'],
                      'originalEngineMethodsExact': checks['originalEngineMethodsExact']}))


if __name__ == '__main__':
    main()
