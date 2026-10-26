"""Build the three absent pet portraits from their own original model renders."""
import argparse,hashlib,json,zipfile,sys
from pathlib import Path
from PIL import Image
import UnityPy
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT))
from tools.ggo_items.build_assets import update_bundle_index,resize_alpha
BASE="2d50ba9e3b640aa8215f6bbed50fc7a1831a2bcdeab5e55cfa90dc0ef595ebf5"
def sha(b):return hashlib.sha256(b).hexdigest()
def bundle(template,image,name):
 e=UnityPy.load(template);o=next(o for o in e.objects if o.type.name=='Texture2D');t=o.read();t.m_Name=name;t.set_image(image,target_format=4,mipmap_count=1);t.save()
 b=next(o for o in e.objects if o.type.name=='AssetBundle');d=b.read_typetree();_,info=d['m_Container'][0];d['m_Container']=[('assets/outputbundle/pet/'+name+'.png',info)];d['m_AssetBundleName']='pet/'+name;b.save_typetree(d)
 old,f=next(iter(e.file.files.items()));cab='CAB-'+sha(('hyjs-pet75:'+name).encode()+image.tobytes())[:32];f.name=cab;e.file.files={cab:f};raw=e.file.save(packer='lz4');v=UnityPy.load(raw);t=next(o.read() for o in v.objects if o.type.name=='Texture2D');assert t.image.convert('RGBA').tobytes()==image.tobytes();return raw

def main():
 p=argparse.ArgumentParser();p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);a=p.parse_args();assert sha(a.base_apk.read_bytes())==BASE;out=a.output_directory.resolve();assert out.is_relative_to(ROOT/'analysis') and not out.exists();out.mkdir(parents=True)
 choices=[('pet_076','pet_030',0,(65,70,320,325)),('pet_077','pet_031',0,(140,90,245,195)),('pet_078','pet_032',0,(90,50,295,255))]
 assets=ROOT/'tools/ggo_items/assets';changes={};proof=[]
 back=Image.open(ROOT/'tools/ggo_items/frames/items/UI_kuang_big_di_red.png').convert('RGBA');frame=Image.open(ROOT/'tools/ggo_items/frames/items/UI_kuang_big_bian_red.png').convert('RGBA')
 with zipfile.ZipFile(a.base_apk) as z:
  template=z.read('assets/android/pet/pet_icon_075')
  for pet,model,yaw,crop in choices:
   icon='pet_icon_'+pet[-3:];source=ROOT/'analysis/growth75/pet-renders'/(model+'-'+str(yaw)+'.png');portrait=Image.open(source).convert('RGBA').crop(crop);image=resize_alpha(back,(128,128));image.alpha_composite(resize_alpha(portrait,(120,120)),(4,4));image.alpha_composite(resize_alpha(frame,(128,128)));image.save(assets/(icon+'.png'))
   entry='assets/android/pet/'+icon;raw=bundle(template,image,icon);target=out/icon;target.write_bytes(raw);changes[entry]=raw;proof.append(dict(id=pet,icon=icon,modelEntry='assets/android/model/'+model,modelSHA256=sha(z.read('assets/android/model/'+model)),renderSHA256=sha(source.read_bytes()),renderCrop=crop,iconSHA256=sha(image.tobytes()),origin='Original mesh and its exact original UV texture, software rasterized; original red quality frame reused. No unrelated pet substitution.'))
  changes['assets/android/android']=update_bundle_index(z.read('assets/android/android'),changes)
  lines=z.read('assets/android/smallassetmanifest').decode().splitlines();entries={l.split('\t')[0]:l.split('\t')[1:] for l in lines[1:] if l}
  for n,b in changes.items():entries[n.removeprefix('assets/android/')]=[hashlib.md5(b).hexdigest(),str(len(b))]
  changes['assets/android/smallassetmanifest']=(lines[0]+'\n'+'\n'.join('\t'.join([k,*v]) for k,v in sorted(entries.items()))+'\n').encode()
 payload={}
 for n,b in changes.items():
  target=out/Path(n).name;target.write_bytes(b);payload[n]=str(target.relative_to(ROOT))
 rows=''.join('%s\t%s\t%d\n'%(n.removeprefix('assets/android/'),hashlib.md5(b).hexdigest(),len(b)) for n,b in changes.items() if n.startswith('assets/android/pet/'))
 cs=ROOT/'tools/ggo_items/client/PetIconManifest75.cs';cs.write_text('// Exact original-model portrait bundle registrations.\nnamespace HYJS.GGO { internal static class PetIconManifest75 { internal const string Text = '+json.dumps(rows)+'; } }\n')
 report=dict(schema='hyjs-pet75-original-render-overlay',baseAPKSHA256=BASE,payloadFiles=payload,replacementEntries={n:sha(b) for n,b in changes.items()},portraits=proof,generatedRegistration=str(cs.relative_to(ROOT)),deviceTested=False)
 (out/'manifest.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(dict(status='BUILT',portraits=3,entries=len(payload))))
if __name__=='__main__':main()
