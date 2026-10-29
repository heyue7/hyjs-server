"""Prepare a pinned, managed-only QA overlay and strict desert resource inputs."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import sys
import zipfile

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.desert_extraction import build_v78_adapter, build_field_patch, package_resources
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body
import dnfile

ENTRY='assets/bin/Data/Managed/Assembly-CSharp.dll'
MAP_HELPER='assets/bin/Data/Managed/SoulFlowerDesert53.dll'
V4_HELPER='assets/bin/Data/Managed/SoulFlowerDesertV4.dll'
BASE=package_resources.BASE_SHA

def digest(data): return hashlib.sha256(data).hexdigest()
def require(ok,msg):
    if not ok: raise ValueError(msg)
def pinned(root,entries):
    for name,value in entries.items():
        f=(root/name).resolve()
        require(f.is_relative_to(root) and f.is_file() and digest(f.read_bytes())==value,'Source pin differs: '+name)

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--map-overlay',type=Path,required=True)
    p.add_argument('--preview',type=Path,required=True);p.add_argument('--bundle',type=Path,required=True);p.add_argument('--output',type=Path,required=True)
    a=p.parse_args();base,overlay,preview,bundle,out=(x.resolve() for x in (a.base_apk,a.map_overlay,a.preview,a.bundle,a.output))
    require(out.is_relative_to(ROOT/'analysis/desert-extraction-v4-client') and not out.exists(),'Use a fresh owned output directory')
    require(package_resources.sha_file(base)==BASE,'Wrong v80 base APK')
    m=json.loads((overlay/'manifest.json').read_text());v=json.loads((preview/'manifest.json').read_text())
    require(m.get('schema')=='hyjs-desert53-runtime-adapter-v2' and m.get('status')=='STATIC_VERIFIED_OVERLAY_NOT_PACKAGED' and m.get('desertEntrySelectable') and m.get('desertLoginResume') and not m.get('walkOnly'),'Unqualified map adapter')
    require(v.get('schema')=='hyjs-desert-v4-scene-overlay-v2' and v.get('status')=='STATIC_PREVIEW_NOT_FOR_RELEASE' and v.get('apkSHA256')==BASE,'Unqualified field preview')
    sources={**m['sourceSHA256'],**v['sourceSHA256']}
    for f in (Path(__file__).resolve(),ROOT/'tools/desert_extraction/package_resources.py'):
        sources[str(f.relative_to(ROOT))]=digest(f.read_bytes())
    pinned(ROOT,sources)
    native_map=(overlay/'Assembly-CSharp.dll').read_bytes();native_final=(preview/'Assembly-CSharp.dll').read_bytes()
    map_helper=(overlay/'SoulFlowerDesert53.dll').read_bytes();v4_helper=(preview/'SoulFlowerDesertV4.dll').read_bytes();map_raw=bundle.read_bytes()
    require(digest(native_map)==m['outputManagedSHA256']==v['inputManagedSHA256'] and digest(native_final)==v['outputManagedSHA256'] and digest(map_helper)==m['helperSHA256'] and digest(v4_helper)==v['helperSHA256'],'Managed payload identity differs')
    with zipfile.ZipFile(base) as z:
        names=z.namelist();require(len(names)==len(set(names)) and ENTRY in names and package_resources.BUNDLE_ENTRY not in names and MAP_HELPER not in names and V4_HELPER not in names,'Base APK entries differ')
        original=z.read(ENTRY);dex=z.read('classes2.dex');catalog=z.read(package_resources.CATALOG_ENTRY)
        require(digest(original)==m['inputManagedSHA256'],'Original v80 managed input differs')
        expected_catalog=package_resources.compose_catalog(catalog,map_raw)
        require(digest(catalog)==m['originalCatalogSHA256'] and digest(expected_catalog)==m['updatedCatalogSHA256'] and expected_catalog==(overlay/'smallassetmanifest').read_bytes(),'v80 catalog composition differs')
        inherited={name:digest(z.read(name)) for name in names if name.startswith('lib/') or (name.startswith('classes') and name.endswith('.dex'))}
    map_check=build_v78_adapter.verify(original,native_map,len(map_raw),False,True,True)
    field_check=build_field_patch.verify(native_map,native_final)
    require(map_check['originalEngineMethodsUnchanged']==field_check['originalEngineMethodsUnchanged']==547 and map_check['qualityFilterV80Exact'] and field_check['qualityFilterV80Exact'],'Combat engine or released quality filter changed')
    pe1,pe2=dnfile.dnPE(data=original),dnfile.dnPE(data=native_final);old,new=methods(pe1),methods(pe2)
    require(len(old)==len(new),'Managed inventory changed');changed=[]
    for (name,rid,x),(name2,rid2,y) in zip(old,new):
        require((name,rid)==(name2,rid2) and x.Signature.value==y.Signature.value and x.struct.Flags==y.struct.Flags,'Managed ABI changed')
        if x.Rva and exact_body(pe1,x)!=exact_body(pe2,y):changed.append(name)
    expected={build_v78_adapter.TARGET,build_v78_adapter.RAY_TARGET}
    expected.update(x['method'] for key in ('loaderChanges','desertEntryChanges','desertLoginResumeChanges') for x in map_check[key])
    expected.update(x['method'] for x in field_check['fieldHooks'])
    require(set(changed)==expected,'Composite changed-method scope differs')
    out.mkdir(parents=True)
    data={'Assembly-CSharp.dll':native_final,'SoulFlowerDesert53.dll':map_helper,'SoulFlowerDesertV4.dll':v4_helper,'smallassetmanifest':expected_catalog,'map_desertfield_001':map_raw}
    for name,raw in data.items():(out/name).write_bytes(raw)
    files={ENTRY:'Assembly-CSharp.dll',MAP_HELPER:'SoulFlowerDesert53.dll',V4_HELPER:'SoulFlowerDesertV4.dll'}
    payload={k:str((out/v).relative_to(ROOT)) for k,v in files.items()}
    report={'schema':'hyjs-desert-v4-managed-composite-v1','status':'STATIC_VERIFIED_QA_NOT_RELEASED','qaOnly':True,'neverPromote':True,
        'managedOnly':True,'baseAPKSHA256':BASE,'inputManagedSHA256':digest(original),'outputManagedSHA256':digest(native_final),'inputDexSHA256':digest(dex),
        'inheritedJavaByteExact':True,'inheritedJavaPublicProtectedABIExact':True,'inheritedJavaEntrySHA256':inherited,
        'javaProof':'This managed-only overlay leaves inherited Java and ABI bytes exact before the separately pinned resource/catalog overlay and its required manifest-pin update.',
        'payloadFiles':payload,'replacementEntries':{k:digest(data[v]) for k,v in files.items()},'allowedChangedMethods':sorted(changed),
        'sourcePins':sources,'originalEngineMethodsExact':547,'qualityFilterV80Exact':True,'deviceExecuted':False,'serverExecuted':False}
    (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    rf={package_resources.CATALOG_ENTRY:'smallassetmanifest',package_resources.BUNDLE_ENTRY:'map_desertfield_001'}
    r={'schema':'hyjs-desert-v4-map-resource-overlay-v1','baseAPKSHA256':BASE,'bundleSHA256':package_resources.BUNDLE_SHA,'bundleBytes':len(map_raw),
       'originalCatalogSHA256':digest(catalog),'updatedCatalogSHA256':digest(expected_catalog),
       'payloadFiles':{k:str((out/v).relative_to(ROOT)) for k,v in rf.items()},'replacementEntries':{k:digest(data[v]) for k,v in rf.items()},
       'sourcePins':sources,'releaseReady':False}
    (out/'resource-manifest.json').write_text(json.dumps(r,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'output':str(out),'managedSHA256':digest(native_final),'helperSHA256':digest(v4_helper),'bundleSHA256':digest(map_raw),'catalogSHA256':digest(expected_catalog),'allowedMethods':len(changed),'qaOnly':True}))
if __name__=='__main__': main()
