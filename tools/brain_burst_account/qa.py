"""Prepare a disposable local-origin overlay over a pinned formal Brain Burst APK."""
import argparse
import hashlib
import json
from pathlib import Path
import sys
import zipfile

import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_feedback58.compose import qa_main
from tools.client_arena_state.build import methods, exact_body


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--base-apk', type=Path, required=True)
parser.add_argument('--base-sha256', required=True)
parser.add_argument('--output-directory', type=Path, required=True)
args = parser.parse_args()
base, out = args.base_apk.resolve(), args.output_directory.resolve()
assert out.is_relative_to(ROOT) and not out.exists()
assert sha(base.read_bytes()) == args.base_sha256
out.mkdir(parents=True)
entry = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
with zipfile.ZipFile(base) as archive:
    before = archive.read(entry)
    dex = sha(archive.read('classes2.dex'))
after, proof = qa_main(before, base, 'http://127.0.0.1:28979', out)
assert proof['onlyFourHTTPOriginLiteralsChanged'] and proof['originalEngineMethodsUnchanged'] == 547
left, right = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
changed = [name for (name, _, a), (_, _, b) in zip(methods(left), methods(right))
           if a.Rva and exact_body(left, a) != exact_body(right, b)]
assert len(changed) == 4
(out / 'Assembly-CSharp.dll').write_bytes(after)
files = ['tools/brain_burst_account/qa.py', 'tools/client_feedback58/compose.py',
         'tools/client_remote/build.py', 'tools/client_remote/Injector.cs']
report = dict(schema='brain-burst-local-qa-v1', baseAPKSHA256=args.base_sha256,
              inputManagedSHA256=sha(before), outputManagedSHA256=sha(after), inputDexSHA256=dex,
              managedOnly=True, inheritedJavaByteExact=True, inheritedJavaPublicProtectedABIExact=True,
              payloadFiles={entry: str((out / 'Assembly-CSharp.dll').relative_to(ROOT))},
              replacementEntries={entry: sha(after)}, allowedChangedMethods=changed,
              sourcePins={name: sha((ROOT / name).read_bytes()) for name in files},
              qaOnly=True, neverPromote=True, qaOrigin='http://127.0.0.1:28979', originProof=proof)
(out / 'manifest.json').write_text(json.dumps(report, indent=2) + '\n')
print('Qualified disposable QA origin overlay; four literals changed, engine exact')
