#!/usr/bin/env python3
"""Renew unchanged P6 scopes only after all strict native groups pass."""
import argparse, copy, hashlib, json, pathlib, re, subprocess
ROOT = pathlib.Path(__file__).resolve().parents[2]
SOURCE = pathlib.Path("/Users/lirui/LR/hyjs/analysis")
OUT = ROOT / "analysis/shop76-completion-20261027"
def digest(raw): return hashlib.sha256(raw).hexdigest()
def raw_file(path):
    p = ROOT / path
    if not p.exists() and path.startswith("analysis/"): p = SOURCE / path.removeprefix("analysis/")
    return p.read_bytes()
def dump(obj): return (json.dumps(obj, ensure_ascii=False, indent=2)+"\n").encode()
families = [
    ("arena047", "v122", "p6ArenaSettlement.go", "P6ArenaNativeProof", "P6ArenaCertificateFile", ["ArenaV122"]),
    ("starter047", "v118", "p6StarterSettlement.go", "P6StarterNativeProof", "P6StarterCertificateFile", ["StarterFriendV118"]),
    ("linear-physical", "v117", "p6ScopedSettlement.go", "P6LinearPhysicalNativeProof", "P6LinearPhysicalCertificateFile", ["LinearPhysicalV111", "LinearFriendV113"]),
    ("owned-partner", "v126", "p6OwnedPartnerSettlement.go", "P6OwnedPartnerNativeProof", "P6OwnedPartnerCertificateFile", ["OwnedPartnerV124"]),
]
suffixes = ["OriginalAutomaticCoreAndDynamic", "OriginalAutomaticExports", "OriginalAutomaticFinalProtocolBytes"]
required = {"TestP6Native"+group+suffix for *_, groups in families for group in groups for suffix in suffixes}
required |= {"TestOriginalLifestealConvertsPercentBeforeDamage", "TestOriginalLifestealStrictOriginalEndRegression", "TestOriginalLifestealFractionResidueCompleteOriginalEnd"}
parser=argparse.ArgumentParser(); parser.add_argument("--apply", action="store_true"); parser.add_argument("--history-compatibility-reviewed", action="store_true"); args=parser.parse_args()
passes = {}
for name in ["full-tests.jsonl", "remaining-tests.jsonl"]:
    p=OUT/name
    if not p.exists(): continue
    for line in p.read_text().splitlines():
        try: event=json.loads(line)
        except json.JSONDecodeError: continue
        test=event.get("Test", "")
        if test in required and event.get("Action") == "pass": passes[test]={"source": str(p.relative_to(ROOT)), "event": event}
missing=sorted(required-passes.keys())
print(json.dumps({"required":len(required),"passed":len(passes),"missing":missing},indent=2))
if missing or not args.apply: raise SystemExit(1 if missing else 0)
if not args.history_compatibility_reviewed: raise SystemExit("Pointer rotation blocked: exact parent-proof historical receipt compatibility must be implemented and reviewed first")
# Freeze extracted pass events independently of still-growing runner streams.
evidence={"candidate": subprocess.check_output(["git","rev-parse","HEAD"],cwd=ROOT,text=True).strip(), "date":"2026-10-27", "kind":"current Go exact comparison against immutable original native recordings", "originalRecaptured":False, "nativeGoldenChanged":False, "events": [passes[k] for k in sorted(passes)]}
evidence_path="analysis/shop76-completion-20261027/certificate-revalidation-evidence.json"
evidence_raw=dump(evidence)
prepared=[]
for family, version, constant, proof_key, file_key, groups in families:
    old_name=f"{family}-capability-certificate-{version}.json"
    new_name=f"{family}-capability-certificate-revalidated-20261027.json"
    parent=raw_file("analysis/p6/"+old_name); doc=json.loads(parent)
    cp=ROOT/"hyjs-server/constUtil"/constant; text=cp.read_text()
    old_proof=re.search(r"\b"+proof_key+r'\s*=\s*"([^"\n]+)"',text).group(1)
    assert digest(parent)==old_proof, (family,"unexpected current pointer")
    proof_pattern = r"(\b"+proof_key+r'\s*=\s*")[^"\n]+(")'
    blank=re.sub(proof_pattern, lambda m: m[1]+m[2], text).replace(old_name,new_name)
    doc["scopePolicy"]["constantSourceNormalizedSHA256"]=digest(blank.encode())
    doc["scopePolicy"]["sourceSHA256"]=digest(raw_file(doc["scopePolicy"]["source"]))
    for entry in doc["runtimeSources"]+doc.get("supportingPolicies",[]): entry["sha256"]=digest(raw_file(entry["path"]))
    for entry in doc["originalNativeEvidence"]: assert digest(raw_file(entry["path"]))==entry["sha256"], (family, entry["path"])
    # Keep all original scope, mechanism, authority, original evidence and historical validation fields intact.
    doc["revalidation"]={"date":"2026-10-27", "candidate":evidence["candidate"], "parentCertificate":{"path":"analysis/p6/"+old_name,"sha256":digest(parent)}, "evidence":{"path":evidence_path,"sha256":digest(evidence_raw)}, "strictNativeGroups":["TestP6Native"+g for g in groups], "scopeChanged":False, "authorityChanged":False, "originalEvidenceChanged":False, "basis":"Exact original callback, actor properties, RNG, lifecycle, ordered exports/collections, final protocol bytes and terminal HP comparisons on current implementation; original assets retained. Lifesteal fraction correction has no producer within the closed family. Optional live record/control inputs remain absent. Earlier shared-source drift is covered by strict original comparisons, without attributing all drift to lifesteal.", "historicalReceiptPolicy":"Exact parent proof retained under original mode/engine/client/config restrictions; persisted terminal and receipt integrity validation unchanged.", "review":"analysis/shop76-completion-20261027/certificate-review.md"}
    new_raw=dump(doc); new_proof=digest(new_raw)
    new_text=re.sub(proof_pattern, lambda m: m[1]+new_proof+m[2], text).replace(old_name,new_name)
    assert digest(re.sub(proof_pattern, lambda m: m[1]+m[2], new_text).encode())==doc["scopePolicy"]["constantSourceNormalizedSHA256"]
    prepared.append((ROOT/"analysis/p6"/new_name,new_raw,cp,new_text.encode()))
# All preconditions verified before first pointer mutation.
(OUT/"certificate-revalidation-evidence.json").write_bytes(evidence_raw)
for path,raw,cp,text in prepared:
    path.parent.mkdir(parents=True,exist_ok=True); path.write_bytes(raw); cp.write_bytes(text)
    print(path.name, digest(raw))
