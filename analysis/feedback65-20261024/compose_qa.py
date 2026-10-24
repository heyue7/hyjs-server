"""Compose the published63 field correction, optionally with four isolated QA origins."""
import argparse,hashlib,json,sys,zipfile
from pathlib import Path
import dnfile
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT))
from tools.client_feedback58.compose import qa_main
from tools.client_one_key59.build import verify
from tools.client_arena_state.build import methods,exact_body
MAIN='assets/bin/Data/Managed/Assembly-CSharp.dll'
BASE='f3ce73cc1f2e41bb45873c64f01e9a6ec81d84d1e41f18482c27d62c42ace3f9'
sha=lambda b:hashlib.sha256(b).hexdigest()
p=argparse.ArgumentParser();p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--field-overlay',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);p.add_argument('--qa-origin');a=p.parse_args()
out=a.output_directory.resolve();assert out.is_relative_to(ROOT) and not out.exists();assert hashlib.file_digest(a.base_apk.open('rb'),'sha256').hexdigest()==BASE
m=json.loads((a.field_overlay/'manifest.json').read_text());assert m['baseAPKSHA256']==BASE and m['originalEngineMethodsExact']==547
pins=dict(m['sourcePins'])
for f,h in pins.items():assert sha((ROOT/f).read_bytes())==h
with zipfile.ZipFile(a.base_apk) as z:before=z.read(MAIN);dex=sha(z.read('classes2.dex'))
assert sha(before)==m['inputManagedSHA256'];payload={}
for entry,file in m['payloadFiles'].items():
 path=(ROOT/file).resolve();assert path.is_relative_to(ROOT) and sha(path.read_bytes())==m['replacementEntries'][entry];payload[entry]=file
current=(ROOT/payload[MAIN]).read_bytes();targets=[{'token':t} for t in m['changedMethodTokens']];checks=verify(before,current,targets)
out.mkdir(parents=True);qa=None
if a.qa_origin:
 assert a.qa_origin=='http://127.0.0.1:28979'
 prev=current;current,qa=qa_main(current,a.base_apk,a.qa_origin,out)
 x,y=dnfile.dnPE(data=prev),dnfile.dnPE(data=current)
 extra=[{'token':hex(0x6000000+rid)} for (_,rid,old),(_,_,new) in zip(methods(x),methods(y)) if old.Rva and exact_body(x,old)!=exact_body(y,new)]
 assert len(extra)==4;targets+=extra;checks=verify(before,current,targets)
 for f in ['tools/client_feedback58/compose.py','tools/client_remote/build.py','tools/client_remote/Injector.cs']:pins[f]=sha((ROOT/f).read_bytes())
(out/'Assembly-CSharp.dll').write_bytes(current);payload[MAIN]=str((out/'Assembly-CSharp.dll').relative_to(ROOT));pins[str(Path(__file__).relative_to(ROOT))]=sha(Path(__file__).read_bytes())
names=sorted(n for n,rid,_ in methods(dnfile.dnPE(data=current)) if hex(0x6000000+rid) in checks['changedMethodTokens'])
report=dict(schema='hyjs-feedback65-qa-managed-v1',baseAPKSHA256=BASE,inputManagedSHA256=sha(before),outputManagedSHA256=sha(current),inputDexSHA256=dex,managedOnly=True,inheritedJavaByteExact=True,inheritedJavaPublicProtectedABIExact=True,payloadFiles=payload,replacementEntries={entry:sha((ROOT/file).read_bytes()) for entry,file in payload.items()},sourcePins=pins,allowedChangedMethods=names,qaOnly=bool(a.qa_origin),neverPromote=bool(a.qa_origin),qaOrigin=a.qa_origin,qaOriginProof=qa,**checks)
(out/'manifest.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({'managedSHA256':sha(current),'qaOnly':bool(a.qa_origin),'changedMethods':names}))
