"""Verify public Wanted source/record qualification pins without running an engine."""
from pathlib import Path
import base64
import hashlib
import json

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
proof_bytes = (HERE / "native-proof.json").read_bytes()
proof = json.loads(proof_bytes)
proof_hash = hashlib.sha256(proof_bytes).hexdigest()
assert proof_hash == "87598dfe7341455b589f327139aa0e3930f53fd48997e6aaebb6f6aa7b93db66"
for name, key in (("Wanted86Caller.cs", "sourceCallerSHA256"), ("Wanted86Host.cs", "sourceHostSHA256")):
    assert hashlib.sha256((HERE / name).read_bytes()).hexdigest() == proof[key]
fixture = json.loads((ROOT / "hyjs-server/service/testdata/p12_wanted_live_controls86.json").read_text())
assert fixture["proof"] == proof
for case in fixture["cases"]:
    name = {0: "public-normal", 2: "public-deadly"}[case["nativeCaseIndex"]]
    assert hashlib.sha256(base64.b64decode(case["recordBase64"], validate=True)).hexdigest() == proof["nativeRecords"][name]
source = (ROOT / "hyjs-server/service/p12WantedLiveRecordedService.go").read_text()
assert 'p12WantedLiveRecordedProof = "' + proof_hash + '"' in source
print("Wanted source, proof and both public native records verified")
