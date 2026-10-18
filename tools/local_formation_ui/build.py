"""Build an opt-in isolated formation bridge/restoration overlay; no APK/device/server actions."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import struct
import subprocess
import tempfile
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
BRIDGE_ENTRY = 'assets/bin/Data/Managed/LocalFormationUI.dll'
BASE_APK = '5e8a99ae198d7cabccb5bea3d95f621ce3c34d11b2541ac7b2a17f16f052c034'
BASE_DLL = '974c930cc6b179bf16ee3ac5ee1791ea2078f02a5992b4558c9497746372fc74'
RESTORED_DLL = '90450ff8b95facef1308d77f73a4905af7e343dac550d21d6e8f898018da70ea'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def require(condition, message):
    if not condition:
        raise ValueError(message)


def run(args):
    result = subprocess.run(['rtk', 'proxy', *map(str, args)], cwd=ROOT,
                            capture_output=True, text=True)
    if result.stdout.strip():
        print(result.stdout.strip(), flush=True)
    if result.returncode:
        if result.stderr.strip():
            print(result.stderr.strip(), flush=True)
        raise RuntimeError('Local formation module build step failed')


def exact_body(pe, row):
    header = pe.get_data(row.Rva, 12)
    if header[0] & 3 == 2:
        size = 1 + (header[0] >> 2)
    else:
        flags = struct.unpack_from('<H', header)[0]
        size = ((flags >> 12) * 4) + struct.unpack_from('<I', header, 4)[0]
        if flags & 8:
            at = (size + 3) & ~3
            while True:
                section = pe.get_data(row.Rva + at, 4)
                count = int.from_bytes(section[1:4], 'little') if section[0] & 0x40 else section[1]
                require(count >= 4, 'Invalid method extra section')
                at += count
                if not section[0] & 0x80:
                    break
                at = (at + 3) & ~3
            size = at
    return pe.get_data(row.Rva, size)


def verify(before_raw, after_raw):
    before, after = dnfile.dnPE(data=before_raw), dnfile.dnPE(data=after_raw)
    require(len(before.net.mdtables.TypeDef.rows) == len(after.net.mdtables.TypeDef.rows), 'Original type count changed')
    require(len(before.net.mdtables.MethodDef.rows) == len(after.net.mdtables.MethodDef.rows), 'Original method count changed')
    unchanged = engine = targets = 0
    target_proof = None
    for index, owner in enumerate(before.net.mdtables.TypeDef.rows):
        next_owner = after.net.mdtables.TypeDef.rows[index]
        require((str(owner.TypeNamespace), str(owner.TypeName)) ==
                (str(next_owner.TypeNamespace), str(next_owner.TypeName)), 'Original type identity changed')
        for member in owner.MethodList:
            old, new = member.row, after.net.mdtables.MethodDef.rows[member.row_index - 1]
            require(old.Name.value == new.Name.value and old.Signature.value == new.Signature.value,
                    'Original ABI changed')
            if not old.Rva:
                require(not new.Rva, 'Bodyless method changed')
                continue
            a_raw, b_raw = exact_body(before, old), exact_body(after, new)
            if str(owner.TypeNamespace) == 'UI' and str(owner.TypeName) == 'ArrayPage' and str(old.Name) == 'DoOpen':
                targets += 1
                a, b = read_method_body_from_bytes(a_raw), read_method_body_from_bytes(b_raw)
                matches = []
                for n, ins in enumerate(b.instructions):
                    token = getattr(ins.operand, 'value', 0)
                    if ins.opcode.name != 'call' or token >> 24 != 0x0A:
                        continue
                    ref = after.net.mdtables.MemberRef.rows[(token & 0xFFFFFF) - 1]
                    parent = ref.Class.row
                    if str(ref.Name) == 'Ensure' and str(getattr(parent, 'TypeNamespace', '')) == 'HYJS.LocalFormation' and str(getattr(parent, 'TypeName', '')) == 'LocalFormationUI':
                        matches.append(n)
                require(len(matches) == 1, 'Expected one local Ensure call')
                n = matches[0]
                require(n >= 1 and b.instructions[n-1].opcode.name == 'ldarg.0'
                        and n + 1 == len(b.instructions) - 1 and b.instructions[-1].opcode.name == 'ret', 'Unexpected hook placement')
                left = a_raw[a.header_size:a.header_size+a.code_size]
                right = b_raw[b.header_size:b.header_size+b.code_size]
                start = b.instructions[n-1].offset - b.header_size
                end = b.instructions[n+1].offset - b.header_size
                require(end-start == 6 and right[:start] + right[end:] == left, 'Original DoOpen instructions changed')
                require(not a.exception_handlers and not b.exception_handlers, 'Unexpected UI EH region')
                require(a.header_size == b.header_size and a_raw[:4] == b_raw[:4] and
                        a_raw[8:a.header_size] == b_raw[8:b.header_size], 'Original UI flags/maxstack/locals changed')
                target_proof = dict(methodToken=hex(0x06000000 | member.row_index),
                                    originalInstructionSHA256=sha(left), revisedInstructionSHA256=sha(right),
                                    addedILByteRange=[start, end], originalInstructionsAfterRemovingSixNewBytesEqual=True,
                                    originalDoOpenBranchesAndUIInitializationPreserved=True,
                                    originalMaxStackLocalsAndExceptionHandlersPreserved=True)
                continue
            require(a_raw == b_raw, 'Other original header/IL/EH changed: ' + str(owner.TypeName) + '::' + str(old.Name))
            unchanged += 1
            if str(owner.TypeNamespace).startswith('Battle.Engine'):
                engine += 1
    require(targets == 1 and engine == 547, 'Exact single UI hook and 547 Engine bodies required')
    return dict(onlyChangedEntries=['UI.ArrayPage::DoOpen'], originalUnmodifiedMethodBodies=unchanged,
                originalEngineMethods=engine, originalEngineInstructionBytesAndExceptionRegionsUnchanged=True,
                allOtherOriginalRawMethodHeadersILAndEHByteIdentical=True, changedUIMethod=target_proof)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-proof', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--restore-managed', type=Path, default=ROOT / 'analysis/growth-first-20261016/first-double/title-overlay-r2/Assembly-CSharp.dll')
    parser.add_argument('--enable-local-formation-ui', action='store_true')
    args = parser.parse_args()
    require(args.enable_local_formation_ui, 'Explicit formation UI opt-in required')
    require(not args.output_directory.exists(), 'Use a new output directory')
    base_sha = hashlib.file_digest(args.base_apk.open('rb'), 'sha256').hexdigest()
    proof_raw = args.base_proof.read_bytes()
    proof = json.loads(proof_raw)
    require(base_sha == BASE_APK and proof['binarySHA256'] == base_sha, 'Exact qualified current 5e8a APK/proof required')
    require(proof.get('signatureVerified') and proof.get('zipalignVerified'), 'Qualified signature/alignment proof required')
    with zipfile.ZipFile(args.base_apk) as apk:
        require(len(apk.namelist()) == len(set(apk.namelist())), 'Duplicate APK entries')
        require(BRIDGE_ENTRY in apk.namelist(), 'Expected in-device r2 bridge')
        before = apk.read(ENTRY)
        restored = args.restore_managed.read_bytes()
        require(sha(restored) == RESTORED_DLL, 'Exact qualified 90450ff8 restoration required')
        require(sha(before) == BASE_DLL, 'Exact current managed DLL required')
        for entry, expected in proof['qualifiedWalletEntries'].items():
            require(sha(apk.read(entry)) == expected, 'Inherited wallet/IME/resource/title entry differs: ' + entry)
    args.output_directory.mkdir(parents=True)
    work = ROOT / '.tools/native-batch'
    dotnet = work / 'dotnet/dotnet'
    compiler = next((work / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    with tempfile.TemporaryDirectory(prefix='hyjs-formation-ui-') as folder:
        folder = Path(folder)
        base = folder / 'Assembly-CSharp.dll'
        base.write_bytes(before)
        references = [base if p.name == 'Assembly-CSharp.dll' else p for p in
                      sorted((ROOT / 'analysis/extracted/assets/bin/Data/Managed').glob('*.dll'))]
        bridge = args.output_directory / 'LocalFormationUI.dll'
        run([dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+',
             '/out:' + str(bridge), *['/reference:' + str(p) for p in references], HERE / 'LocalFormationUI.cs'])
        output = args.output_directory / 'Assembly-CSharp.dll'
        output.write_bytes(restored)
        # Inverse of the proven six-byte insertion. Every other original body is
        # exact, and the trusted whole restored DLL is pinned above.
        checks = verify(restored, before)
        checks['changedUIMethod']['restoresOriginalDoOpen'] = True
        checks['changedUIMethod']['removedILByteRange'] = checks['changedUIMethod'].pop('addedILByteRange')
    manifest = dict(schema='local-formation-bootstrap-overlay-v1', baseAPKSHA256=base_sha,
                    baseProof=str(args.base_proof), baseProofSHA256=sha(proof_raw),
                    inputManagedSHA256=sha(before), outputManagedSHA256=sha(output.read_bytes()),
                    bridgeSHA256=sha(bridge.read_bytes()), replacementEntries={ENTRY: sha(output.read_bytes()), BRIDGE_ENTRY: sha(bridge.read_bytes())},
                    **checks, originalArrayPageAllMethodsRestored=True, bootstrapUsesReflection=True,
                    bootstrapEntry='HYJS.LocalFormation.LocalFormationUI, LocalFormationUI::Ensure(UI.ArrayPage)->bool',
                    bootstrapOwner='LocalWalletSessionBridge.Update private PollFormationUI proposal',
                    bootstrapRequired=True, pureUIConditions=['own local account>1', 'real non-main GUID and current owner/camp',
                                             'mounted hero shows remove equipment/treasure/pet hint without mutation',
                                             'only naked non-main sends original21049', 'same player/session and slot checked again'],
                    nakedNonMainOnly=True, fetchOriginal20111AfterSuccess=True,
                    visibleControl=dict(page='ArrayPage', label='下阵', hook='wallet existing .5s authenticated Update reflection bootstrap; original ArrayPage untouched',
                                        localPosition=[40,230,0], size=[110,46],
                                        theme='original tabProperty UISprite atlas and UILabel font',
                                        click='UIEventListener actual BoxCollider, selected heroGuid resolved on click'),
                    authorityFailureLeavesOriginalCache=True, committedFetchFailureRetriesOnly20111=True,
                    originalArrayRefreshUsesAuthoritativeResponse=True,
                    requiredServerCompatibility='20111 must omit empty no-hero FormationItem rows for original cold loader',
                    playerStateOrBattleResultChanged=False,
                    runtimeOriginalButtonProtocolMutatesNormally=True,
                    originalResourceReadinessBridgeChanged=False, walletOrIMEChanged=False,
                    apkBuilt=False, deviceVerified=False,
                    sourcePins={str(path.relative_to(ROOT)): sha(path.read_bytes()) for path in
                                [HERE / 'LocalFormationUI.cs', HERE / 'Injector.cs', HERE / 'LocalWalletSessionBridge.proposal.cs', HERE / 'wallet-bootstrap-proposal.json', Path(__file__).resolve()]})
    (args.output_directory / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(manifest, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
