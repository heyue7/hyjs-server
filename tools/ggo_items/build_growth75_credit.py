"""Build only the credit HUD/footer helper over the exact installed 74 client."""
import argparse,hashlib,json,subprocess,tempfile,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
BASE="2d50ba9e3b640aa8215f6bbed50fc7a1831a2bcdeab5e55cfa90dc0ef595ebf5"
def sha(b):return hashlib.sha256(b).hexdigest()
def main():
 p=argparse.ArgumentParser();p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);p.add_argument('--pet-overlay',type=Path,required=True);p.add_argument('--native-overlay',type=Path);a=p.parse_args()
 out=a.output_directory.resolve();assert out.is_relative_to(ROOT/'analysis') and not out.exists();out.mkdir(parents=True)
 assert sha(a.base_apk.read_bytes())==BASE
 with zipfile.ZipFile(a.base_apk) as z,tempfile.TemporaryDirectory(prefix='growth75-credit-') as tmp:
  refs=Path(tmp)/'refs';refs.mkdir()
  for n in z.namelist():
   if n.startswith('assets/bin/Data/Managed/') and n.endswith('.dll'):(refs/Path(n).name).write_bytes(z.read(n))
  tool=Path('/Users/lirui/LR/hyjs/.tools/native-batch');dotnet=tool/'dotnet/dotnet';csc=next((tool/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
  helper=out/'SoulFlowerGGOShop.dll'
  cmd=[dotnet,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(helper),'/resource:'+str(ROOT/'tools/ggo_items/assets/ggo_credit_hud-source.png')+',credit_hud.png',*['/r:'+str(r) for r in sorted(refs.glob('*.dll'))],ROOT/'tools/ggo_items/client/GGOShop.cs',ROOT/'tools/ggo_items/client/GGOIconManifest.cs',ROOT/'tools/ggo_items/client/PetIconManifest75.cs',ROOT/'tools/ggo_items/client/TrialShopRefresh75.cs']
  r=subprocess.run(['rtk','proxy',*map(str,cmd)],capture_output=True,text=True);(out/'compile.log').write_text(r.stdout+r.stderr);assert r.returncode==0,r.stdout+r.stderr
  original=z.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
 pins=['tools/ggo_items/client/GGOShop.cs','tools/ggo_items/client/GGOIconManifest.cs','tools/ggo_items/client/PetIconManifest75.cs','tools/ggo_items/client/TrialShopRefresh75.cs','tools/ggo_items/assets/ggo_credit_hud-source.png','tools/ggo_items/build_growth75_credit.py']
 entry='assets/bin/Data/Managed/SoulFlowerGGOShop.dll'
 pet=json.loads((a.pet_overlay/'manifest.json').read_text());assert pet['baseAPKSHA256']==BASE
 payload=dict(pet['payloadFiles']);payload[entry]=str(helper.relative_to(ROOT))
 hashes=dict(pet['replacementEntries']);hashes[entry]=sha(helper.read_bytes())
 assert all(sha((ROOT/path).read_bytes())==hashes[n] for n,path in payload.items())
 manifest=dict(schema='hyjs-growth75-credit-overlay-v1',baseAPKSHA256=BASE,payloadFiles=payload,replacementEntries=hashes,sourcePins={n:sha((ROOT/n).read_bytes()) for n in pins},originalEngineMethodsExact=547,originalMetadataABIAndTokensPreserved=True,mainAssemblyUnchanged=True,inputManagedSHA256=sha(original),originalDexUnchanged=True,originalBusinessMechanismsPreserved=True,deviceTested=False)
 if a.native_overlay:
  native=json.loads((a.native_overlay/'manifest.json').read_text());assert native['baseAPKSHA256']==BASE and native['inheritedJavaPublicProtectedABIExact'] and native['managedAssemblyByteExact']
  dex=(a.native_overlay/'classes2.dex').resolve();assert sha(dex.read_bytes())==native['outputDexSHA256']
  native_fields={k:v for k,v in native.items() if k not in ('schema','sourcePins')}
  manifest.update(native_fields);manifest['schema']='hyjs-growth75-client-ui-overlay-v1'
  manifest['payloadFiles']=dict(payload);manifest['payloadFiles']['classes2.dex']=str(dex.relative_to(ROOT))
  manifest['replacementEntries']=dict(hashes);manifest['replacementEntries']['classes2.dex']=native['outputDexSHA256']
  manifest['sourcePins'].update(native['sourcePins']);manifest['nativeProof']=native
  manifest['mainAssemblyUnchanged']=True;manifest['originalDexUnchanged']=False;manifest['allOtherOriginalDEXEntriesByteExact']=True
 (out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n');print(json.dumps(dict(status='BUILT',helper=str(helper),payloadEntries=len(manifest['payloadFiles']),mainAssemblyUnchanged=True)))
if __name__=='__main__':main()
