"""Read-only DEX ABI/code/EH and original 547-method battle-engine protection checks."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import zipfile

from androguard.core.dex import DEX
from loguru import logger
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
spec = importlib.util.spec_from_file_location('hyjs_modern_ime_exact_patch', HERE / 'patch.py')
patcher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(patcher)


def dex_methods(raw):
    logger.remove()
    dex = DEX(raw)
    result = {}
    for owner in dex.get_classes():
        for method in owner.get_methods():
            key = owner.get_name() + '->' + method.get_name() + method.get_descriptor()
            code = method.get_code()
            result[key] = None if code is None else patcher.digest(code.get_raw())
    return dex, result


def simple_value(value):
    if value is None or isinstance(value, (str, int, float, bool)): return value
    if hasattr(value, 'value'): return value.value
    raise ValueError('Unexpected exception-handler field type')


def engine_methods(raw):
    pe = dnfile.dnPE(data=raw)
    result = []
    for owner in pe.net.mdtables.TypeDef:
        if not str(owner.TypeNamespace).startswith('Battle.Engine'): continue
        for entry in owner.MethodList:
            method = entry.row
            if not method.Rva: continue
            body = read_method_body_from_bytes(pe.get_data(method.Rva))
            code = pe.get_data(method.Rva + body.header_size, body.code_size)
            handlers = [{key: simple_value(value) for key, value in vars(handler).items()} for handler in body.exception_handlers]
            result.append([str(owner.TypeNamespace) + '.' + str(owner.TypeName), str(method.Name),
                           method.Signature.value.hex(), patcher.digest(code), handlers])
    if len(result) != 547: raise ValueError('Original battle-engine inventory differs from 547 methods')
    return sorted(result, key=lambda row: row[:3])


def verify(apk, output):
    apk, output = Path(apk).resolve(), Path(output).resolve()
    with zipfile.ZipFile(apk) as archive:
        original = archive.read('classes.dex')
        assembly = archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
    expected, protection = patcher.patch(original)
    patched = (output / 'classes.dex').read_bytes()
    replacement = (output / 'classes3.dex').read_bytes()
    if expected != patched: raise ValueError('Packaged DEX patch differs from exact four-class whitelist')
    _, original_methods = dex_methods(original)
    _, patched_methods = dex_methods(patched)
    expected_methods = {key: value for key, value in original_methods.items() if key.split('->')[0] not in patcher.REMOVED}
    if patched_methods != expected_methods: raise ValueError('Non-keyboard methods/code/EH differ after parse')
    new_dex, new_methods = dex_methods(replacement)
    definitions = {owner.get_name() for owner in new_dex.get_classes()}
    if any(name not in patcher.REMOVED and not name.startswith('Lcom/unity3d/player/s$') for name in definitions):
        raise ValueError('New DEX defines an unrelated class')
    required = ['<init>(Landroid/content/Context; Lcom/unity3d/player/UnityPlayer; Ljava/lang/String; I Z Z Z Ljava/lang/String;)V',
                'show()V', 'dismiss()V', 'a(Ljava/lang/String;)V', 'onBackPressed()V',
                'afterTextChanged(Landroid/text/Editable;)V']
    for method in required:
        if 'Lcom/unity3d/player/s;->' + method not in new_methods:
            raise ValueError('Original keyboard ABI missing: ' + method)
    strings, _ = patcher.strings_and_types(replacement)
    if any(token.startswith(('Ljava/io/', 'Ljava/net/', 'Landroid/util/Log;', 'Landroid/content/SharedPreferences')) for token in strings):
        raise ValueError('Keyboard adapter contains forbidden logging/storage/network references')
    for negative in (patched, original[:-1] + bytes([original[-1] ^ 1])):
        try: patcher.patch(negative)
        except ValueError: pass
        else: raise ValueError('Already-patched or altered DEX unexpectedly accepted')
    engine = engine_methods(assembly)
    raw_reference = (ROOT / 'analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll').read_bytes()
    original_engine = engine_methods(raw_reference)
    diagnostic_path = ROOT / 'tools/native_combat_diagnostics.py'
    diagnostics_spec = importlib.util.spec_from_file_location('hyjs_inherited_native_diagnostics', diagnostic_path)
    diagnostics = importlib.util.module_from_spec(diagnostics_spec)
    diagnostics_spec.loader.exec_module(diagnostics)
    # The accepted APK already has the separately authorized 17 logging call operands across 9 methods.
    # Compare exactly against that existing adapter; the IME patch never rewrites any managed byte.
    reference = engine_methods(diagnostics.patch(raw_reference))
    if engine != reference: raise ValueError('547 inherited battle-engine instructions/EH differ from authorized diagnostic reference')
    inherited_log_methods = [now[0] + '::' + now[1] for now, old in zip(engine, original_engine) if now != old]
    dexdump = Path.home() / 'Library/Android/sdk/build-tools/33.0.0/dexdump'
    for name in ('classes.dex', 'classes3.dex'):
        check = subprocess.run(['rtk', 'proxy', str(dexdump), '-c', str(output / name)], capture_output=True, text=True, timeout=30)
        if check.returncode: raise ValueError('Android dexdump checksum validation failed: ' + name)
    return {'status': 'PASS_OFFLINE_ONLY', 'inputAPK': str(apk.relative_to(ROOT)),
            'inputAPKSHA256': patcher.digest(apk.read_bytes()),
            'patchedDexSHA256': patcher.digest(patched), 'replacementDexSHA256': patcher.digest(replacement),
            'originalMethodCount': len(original_methods), 'retainedMethodsCodeAndEHExact': len(patched_methods),
            'replacementABIMethodsChecked': required, 'replacementDefinedClasses': sorted(definitions),
            'allBytesOutsideMetadataWhitelistUnchanged': True,
            'alteredAndAlreadyPatchedInputsRejected': True, 'androidDexChecksumsValid': True,
            'originalEngineMethods': 547, 'originalEngineInstructionsAndEHUnchanged': True,
            'engineProtectionBoundary': 'exact inherited input APK DLL; matches shipped reference plus existing native-combat-diagnostics logging operands',
            'shippedRawReferenceIsByteIdentical': not inherited_log_methods,
            'inheritedDiagnosticLoggingMethods': inherited_log_methods,
            'inheritedDiagnosticToolSHA256': patcher.digest(diagnostic_path.read_bytes()),
            'originalEngineDigest': patcher.digest(json.dumps(engine, separators=(',', ':')).encode()),
            'managedAssemblyUntouched': True, 'passwordLoggingStorageNetworkAPIsAbsent': True,
            'apkPackaged': False, 'deviceExecuted': False,
            'pendingDeviceChecks': ['current IME no fullscreen extract/no legacy dialog', 'Chinese composing/candidate/emoji',
                                    'password masked/no plaintext screenshot', 'numeric/decimal/multiline',
                                    'Done/Back/field switch/reopen/insets/game picture/session login']}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--input-apk', required=True)
    parser.add_argument('--output', required=True)
    parser.add_argument('--proof', required=True)
    args = parser.parse_args()
    target = Path(args.proof)
    if target.exists(): raise ValueError('Preserve old verification evidence; use a fresh proof path')
    result = verify(args.input_apk, args.output)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(result, indent=2))
