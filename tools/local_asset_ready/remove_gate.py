"""Explicit opt-in removal of the two inherited Awake resource entry guards.

Builds a single managed overlay from the exact qualified APK. Never packages,
installs, executes the game or changes global resource/download/save flags.
"""
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
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
BASE_APK = '28921b0d84fbc07b3e0f0d1105418e9af7fd8e92b6cbaaf9150ed86878c6b73f'
BASE_DLL = '90450ff8b95facef1308d77f73a4905af7e343dac550d21d6e8f898018da70ea'
TARGETS = [
    ('Room.RCUINormalNPC', 'OpenPage', 0x0600458F, 0x15,
     '5137a9f51747a1ddb5769a4e4dce427a94a62add3ec19780c95c035e3b6a5f26', 0x0A0020AA),
    ('AwakeDiffItem', 'OnChallengeClick', 0x06004C8C, 0,
     '47dd5ecb1de2d415f696938caa9cdaee603ca67eac72b336bbb345a70d342f19', 0x0A0020AB),
]


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def require(ok, message):
    if not ok:
        raise ValueError(message)


def exact_body(pe, method):
    header = pe.get_data(method.Rva, 12)
    if header[0] & 3 == 2:
        length = 1 + (header[0] >> 2)
    else:
        flags = struct.unpack_from('<H', header)[0]
        length = ((flags >> 12) * 4) + struct.unpack_from('<I', header, 4)[0]
        if flags & 8:
            at = (length + 3) & ~3
            while True:
                section = pe.get_data(method.Rva + at, 4)
                size = int.from_bytes(section[1:4], 'little') if section[0] & 0x40 else section[1]
                require(size >= 4, 'Malformed method extra section')
                at += size
                if not section[0] & 0x80:
                    break
                at = (at + 3) & ~3
            length = at
    return pe.get_data(method.Rva, length)


def target(pe, spec):
    owner, name, token, at, il_sha, helper = spec
    row = pe.net.mdtables.MethodDef.rows[(token & 0xffffff) - 1]
    t = next(t for t in pe.net.mdtables.TypeDef
             if '.'.join(filter(None, [str(t.TypeNamespace), str(t.TypeName)])) == owner)
    require(any(m.row is row for m in t.MethodList) and str(row.Name) == name, 'Original method identity differs')
    raw = exact_body(pe, row)
    body = read_method_body_from_bytes(raw)
    code = raw[body.header_size:body.header_size+body.code_size]
    return row, raw, body, code


def verify(before_raw, after_raw):
    require(sha(before_raw) == BASE_DLL, 'Exact qualified original managed DLL required')
    require(len(before_raw) == len(after_raw), 'PE size changed')
    before, after = dnfile.dnPE(data=before_raw), dnfile.dnPE(data=after_raw)
    permitted = set()
    methods = []
    for spec in TARGETS:
        owner, name, token, at, il_sha, helper = spec
        row, old, body, code = target(before, spec)
        new_row, new, new_body, revised = target(after, spec)
        require(sha(code) == il_sha, 'Unknown or already removed original gate')
        expected = (b'\x28'+struct.pack('<I', 0x06003C75) +
                    b'\x6f'+struct.pack('<I', 0x06003C6A) +
                    b'\x02\x28'+struct.pack('<I', helper))
        require(code[at:at+16] == expected and code[at+16] == 0x3a,
                'Inherited original getter / scoped helper / brtrue differs')
        replacement = b'\x17' + b'\0'*15
        require(revised == code[:at] + replacement + code[at+16:], 'Changed IL exceeds gate expression')
        a, b = body.header_size + at, body.header_size + at + 16
        require(new == old[:a] + replacement + old[b:], 'Raw header/locals/branch/EH changed')
        require(row.Rva == new_row.Rva and body.size == new_body.size, 'Method layout changed')
        file_at = before.get_offset_from_rva(row.Rva) + a
        permitted.update(range(file_at, file_at+16))
        methods.append(dict(method=owner+'::'+name, token=hex(token),
                            inputMethodILSHA256=sha(code), outputMethodILSHA256=sha(revised),
                            changedILByteWhitelist=[[at, at+16]],
                            changedFileByteWhitelist=[[file_at, file_at+16]],
                            beforeHex=expected.hex(), afterHex=replacement.hex(),
                            codeSize=body.code_size, exceptionRegions=len(body.exception_handlers),
                            originalHeaderLocalsBranchesAndRawEHByteIdentical=True))
    changes = {i for i, (a, b) in enumerate(zip(before_raw, after_raw)) if a != b}
    require(changes and changes <= permitted, 'Other original PE/metadata byte changed')
    unchanged = engine = 0
    target_tokens = {s[2] for s in TARGETS}
    for owner in before.net.mdtables.TypeDef:
        for member in owner.MethodList:
            old = member.row
            new = after.net.mdtables.MethodDef.rows[member.row_index-1]
            require(str(old.Name) == str(new.Name) and old.Signature.value == new.Signature.value,
                    'Method ABI changed')
            if not old.Rva:
                require(not new.Rva, 'Bodyless method gained code')
                continue
            if (0x06000000 | member.row_index) in target_tokens:
                continue
            require(exact_body(before, old) == exact_body(after, new),
                    'Other raw method header/IL/EH changed: '+str(owner.TypeName)+'::'+str(old.Name))
            unchanged += 1
            if str(owner.TypeNamespace).startswith('Battle.Engine'):
                engine += 1
    require(engine == 547, 'All 547 inherited engine methods required')
    return dict(originalUnmodifiedMethodBodies=unchanged, originalEngineMethods547=engine,
                originalEngineMethods=engine,
                originalEngineInstructionBytesAndExceptionRegionsUnchanged=True,
                allOtherOriginalRawMethodHeadersILAndEHByteIdentical=True,
                allOtherInputManagedBytesIdentical=True,
                actualChangedFileBytes=len(changes), permittedFileBytes=len(permitted),
                onlyChangedEntries=[s[0]+'::'+s[1] for s in TARGETS], methods=methods)


def run(args):
    subprocess.run(['rtk', 'proxy', *map(str, args)], cwd=ROOT, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-proof', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--remove-awake-resource-gates', action='store_true')
    args = parser.parse_args()
    require(args.remove_awake_resource_gates, 'Explicit resource-gate removal opt-in required')
    require(not args.output_directory.exists(), 'Use a fresh overlay directory')
    base_sha = hashlib.file_digest(args.base_apk.open('rb'), 'sha256').hexdigest()
    proof_raw = args.base_proof.read_bytes()
    proof = json.loads(proof_raw)
    require(base_sha == BASE_APK and proof.get('binarySHA256') == base_sha, 'Exact qualified 28921b APK/proof required')
    require(proof.get('signatureVerified') and proof.get('zipalignVerified'), 'Qualified signature/alignment proof required')
    with zipfile.ZipFile(args.base_apk) as apk:
        require(len(apk.namelist()) == len(set(apk.namelist())), 'Duplicate ZIP entries')
        original = apk.read(ENTRY)
        require(sha(original) == BASE_DLL, 'Qualified input managed DLL changed')
        for entry, expected in proof['qualifiedWalletEntries'].items():
            require(sha(apk.read(entry)) == expected, 'Inherited qualified entry changed: '+entry)
    args.output_directory.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'
    with tempfile.TemporaryDirectory(prefix='hyjs-remove-awake-gate-') as temporary:
        folder = Path(temporary)
        incoming = folder / 'Assembly-CSharp.dll'
        incoming.write_bytes(original)
        shutil.copy2(Path(__file__).with_name('RemoveGateInjector.cs'), folder / 'Program.cs')
        project = folder / 'RemoveGate.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                           '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib">'
                           '<HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run([dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        output = args.output_directory / 'Assembly-CSharp.dll'
        run([dotnet, folder / 'bin/Debug/net8.0/RemoveGate.dll', incoming, output])
        checks = verify(original, output.read_bytes())
    require(hashlib.file_digest(args.base_apk.open('rb'), 'sha256').hexdigest() == base_sha,
            'Input APK changed during overlay build')
    manifest = dict(schema='local-awake-resource-gate-removal-overlay-v1', **checks,
                    baseAPKSHA256=base_sha, baseProof=str(args.base_proof),
                    baseProofSHA256=sha(proof_raw), inputManagedSHA256=sha(original),
                    outputManagedSHA256=sha(output.read_bytes()),
                    replacementEntries={ENTRY: sha(output.read_bytes())},
                    noNewRuntimeAssembly=True, runtimeResourceBridgeCalledByPatchedGates=False,
                    globalFullGetterAndPersistentFlagsUnchanged=True,
                    introducedQualityPartyResourceOrCopyIDChecks=False,
                    originalPostGateLevelEligibilitySelectionUnchanged=True,
                    inheritedWalletIMEFormationAndOtherManagedCodeByteIdentical=True,
                    playerStateOrBattleResultChanged=False, apkBuilt=False, deviceVerified=False,
                    sourcePins={str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [
                        Path(__file__).resolve(), Path(__file__).with_name('RemoveGateInjector.cs').resolve()]})
    (args.output_directory / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps(manifest, ensure_ascii=False))


if __name__ == '__main__':
    main()
