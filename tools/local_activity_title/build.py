"""Build a single qualified managed-DLL overlay for the original blank Type33 title.

Explicit opt-in only. Never packages or installs an APK. Uses the existing dnlib
toolchain and rejects any change outside the original ten-byte title IL slice.
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
BASE_APK = 'b77673db29b6caf94fbfa0939b6d2cfd2346e15df44d1c0e1a455f243ddf0a3f'
BASE_DLL = '37955adcd372e3a2f8891deafbd6668163e0bbe359886e5da02053ea08293771'
TITLE = '首充双倍'
TARGETS = [
    ('UIActivity', 'InitTabs', 0x060059A6, '975e2848364e017f2f2f93bf342d48d8c4fab2967f7c50e23e590d21a470d872',
     0xA72, b'\x7e' + struct.pack('<I', 0x0A00005F)),
    ('CopyBenefit', 'PrepareData', 0x0600597D, '3de4898459dcffd807572843650eea1e6ad28ce048b78a6c4377ab8806ca2beb',
     0x55, b'\x72' + struct.pack('<I', 0x70024921)),
]


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def require(condition, explanation):
    if not condition:
        raise ValueError(explanation)


def original_method(pe, spec):
    owner_name, method_name, token, _, _, _ = spec
    owner = next(t for t in pe.net.mdtables.TypeDef if str(t.TypeNamespace) == 'UI' and str(t.TypeName) == owner_name)
    members = [m for m in owner.MethodList if str(m.row.Name) == method_name]
    require(len(members) == 1 and (0x06000000 | members[0].row_index) == token, 'Original method identity changed')
    row = members[0].row
    body = read_method_body_from_bytes(pe.get_data(row.Rva, 65536))
    code = pe.get_data(row.Rva + body.header_size, body.code_size)
    return row, body, code


def verify(before_raw, after_raw):
    before = dnfile.dnPE(data=before_raw)
    after = dnfile.dnPE(data=after_raw)
    title_checks = []
    for spec in TARGETS:
        owner_name, method_name, token, expected_sha, load_at, original_load = spec
        call_at = load_at + 5
        target, original_body, original_code = original_method(before, spec)
        replacement, revised_body, revised_code = original_method(after, spec)
        require(sha(original_code) == expected_sha, 'Original title method changed or already patched')
        require(original_code[load_at:call_at+5] == original_load + b'\x28' + struct.pack('<I', 0x060034F3), 'Original ten-byte title slice differs')
        require(len(original_code) == len(revised_code), 'Original method size changed')
        require(revised_code[load_at] == 0x72 and revised_code[call_at:call_at+5] == b'\0'*5, 'Expected only ldstr plus five nops')
        title_token = struct.unpack_from('<I', revised_code, load_at+1)[0]
        require(title_token >> 24 == 0x70 and after.net.user_strings.get(title_token & 0xFFFFFF).value == TITLE, 'Title literal differs')
        require(original_code[:load_at] == revised_code[:load_at] and original_code[call_at+5:] == revised_code[call_at+5:], 'Other original title-method IL changed')
        original_bytes = before.get_data(target.Rva, original_body.size)
        revised_bytes = after.get_data(replacement.Rva, revised_body.size)
        require(original_body.header_size == revised_body.header_size and original_body.size == revised_body.size, 'Title body/EH size changed')
        a = original_body.header_size + load_at
        b = original_body.header_size + call_at + 5
        require(original_bytes[:a] == revised_bytes[:a] and original_bytes[b:] == revised_bytes[b:], 'Original title header/locals/other IL/raw EH changed')
        title_checks.append(dict(method=owner_name+'::'+method_name, methodToken=hex(token),
                                 inputMethodILSHA256=sha(original_code), outputMethodILSHA256=sha(revised_code),
                                 codeSize=len(original_code), changedILByteWhitelist=[[load_at, call_at+5]],
                                 rawHeaderOtherILAndEHByteIdentical=True))
    unchanged = engine = 0
    require(len(before.net.mdtables.TypeDef.rows) == len(after.net.mdtables.TypeDef.rows), 'Type count changed')
    require(len(before.net.mdtables.MethodDef.rows) == len(after.net.mdtables.MethodDef.rows), 'Method count changed')
    for index, owner in enumerate(before.net.mdtables.TypeDef.rows):
        new_owner = after.net.mdtables.TypeDef.rows[index]
        require((str(owner.TypeNamespace), str(owner.TypeName)) == (str(new_owner.TypeNamespace), str(new_owner.TypeName)), 'Type identity changed')
        for member in owner.MethodList:
            old = member.row
            new = after.net.mdtables.MethodDef.rows[member.row_index - 1]
            require(str(old.Name) == str(new.Name) and old.Signature.value == new.Signature.value, 'Original method ABI changed')
            if not old.Rva:
                require(not new.Rva, 'Original bodyless method gained code')
                continue
            if (0x06000000 | member.row_index) in {spec[2] for spec in TARGETS}:
                continue
            # Read the method's exact header/code length first. Avoid copying
            # the entire remaining PE section for each of 26,000 methods.
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
            left = exact_body(before, old)
            right = exact_body(after, new)
            require(left == right,
                    'Other original raw method header/IL/EH changed: ' + str(owner.TypeName) + '::' + str(old.Name))
            unchanged += 1
            if str(owner.TypeNamespace).startswith('Battle.Engine'):
                engine += 1
    require(engine == 547, 'All 547 inherited Engine methods required')
    return dict(originalUnmodifiedMethodBodies=unchanged, originalEngineMethods=engine,
                originalEngineInstructionBytesAndExceptionRegionsUnchanged=True,
                allOtherOriginalRawMethodHeadersILAndEHByteIdentical=True,
                originalActivityHeaderLocalsBranchesAndRawEHByteIdentical=True,
                titleMethods=title_checks,
                titleLiteral=TITLE, localizationBypassedOnlyForTitle=True,
                replacementLoad='ldstr 首充双倍',
                originalCall='TableTextBlo.GetText', replacementCall='5 one-byte nop instructions',
                onlyChangedEntries=['UIActivity::InitTabs', 'CopyBenefit::PrepareData'])


def run(arguments):
    subprocess.run(['rtk', 'proxy', *map(str, arguments)], cwd=ROOT, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-proof', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--enable-local-activity-title', action='store_true')
    args = parser.parse_args()
    require(args.enable_local_activity_title, 'Explicit title-patch opt-in required')
    require(not args.output_directory.exists(), 'Use a new output directory')
    base_sha = hashlib.file_digest(args.base_apk.open('rb'), 'sha256').hexdigest()
    proof_raw = args.base_proof.read_bytes()
    proof = json.loads(proof_raw)
    require(base_sha == BASE_APK and proof['binarySHA256'] == base_sha, 'Exact qualified b776 APK/proof required')
    require(proof.get('signatureVerified') and proof.get('zipalignVerified'), 'Qualified signature/alignment proof required')
    with zipfile.ZipFile(args.base_apk) as apk:
        require(len(apk.namelist()) == len(set(apk.namelist())), 'Duplicate APK entries')
        before = apk.read(ENTRY)
        require(sha(before) == BASE_DLL, 'Qualified managed DLL differs')
        for entry, expected in proof['qualifiedWalletEntries'].items():
            require(sha(apk.read(entry)) == expected, 'Inherited wallet/IME/resource entry differs: ' + entry)
    original_pe = dnfile.dnPE(data=before)
    for spec in TARGETS:
        _, _, code = original_method(original_pe, spec)
        require(sha(code) == spec[3], 'Original title method differs or already patched')
    args.output_directory.mkdir(parents=True)
    work = ROOT / '.tools/native-batch'
    dotnet = work / 'dotnet/dotnet'
    dnlib = work / 'injector/bin/Debug/net8.0/dnlib.dll'
    with tempfile.TemporaryDirectory(prefix='hyjs-activity-title-') as folder:
        folder = Path(folder)
        input_dll = folder / 'Assembly-CSharp.dll'
        input_dll.write_bytes(before)
        shutil.copy2(ROOT / 'tools/local_activity_title/Injector.cs', folder / 'Program.cs')
        project = folder / 'ActivityTitleInjector.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                           '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib">'
                           '<HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run([dotnet, 'build', project, '--nologo', '--verbosity', 'quiet'])
        output_dll = args.output_directory / 'Assembly-CSharp.dll'
        run([dotnet, folder / 'bin/Debug/net8.0/ActivityTitleInjector.dll', input_dll, output_dll])
        checks = verify(before, output_dll.read_bytes())
    manifest = dict(schema='local-activity-title-overlay-v1', baseAPKSHA256=base_sha,
                    baseProof=str(args.base_proof), baseProofSHA256=sha(proof_raw),
                    inputManagedSHA256=sha(before), outputManagedSHA256=sha(output_dll.read_bytes()),
                    replacementEntries={ENTRY: sha(output_dll.read_bytes())}, **checks,
                    noNewRuntimeAssembly=True, noResourceEntriesChanged=True,
                    inheritedNativeCombatDiagnosticLogging=True,
                    playerStateOrBattleResultChanged=False, apkBuilt=False, deviceVerified=False,
                    sourcePins={str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [
                        ROOT / 'tools/local_activity_title/Injector.cs', Path(__file__).resolve()]})
    (args.output_directory / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(manifest, ensure_ascii=False))


if __name__ == '__main__':
    main()
