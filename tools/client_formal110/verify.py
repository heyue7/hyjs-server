"""Independently check the delivered 1.1.0 candidate without installing it."""
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_formal110 import build
from tools.client_release.package_native import verify_final_resource_pins
from tools.local_asset_ready.remove_gate import exact_body


def check(condition, message):
    if not condition:
        raise ValueError(message)


def main():
    destination = ROOT / 'analysis/formal110/independent-verification.json'
    check(not destination.exists(), 'Independent evidence already exists')
    proof = json.loads(build.RECORD.read_text())
    check(build.file_sha(build.BASE) == build.BASE_SHA, 'Base APK changed')
    check(build.file_sha(build.OUTPUT) == proof['binarySHA256'], 'Candidate APK differs from build record')
    check(build.source_pins() == proof['sourcePins'], 'Pinned build source changed after packaging')
    env = dict(os.environ, JAVA_HOME=str(build.JAVA),
               PATH=str(build.JAVA / 'bin') + os.pathsep + os.environ.get('PATH', ''))
    target = build.target_origin(env)
    check(hashlib.sha256(target.encode()).hexdigest() == proof['targetOriginSHA256'],
          'Current SSH target differs from build target')
    expected_identity = dict(build.build_identity(build.BASE, env),
                             versionCode=build.BUILD_CODE, versionName=build.VERSION_NAME)
    check(build.build_identity(build.OUTPUT, env) == expected_identity, 'APK identity differs')
    check(build.signer(build.OUTPUT, env) == build.signer(build.BASE, env) == build.EXPECTED_CERT,
          'APK signing identity differs')
    build.run('independent zip alignment', [build.SDK / 'zipalign', '-c', '-p', '4', build.OUTPUT], env=env)
    with zipfile.ZipFile(build.BASE) as original, zipfile.ZipFile(build.OUTPUT) as candidate:
        verify_final_resource_pins(original, {})
        verify_final_resource_pins(candidate, {})
        old = json.loads(original.read(build.WALLET))['origin']
        check(json.loads(candidate.read(build.WALLET)) == {'origin': target}, 'Candidate wallet origin differs')
        original_dex = sorted(n for n in original.namelist() if re.fullmatch(r'classes(?:[2-9][0-9]*)?\.dex', n))
        candidate_dex = sorted(n for n in candidate.namelist() if re.fullmatch(r'classes(?:[2-9][0-9]*)?\.dex', n))
        check(original_dex == candidate_dex and original_dex, 'DEX inventory differs')
        for name in original_dex:
            check(hashlib.sha256(original.read(name)).digest() == hashlib.sha256(candidate.read(name)).digest(),
                  'A DEX file changed: ' + name)
        pe = dnfile.dnPE(data=candidate.read(build.DLL))
        expected = {name: target + '/sao/' if i == 0 else target for i, name in enumerate(build.TARGETS)}
        found = {}
        for name, _, row in methods(pe):
            if not row.Rva:
                continue
            strings = []
            for instruction in read_method_body_from_bytes(exact_body(pe, row)).instructions:
                if instruction.opcode.name == 'ldstr':
                    value = pe.net.user_strings.get(instruction.operand.value & 0xffffff).value
                    check(old not in value, 'Inherited address remains in executable IL')
                    strings.append(value)
            if name in expected:
                check(strings.count(expected[name]) == 1, 'Target address missing from controlled method: ' + name)
                found[name] = True
        check(set(found) == set(expected), 'Controlled method inventory differs')
    result = {'status': 'PASS_STATIC_FORMAL110_NOT_INSTALLED',
              'candidateSHA256': proof['binarySHA256'], 'baseSHA256': build.BASE_SHA,
              'packageVersionLauncherVerified': True, 'targetMatchesSSHConfigAndWallet': True,
              'fourExecutableOriginsVerified': True, 'oldOriginAbsentFromExecutableIL': True,
              'allDexFilesByteIdentical': True, 'dexFileCount': len(original_dex),
              'compatibilityResourcePinsVerifiedOnBaseAndCandidate': True,
              'signatureAndAlignmentVerified': True, 'buildSourcePinsMatch': True,
              'verifierSHA256': build.file_sha(Path(__file__)), 'phoneTested': False,
              'serverContacted': False}
    destination.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(f'Formal 1.1.0 independent verification failed: {type(error).__name__}: {error}', file=sys.stderr)
        raise SystemExit(1)
