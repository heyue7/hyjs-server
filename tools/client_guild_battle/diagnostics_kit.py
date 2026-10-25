"""De-alias PlaybackKit.Start and enable its independent original visibility opcode in the exact r7 QA APK. Never package."""
import argparse
import hashlib
import json
from pathlib import Path
import sys
import struct
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require
from tools.client_release.package_native import verify_java_class_inventory

BASE_SHA = '552c21874227abb525b43a9f89688290d8547735fb8d51e5f686a5be6f08aecf'
MAIN_SHA = 'e33d90e2e03c30c4f959151f58fc7b62ba8411600e18f9ba270b5141bfc567fa'
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, default=ROOT / 'build/guild-r7-qa/hyjs-terminal-recycle-qa.apk')
    parser.add_argument('--output', type=Path, default=ROOT / 'tools/client_guild_battle/diagnostic-kit')
    args = parser.parse_args()
    with args.base_apk.open('rb') as source:
        require(hashlib.file_digest(source, 'sha256').hexdigest() == BASE_SHA, 'Exact packaged r7 QA input required')
    out = args.output.resolve()
    require(out.is_relative_to(ROOT) and not out.exists(), 'Use a new worktree-local diagnostic overlay')
    with zipfile.ZipFile(args.base_apk) as archive:
        before = archive.read(ENTRY)
        require(sha(before) == MAIN_SHA, 'Pinned QA managed input differs')
        pe = dnfile.dnPE(data=before)
        target = next(t for t in pe.net.mdtables.TypeDef.rows if str(t.TypeName) == 'PlaybackKit' and not str(t.TypeNamespace))
        member = next(m for m in target.MethodList if str(m.row.Name) == 'Start')
        require(member.row_index == 15288, 'PlaybackKit.Start RID differs')
        body = read_method_body_from_bytes(exact_body(pe, member.row))
        require([i.opcode.name for i in body.instructions] == ['ldarg.0', 'call', 'ldc.i4.0', 'callvirt', 'ret'] and not body.exception_handlers, 'Visibility method shape differs')
        shared_offset = pe.get_offset_from_rva(member.row.Rva)
        original_body = exact_body(pe, member.row)
        instruction_offset = body.instructions[2].offset
        require(original_body[instruction_offset] == 0x16, 'Already enabled or unknown visibility byte')
        aliases = [(name, rid) for name, rid, row in methods(pe) if row.Rva == member.row.Rva]
        require(len(aliases) == 53, 'Pinned shared-body alias set differs')
        text = next(section for section in pe.sections if section.Name.rstrip(b'\0') == b'.text')
        append_at = (text.Misc_VirtualSize + 3) & ~3
        require(append_at + len(original_body) <= text.SizeOfRawData, 'No original executable section padding for independent body')
        offset = text.PointerToRawData + append_at
        new_rva = text.VirtualAddress + append_at
        require(before[offset:offset+len(original_body)] == bytes(len(original_body)), 'Executable padding is not unused zero bytes')
        enabled = bytearray(original_body)
        enabled[instruction_offset] = 0x17
        require([i for i, (a, b) in enumerate(zip(original_body, enabled)) if a != b] == [instruction_offset], 'Unexpected copied-body instruction change')
        after = bytearray(before)
        after[offset:offset+len(enabled)] = enabled
        rva_field = member.row.struct.get_field_absolute_offset('Rva')
        size_field = text.get_field_absolute_offset('Misc_VirtualSize')
        struct.pack_into('<I',after,rva_field,new_rva)
        struct.pack_into('<I',after,size_field,append_at+len(enabled))
        after = bytes(after)
        differences = [i for i, (a, b) in enumerate(zip(before,after)) if a != b]
        allowed_bytes = set(range(offset,offset+len(enabled))) | set(range(rva_field,rva_field+4)) | set(range(size_field,size_field+4))
        require(len(before) == len(after) and set(differences).issubset(allowed_bytes), 'Unexpected physical PE difference')
        require(before[shared_offset:shared_offset+len(original_body)] == after[shared_offset:shared_offset+len(original_body)], 'Original shared body changed')
        revised = dnfile.dnPE(data=after)
        changed, engine = [], 0
        for (name, rid, old), (other, rid2, new) in zip(methods(pe), methods(revised)):
            require((name, rid, old.Signature.value, old.struct.Flags, old.struct.ImplFlags) == (other, rid2, new.Signature.value, new.struct.Flags, new.struct.ImplFlags), 'ABI changed')
            if not old.Rva:
                continue
            x, y = exact_body(pe, old), exact_body(revised, new)
            if x != y:
                require(rid == 15288, 'Unapproved changed method')
                changed.append(name)
            if name.startswith('Battle.Engine'):
                require(x == y, 'Battle engine changed')
                engine += 1
        require(len(changed) == 1 and engine == 547, 'Method scope differs')
        require([i.opcode.name for i in read_method_body_from_bytes(exact_body(revised, revised.net.mdtables.MethodDef.rows[15287])).instructions] == ['ldarg.0', 'call', 'ldc.i4.1', 'callvirt', 'ret'], 'Unexpected enabled body')
        dex = sha(archive.read('classes2.dex'))
        java = verify_java_class_inventory(archive, {ENTRY: after})
        inherited = {name: sha(archive.read(name)) for name in archive.namelist() if name.startswith('assets/bin/Data/Managed/') and name != ENTRY}
    out.mkdir(parents=True)
    (out / 'Assembly-CSharp.dll').write_bytes(after)
    pins = {str(path.relative_to(ROOT)): sha(path.read_bytes()) for path in [Path(__file__).resolve(), ROOT / 'tools/awake_diff_rows/build.py', ROOT / 'tools/local_asset_ready/remove_gate.py']}
    manifest = dict(schema='guild-r7-playback-kit-diagnostic-overlay-v1', status='STATIC_DEALIASED_VISIBILITY_DIAGNOSTIC_ONLY_NOT_PACKAGED',
                    diagnosticOnly=True, qaOnly=True, neverPromote=True, baseAPK=str(args.base_apk.resolve()), baseAPKSHA256=BASE_SHA,
                    inputManagedSHA256=MAIN_SHA, outputManagedSHA256=sha(after), inputDexSHA256=dex, outputDexSHA256=dex,
                    managedOnly=True, inheritedJavaByteExact=True, inheritedJavaPublicProtectedABIExact=True,
                    sourcePins=pins, payloadFiles={ENTRY: str(out / 'Assembly-CSharp.dll')}, replacementEntries={ENTRY: sha(after)},
                    allowedChangedMethods=changed, changedMethodRID=15288, bodyDealias=dict(originalRVA=member.row.Rva, independentRVA=new_rva, bodyLength=len(enabled), originalSharedAliases=[dict(method=name,rid=rid) for name,rid in aliases], other52AliasBodiesExact=True), bytePatch=dict(fileOffset=offset+instruction_offset, before=0x16, after=0x17, independentBodyInstructionByteChanges=1, physicalFileChangedBytes=len(differences), physicalFileOffsets=differences, peTextVirtualSizeExtended=True),
                    allMetadataExceptTargetRVAByteExact=True, allABITokensSignaturesFlagsExact=True, allOtherOriginalBodiesByteExact=True, originalEngineMethodsExact=547,
                    inheritedManagedEntrySHA256=inherited, guild16HooksAndFourOriginsUntouched=True, javaClassInventory=java,
                    phoneVerified=False, sourceReference='tools/patch_client.py demo_replay_controls opcode; approved independent-body de-aliasing because the QA writer shared this body with52 other methods',
                    rollback='Replace diagnostic QA with the normal r7 QA APK after native10 capture')
    (out / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(dict(status=manifest['status'], changed=changed, offset=offset, manifestSHA256=sha((out / 'manifest.json').read_bytes()), outputManagedSHA256=sha(after))))


if __name__ == '__main__':
    main()
