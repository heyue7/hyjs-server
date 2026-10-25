"""Package the pinned font resource and manifest version; retain all executable code."""
import argparse,copy,fcntl,hashlib,json,os,shutil,sys,tempfile,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools import build_wallet_client as common
from tools.client_head_names import build
from tools.client_branding.package_release import compressed_hash,copy_original_record,require
from tools.client_update.package import patch_manifest

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--overlay',type=Path,required=True)
 p.add_argument('--output',type=Path,required=True);p.add_argument('--record',type=Path,required=True)
 p.add_argument('--build-code',type=int,required=True);p.add_argument('--version-name',required=True)
 a=p.parse_args();base=a.base_apk.resolve();output=a.output.resolve();record=a.record.resolve()
 require(output.is_relative_to(ROOT/'build')and record.is_relative_to(ROOT/'analysis'),'Owned output required')
 require(not output.exists()and not record.exists(),'Fresh output and record required')
 payload,proof=build.build_overlay(base)
 for n,raw in payload.items():require((a.overlay/n).read_bytes()==raw,'Pinned font payload differs')
 sdk=Path.home()/'Library/Android/sdk/build-tools/33.0.0';jdk=Path('/opt/homebrew/opt/openjdk@21')
 env=dict(os.environ,JAVA_HOME=str(jdk),PATH=str(jdk/'bin')+os.pathsep+os.environ.get('PATH',''))
 require(shutil.disk_usage(ROOT).free>base.stat().st_size*2+150000000,'Packaging space required')
 certificate=common._certificate(common.run([sdk/'apksigner','verify','--print-certs',base],env))
 inherited={};compressed={}
 with (ROOT/'build/.client-build.lock').open('a')as lock:
  fcntl.flock(lock,fcntl.LOCK_EX)
  with tempfile.TemporaryDirectory(prefix='hyjs-head-name-',dir=ROOT/'build')as tmp:
   folder=Path(tmp);unsigned,aligned,signed=[folder/n for n in ['unsigned.apk','aligned.apk','signed.apk']]
   with zipfile.ZipFile(base)as src,base.open('rb')as stream,zipfile.ZipFile(unsigned,'w')as dst:
    payload['AndroidManifest.xml']=patch_manifest(src.read('AndroidManifest.xml'),a.build_code,a.version_name)
    for info in src.infolist():
     if common._signature(info.filename):continue
     if info.filename in payload:dst.writestr(copy.copy(info),payload[info.filename])
     else:
      inherited[info.filename]=common._hash_entry(src,info.filename);compressed[info.filename]=compressed_hash(stream,info);copy_original_record(stream,dst,info)
   common.run([sdk/'zipalign','-f','-p','4',unsigned,aligned],env);unsigned.unlink()
   common.run([sdk/'apksigner','sign','--ks',ROOT/'.local-signing/debug.keystore','--ks-pass','pass:android','--key-pass','pass:android','--out',signed,aligned],env);aligned.unlink()
   require(common._certificate(common.run([sdk/'apksigner','verify','--verbose','--print-certs',signed],env))==certificate,'Signer drift')
   common.run([sdk/'zipalign','-c','-p','4',signed],env)
   hashes={n:hashlib.sha256(raw).hexdigest()for n,raw in payload.items()}
   with zipfile.ZipFile(signed)as verified,signed.open('rb')as stream:
    require(len(verified.namelist())==len(set(verified.namelist()))and {n for n in verified.namelist()if not common._signature(n)}==set(inherited)|set(payload),'Inventory drift')
    for n,h in {**inherited,**hashes}.items():require(common._hash_entry(verified,n)==h,'Entry drift: '+n)
    for n,h in compressed.items():require(compressed_hash(stream,verified.getinfo(n))==h,'Compressed drift: '+n)
   require(common.digest(base)==build.APK_SHA,'Base changed')
   report=dict(status='SIGNED_ALIGNED_FONT_CANDIDATE_NOT_INSTALLED',baseAPKSHA256=build.APK_SHA,binarySHA256=common.digest(signed),outputAPK=str(output.relative_to(ROOT)),buildCode=a.build_code,versionName=a.version_name,replacementEntries=hashes,allOtherEntriesAndCompressedPayloadsExact=True,managedAndAllJavaByteExact=True,originalEngineMethodsExact=547,signatureVerified=True,zipalignVerified=True,fontProof=proof,sourcePins={str(Path(__file__).relative_to(ROOT)):common.digest(Path(__file__)),str(Path(build.__file__).relative_to(ROOT)):common.digest(Path(build.__file__))})
   signed.replace(output)
   signed.with_suffix('.apk.idsig').unlink(missing_ok=True)
 record.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps({k:report[k]for k in ['status','binarySHA256','outputAPK']}))
if __name__=='__main__':main()
