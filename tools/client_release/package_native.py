"""Package a qualified native-account DEX overlay, preserving the entire managed client."""
import argparse,copy,fcntl,hashlib,json,os,shutil,sys,tempfile,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools import build_wallet_client as common
from tools.client_branding.package_release import compressed_hash,copy_original_record,require

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument("--base-apk",type=Path,required=True);p.add_argument("--overlay",type=Path,required=True);p.add_argument("--output",type=Path,required=True);p.add_argument("--record",type=Path,required=True);a=p.parse_args()
 base,overlay,output,record=[x.resolve()for x in (a.base_apk,a.overlay,a.output,a.record)]
 proof=json.loads((overlay/"manifest.json").read_text());require(proof.get("managedAssemblyByteExact") and proof["inheritedJavaPublicProtectedABIExact"],"Qualified DEX-only overlay required")
 require(not output.exists() and not record.exists(),"Fresh output required");require(common.digest(base)==proof["baseAPKSHA256"],"Base changed")
 pins={ROOT/n:h for n,h in proof["sourcePins"].items()}
 for f,h in pins.items():require(f.resolve().is_relative_to(ROOT)and common.digest(f)==h,"Source pin changed: "+str(f))
 raw=(overlay/"classes2.dex").read_bytes();require(hashlib.sha256(raw).hexdigest()==proof["outputDexSHA256"],"DEX differs")
 sdk=Path.home()/"Library/Android/sdk/build-tools/33.0.0";jdk=Path("/opt/homebrew/opt/openjdk@21");env=dict(os.environ,JAVA_HOME=str(jdk),PATH=str(jdk/"bin")+os.pathsep+os.environ.get("PATH",""))
 require(shutil.disk_usage(ROOT).free>base.stat().st_size*2+150_000_000,"Insufficient packaging space")
 cert=common._certificate(common.run([sdk/"apksigner","verify","--print-certs",base],env));inherited={};compressed={}
 with (ROOT/"build/.client-build.lock").open("a") as lock:
  fcntl.flock(lock,fcntl.LOCK_EX)
  with tempfile.TemporaryDirectory(prefix="hyjs-native-only-",dir=ROOT/"build") as temporary:
   temp=Path(temporary);unsigned,aligned,signed=[temp/n for n in ("unsigned.apk","aligned.apk","signed.apk")]
   with zipfile.ZipFile(base) as z,base.open("rb") as stream:
    require(common._hash_entry(z,common.DLL)==proof["inputManagedSHA256"]==proof["outputManagedSHA256"],"Managed base differs")
    require(common._hash_entry(z,"classes2.dex")==proof["inputDexSHA256"],"Native base differs")
    with zipfile.ZipFile(unsigned,"w") as target:
     target.comment=z.comment
     for info in z.infolist():
      if common._signature(info.filename):continue
      if info.filename=="classes2.dex":target.writestr(copy.copy(info),raw)
      else:inherited[info.filename]=common._hash_entry(z,info.filename);compressed[info.filename]=compressed_hash(stream,info);copy_original_record(stream,target,info)
   common.run([sdk/"zipalign","-f","-p","4",unsigned,aligned],env);unsigned.unlink()
   common.run([sdk/"apksigner","sign","--ks",ROOT/".local-signing/debug.keystore","--ks-pass","pass:android","--key-pass","pass:android","--out",signed,aligned],env);aligned.unlink()
   require(common._certificate(common.run([sdk/"apksigner","verify","--verbose","--print-certs",signed],env))==cert,"Signer differs");common.run([sdk/"zipalign","-c","-p","4",signed],env)
   with zipfile.ZipFile(signed) as z,signed.open("rb") as stream:
    require(len(z.namelist())==len(set(z.namelist())) and {n for n in z.namelist()if not common._signature(n)}==set(inherited)|{"classes2.dex"},"Entry inventory differs")
    for n,h in {**inherited,"classes2.dex":proof["outputDexSHA256"]}.items():require(common._hash_entry(z,n)==h,"Entry differs: "+n)
    for n,h in compressed.items():require(compressed_hash(stream,z.getinfo(n))==h,"Compressed inherited entry differs: "+n)
   for f,h in pins.items():require(common.digest(f)==h,"Source changed while packaging")
   require(common.digest(base)==proof["baseAPKSHA256"],"Base changed while packaging")
   result={"status":"PASS_SIGN_ALIGN_NATIVE_ONLY_NOT_INSTALLED","binarySHA256":common.digest(signed),"baseAPKSHA256":proof["baseAPKSHA256"],"replacementEntries":{"classes2.dex":proof["outputDexSHA256"]},"allOtherEntriesAndCompressedPayloadsExact":True,"managedAssemblyByteExact":True,"signatureVerified":True,"zipalignVerified":True,"outputAPK":str(output.relative_to(ROOT))}
   signed.replace(output)
   if signed.with_suffix(".apk.idsig").exists():signed.with_suffix(".apk.idsig").replace(output.with_suffix(".apk.idsig"))
 record.parent.mkdir(parents=True,exist_ok=True);record.write_text(json.dumps(result,indent=2)+"\n");print(json.dumps(result))
if __name__=="__main__":main()
