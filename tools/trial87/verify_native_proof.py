"""Verify original formation qualification source and all public record pins."""
from pathlib import Path
import base64,hashlib,json
HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[1]
raw=(HERE/'native-proof.json').read_bytes();proof=json.loads(raw)
sha=lambda b:hashlib.sha256(b).hexdigest()
assert sha(raw)=='0c6e61f38312234462b5391a855956024b430f0f840dca5c788faddfb74f6637'
for name,key in [('Trial87Caller.cs','callerSourceSHA256'),('Trial87Host.cs','hostSourceSHA256')]:assert sha((HERE/name).read_bytes())==proof[key]
fixture=json.loads((ROOT/'hyjs-server/service/testdata/p7_formation_recorded87.json').read_text());assert fixture['proof']==proof
for case in fixture['cases']:
 assert sha(base64.b64decode(case['recordBase64'],validate=True))==case['recordSHA256']==proof['records'][case['name']]
 assert sha(bytes.fromhex(case['startHex']))==case['startSHA256']
source=(ROOT/'hyjs-server/service/p7FormationRecordedService.go').read_text();assert 'p7FormationRecordedProof = "'+sha(raw)+'"' in source
print('Formation caller/host source and all three native complete-record pins verified')
