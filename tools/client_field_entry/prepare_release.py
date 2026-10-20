"""Prepare a package_native-compatible combined overlay without rebuilding or packaging."""
import argparse
import json
from pathlib import Path
import shutil
import sys
import zipfile

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.client_stamina_sweep import build as stamina
from tools.client_field_entry import build as field
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body,require
import dnfile

sha=stamina.sha
MAIN='assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER='assets/bin/Data/Managed/SoulFlowerStaminaSweep.dll'
FINAL_SHA='a1223acf96fff8db43a14de799028af12a7fd5ef8f692d21f600192de2c22ef3'
BASE_SHA='45bcf1ef1529bcdc85866923a53629f7e08a7a24540d34beb7230822a9a22b13'

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--base-apk',type=Path,default=ROOT/'apks/build/hyjs-local.apk')
 p.add_argument('--map-overlay',type=Path,default=ROOT/'analysis/playability31-20261020/field-audit/map-lock-r1')
 p.add_argument('--stamina-overlay',type=Path,default=field.BASE,help='Qualified stamina overlay with the same pinned main DLL; helper may contain layout-only updates')
 p.add_argument('--output-directory',type=Path,required=True)
 a=p.parse_args();out=a.output_directory.resolve();base=a.base_apk.resolve();overlay=a.map_overlay.resolve()
 require(out.is_relative_to(ROOT/'analysis') and not out.exists(),'Fresh analysis output required')
 require(sha(base.read_bytes())==field.APK_SHA,'Base APK SHA mismatch')
 final=(overlay/'Assembly-CSharp.dll').read_bytes();require(sha(final)==FINAL_SHA,'Final managed SHA mismatch')
 stamina_overlay=a.stamina_overlay.resolve()
 intermediate=(stamina_overlay/'Assembly-CSharp.dll').read_bytes()
 require(sha(intermediate)==field.INPUT_SHA,'Qualified stamina main DLL differs')
 helper=(stamina_overlay/'SoulFlowerStaminaSweep.dll').read_bytes()
 inherited=json.loads((stamina_overlay/'manifest.json').read_text())
 require(sha(helper)==inherited['helperSHA256'],'Inherited stamina helper SHA mismatch')
 with zipfile.ZipFile(base) as z:
  require(len(z.namelist())==len(set(z.namelist())),'Duplicate base entries')
  original=z.read(MAIN);require(sha(original)==BASE_SHA,'Base managed SHA mismatch')
  dex=sha(z.read('classes2.dex'))
  # Neither adapter nor package_native has a Java payload in managedOnly mode.
  # Exact original Java/Dex bytes are selected for inheritance, hence their ABI is exact.
  java={n:sha(z.read(n)) for n in z.namelist() if n.endswith(('.dex','.class','.jar')) or n.startswith('lib/') and n.endswith('.so')}
  require('classes2.dex' in java and bool(java),'Java inheritance inventory missing')
 stage1=stamina.verify(original,intermediate)
 stage2=field.verify(intermediate,final)
 left,right=dnfile.dnPE(data=original),dnfile.dnPE(data=final)
 lm,rm=methods(left),methods(right);require(len(lm)==len(rm),'Combined method inventory differs')
 allowed=set(stamina.PINS)|set(field.PINS);require(len(allowed)==13,'Expected thirteen target methods')
 changed=[];unchanged=engine=0
 for (name,rid,x),(newname,newrid,y) in zip(lm,rm):
  require((name,rid,x.Signature.value,x.struct.Flags,x.struct.ImplFlags)==(newname,newrid,y.Signature.value,y.struct.Flags,y.struct.ImplFlags),'Combined method ABI/token differs')
  if not x.Rva:require(not y.Rva,'Unexpected new body');continue
  old,new=exact_body(left,x),exact_body(right,y)
  if old!=new:
   require(name in allowed,'Unexpected combined change: '+name)
   changed.append(dict(method=name,beforeSHA256=sha(old),afterSHA256=sha(new)))
  else:unchanged+=1;engine+=name.startswith('Battle.Engine')
 require({x['method'] for x in changed}==allowed and engine==547,'Combined target or engine scope differs')
 payload={MAIN:final,HELPER:helper}
 require(all(n.startswith('assets/bin/Data/Managed/') and n.endswith('.dll') for n in payload),'Unexpected Java payload')
 # Recheck all inherited entry digests in the pinned APK before attesting inheritance.
 with zipfile.ZipFile(base) as z:
  require(all(sha(z.read(n))==h for n,h in java.items()),'Inherited Java bytes changed')
 require(sha(base.read_bytes())==field.APK_SHA,'Base APK changed during validation')
 out.mkdir(parents=True)
 files={}
 for entry,raw in payload.items():
  destination=out/Path(entry).name;destination.write_bytes(raw)
  require(sha(destination.read_bytes())==sha(raw),'Copied payload differs')
  files[entry]=str(destination.relative_to(ROOT))
 sourcefiles=[Path(__file__).resolve()]
 for directory in ('client_stamina_sweep','client_field_entry'):
  sourcefiles += [x for x in sorted((ROOT/'tools'/directory).iterdir()) if x.is_file() and x.suffix in ('.py','.cs','.md')]
 sources={str(x.relative_to(ROOT)):sha(x.read_bytes()) for x in set(sourcefiles)}
 report=dict(schema='hyjs-combined-stamina-world-map-managed-release-v1',status='STATIC_VERIFIED_NOT_PACKAGED',managedOnly=True,
  baseAPK=str(base.relative_to(ROOT)),baseAPKSHA256=field.APK_SHA,inputDexSHA256=dex,inputManagedSHA256=BASE_SHA,
  intermediateManagedSHA256=field.INPUT_SHA,outputManagedSHA256=FINAL_SHA,
  inheritedJavaByteExact=True,inheritedJavaPublicProtectedABIExact=True,inheritedJavaEntrySHA256=java,
  javaProof='Payload has only two managed DLL entries. Every original Java/Dex/native-library entry is pinned and prevalidated from the fixed base APK; package_native copies and verifies each inherited entry and compressed payload again.',
  payloadFiles=files,replacementEntries={n:sha(v) for n,v in payload.items()},allowedChangedMethods=sorted(allowed),
  sourcePins=dict(sorted(sources.items())),changes=changed,originalEngineMethodsUnchanged=engine,unrelatedRawMethodBodiesUnchanged=unchanged,
  originalMetadataABIAndTokensPreserved=True,originalBranchAndGuardBoundariesVerified=True,
  stageChecks={'stamina':stage1,'map':stage2},deviceExecuted=False,visualVerified=False,packaged=False,
  requiredDeviceChecks=['World map 001/002 retain their level gate; 003..006 stay visible with original lock layers and readable 暂未开放 labels.','Clicks on 003..006 use the original not-open popup; 001/002 enter normally.','Inherited stamina purchase and sweep-stop interactions remain correct.'])
 (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
 print(json.dumps({k:report[k] for k in ['status','outputManagedSHA256','replacementEntries','allowedChangedMethods','originalEngineMethodsUnchanged','unrelatedRawMethodBodiesUnchanged']},ensure_ascii=False))

if __name__=='__main__':main()
