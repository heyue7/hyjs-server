"""Compose HUD helper and original-format growth rows over the verified 73 APK."""
import argparse,hashlib,json,subprocess,tempfile,zipfile,sys
from pathlib import Path
import UnityPy,dnfile
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
BASE='f1fd6052e26b8646458e1fb2b3ada53a647d86b133fd3fdbd4cd4bd27895e2bc'
def sha(x):return hashlib.sha256(x).hexdigest()
def patch_table(raw,changes,key):
 env=UnityPy.load(raw);obj=next(o for o in env.objects if o.type.name=='TextAsset');tree=obj.read_typetree();text=tree['m_Script'];text=text.decode()if isinstance(text,bytes)else text
 lines=text.splitlines();keys=lines[1].split('\t');where={l.split('\t')[keys.index(key)]:i for i,l in enumerate(lines[5:],5)}
 for row in changes:
  line='\t'.join(row.get(k,'')for k in keys)
  if row[key]in where:lines[where[row[key]]]=line
  else:lines.append(line)
 tree['m_Script']='\n'.join(lines)+'\n';obj.save_typetree(tree);return env.file.save(packer='lz4')
def main():
 p=argparse.ArgumentParser();p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);a=p.parse_args();base=a.base_apk;out=a.output_directory.resolve();assert out.is_relative_to(ROOT/'analysis')and not out.exists();out.mkdir(parents=True)
 assert sha(base.read_bytes())==BASE
 resources=json.loads((ROOT/'hyjs-server/service/data/growth74_resources.json').read_text());items,uses=resources['items'],resources['uses']
 texts=[]
 for r in items:
  r['intro']=r['desc']
  texts.extend([dict(id='name_'+r['itemId'],text=r['name']),dict(id='desc_'+r['itemId'],text=r['desc'])])
 patches=json.loads((ROOT/'hyjs-server/service/data/growth74_table_rows.json').read_text());patches.update(Item=dict(key='itemId',rows=items),ItemUse=dict(key='id',rows=uses),Text=dict(key='id',rows=texts))
 payload={}
 with zipfile.ZipFile(base)as z,tempfile.TemporaryDirectory(prefix='growth74-helper-')as tmp:
  work=Path(tmp);refs=work/'refs';refs.mkdir()
  for n in z.namelist():
   if n.startswith('assets/bin/Data/Managed/')and n.endswith('.dll'):(refs/Path(n).name).write_bytes(z.read(n))
  tool=Path('/Users/lirui/LR/hyjs/.tools/native-batch');dotnet=tool/'dotnet/dotnet';csc=next((tool/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
  helper=out/'SoulFlowerGGOShop.dll';cmd=[dotnet,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(helper),'/resource:'+str(ROOT/'tools/ggo_items/assets/ggo_credit_hud-source.png')+',credit_hud.png',*['/r:'+str(r)for r in sorted(refs.glob('*.dll'))],ROOT/'tools/ggo_items/client/GGOShop.cs',ROOT/'tools/ggo_items/client/GGOIconManifest.cs']
  r=subprocess.run(['rtk','proxy',*map(str,cmd)],capture_output=True,text=True);(out/'compile.log').write_text(r.stdout+r.stderr);assert r.returncode==0,r.stdout+r.stderr
  payload['assets/bin/Data/Managed/SoulFlowerGGOShop.dll']=str(helper.relative_to(ROOT))
  for name,patch in patches.items():
   entry='assets/android/tabledata/table'+name.lower();raw=patch_table(z.read(entry),patch['rows'],patch['key']);file=out/('table'+name.lower());file.write_bytes(raw);payload[entry]=str(file.relative_to(ROOT))
  original=z.read('assets/bin/Data/Managed/Assembly-CSharp.dll');pe=dnfile.dnPE(data=original);engine=sum(bool(row.Rva)for name,_,row in methods(pe)if name.startswith('Battle.Engine'))
  assert engine==547
 pins=['tools/ggo_items/client/GGOShop.cs','tools/ggo_items/client/GGOIconManifest.cs','tools/ggo_items/build_growth74.py','tools/ggo_items/assets/ggo_credit_hud-source.png','hyjs-server/service/data/growth74_resources.json','hyjs-server/service/data/growth74_table_rows.json']
 manifest=dict(sourcePins={n:sha((ROOT/n).read_bytes())for n in pins},schema='hyjs-growth74-overlay-v1',baseAPKSHA256=BASE,payloadFiles=payload,replacementEntries={k:sha((ROOT/v).read_bytes())for k,v in payload.items()},originalEngineMethodsExact=engine,originalMetadataABIAndTokensPreserved=True,mainAssemblyUnchanged=True,inputManagedSHA256=sha(original),originalDexUnchanged=True,originalBusinessMechanismsPreserved=True,deviceTested=False)
 (out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n');print('Verified overlay:',len(payload),'entries; engine',engine,'methods untouched')
if __name__=='__main__':main()
