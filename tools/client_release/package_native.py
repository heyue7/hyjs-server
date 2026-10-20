"""Package qualified native/runtime overlays with explicit managed method boundaries."""
import argparse,copy,fcntl,hashlib,json,os,shutil,sys,tempfile,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools import build_wallet_client as common
from tools.client_branding.package_release import compressed_hash,copy_original_record,require
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body
import dnfile

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument("--base-apk",type=Path,required=True);p.add_argument("--overlay",type=Path,required=True);p.add_argument("--output",type=Path,required=True);p.add_argument("--record",type=Path,required=True);p.add_argument("--opening-font-overlay",type=Path);p.add_argument("--font-fallback-overlay",type=Path);a=p.parse_args()
 base,overlay,output,record=[x.resolve()for x in (a.base_apk,a.overlay,a.output,a.record)]
 proof=json.loads((overlay/"manifest.json").read_text());require(proof["inheritedJavaPublicProtectedABIExact"],"Qualified native ABI required")
 require(not output.exists() and not record.exists(),"Fresh output required");require(common.digest(base)==proof["baseAPKSHA256"],"Base changed")
 pins={ROOT/n:h for n,h in proof["sourcePins"].items()}
 for f,h in pins.items():require(f.resolve().is_relative_to(ROOT)and common.digest(f)==h,"Source pin changed: "+str(f))
 payload={}
 if not proof.get("managedOnly",False):
  raw=(overlay/"classes2.dex").read_bytes();require(hashlib.sha256(raw).hexdigest()==proof["outputDexSHA256"],"DEX differs")
  payload["classes2.dex"]=raw
 else:
  require(proof.get("inheritedJavaByteExact") is True,"Managed-only overlay must retain all Java entries")
 for name,file in proof.get("payloadFiles",{}).items():
  require(name.startswith("assets/bin/Data/Managed/") and name.endswith(".dll"),"Unexpected overlay entry")
  f=ROOT/file;require(f.resolve().is_relative_to(ROOT),"Overlay file outside project");payload[name]=f.read_bytes()
 # This optional resource overlay has an exact pinned UILabel-font byte scope.
 # Rebuild expected bytes from the pinned base rather than trusting its manifest.
 font_changes=[]
 if a.opening_font_overlay:
  from tools.client_opening_fonts import build as opening_fonts
  font_dir=a.opening_font_overlay.resolve()
  require(font_dir.is_relative_to(ROOT),"Font overlay outside project")
  require(common.digest(base)==opening_fonts.APK_SHA,"Font base differs")
  pins[Path(opening_fonts.__file__).resolve()]=common.digest(Path(opening_fonts.__file__))
  with zipfile.ZipFile(base) as z:
   opening_fonts.font_evidence(z)
   for pin in opening_fonts.PINS:
    expected,change=opening_fonts.verify_and_patch(z.read(pin["entry"]),pin)
    f=font_dir/pin["entry"]
    require(f.resolve().is_relative_to(font_dir) and f.read_bytes()==expected,"Font payload differs: "+pin["entry"])
    require(pin["entry"] not in payload,"Overlapping font payload")
    payload[pin["entry"]]=expected
    font_changes.append(change)
 fallback_report=None
 if a.font_fallback_overlay:
  from tools.client_font_fallback import build as font_fallback
  fallback_dir=a.font_fallback_overlay.resolve()
  require(fallback_dir.is_relative_to(ROOT),"Font fallback overlay outside project")
  expected,fallback_report=font_fallback.build_overlay(base)
  pins[Path(font_fallback.__file__).resolve()]=common.digest(Path(font_fallback.__file__))
  require(bool(expected),"Empty font fallback overlay")
  for name,raw in expected.items():
   require(name not in payload,"Overlapping font fallback payload")
   f=fallback_dir/name
   require(f.resolve().is_relative_to(fallback_dir) and f.read_bytes()==raw,"Font fallback payload differs: "+name)
   payload[name]=raw
 require(bool(payload),"Empty overlay is not a release")
 hashes={n:hashlib.sha256(v).hexdigest()for n,v in payload.items()}
 for n,h in proof.get("replacementEntries",{}).items():require(hashes[n]==h,"Overlay payload differs: "+n)
 sdk=Path.home()/"Library/Android/sdk/build-tools/33.0.0";jdk=Path("/opt/homebrew/opt/openjdk@21");env=dict(os.environ,JAVA_HOME=str(jdk),PATH=str(jdk/"bin")+os.pathsep+os.environ.get("PATH",""))
 require(shutil.disk_usage(ROOT).free>base.stat().st_size*2+150_000_000,"Insufficient packaging space")
 cert=common._certificate(common.run([sdk/"apksigner","verify","--print-certs",base],env));inherited={};compressed={}
 with (ROOT/"build/.client-build.lock").open("a") as lock:
  fcntl.flock(lock,fcntl.LOCK_EX)
  with tempfile.TemporaryDirectory(prefix="hyjs-native-only-",dir=ROOT/"build") as temporary:
   temp=Path(temporary);unsigned,aligned,signed=[temp/n for n in ("unsigned.apk","aligned.apk","signed.apk")]
   with zipfile.ZipFile(base) as z,base.open("rb") as stream:
    require(common._hash_entry(z,common.DLL)==proof["inputManagedSHA256"],"Managed base differs")
    require(common._hash_entry(z,"classes2.dex")==proof["inputDexSHA256"],"Native base differs")
    if common.DLL in payload:
     before,after=[dnfile.dnPE(data=b)for b in (z.read(common.DLL),payload[common.DLL])];allowed=set(proof["allowedChangedMethods"]);changed=set();engine=0
     require(len(methods(before))==len(methods(after)),"Managed method inventory differs")
     for (n,i,x),(m,j,y)in zip(methods(before),methods(after)):
      require((n,i,x.Signature.value,x.struct.Flags)==(m,j,y.Signature.value,y.struct.Flags),"Managed ABI differs")
      if x.Rva:
       if exact_body(before,x)!=exact_body(after,y):require(n in allowed,"Unapproved method: "+n);changed.add(n)
       else:engine+=n.startswith("Battle.Engine")
     require(changed==allowed and engine==547,"Managed overlay or engine scope differs")
     for row in after.net.mdtables.AssemblyRef.rows:require(not(row.struct.Flags&1 and not row.PublicKey.value),"Invalid assembly public key")
    else:require(proof.get("managedAssemblyByteExact"),"Native-only main proof missing")
    with zipfile.ZipFile(unsigned,"w") as target:
     target.comment=z.comment
     for info in z.infolist():
      if common._signature(info.filename):continue
      if info.filename in payload:target.writestr(copy.copy(info),payload[info.filename])
      else:inherited[info.filename]=common._hash_entry(z,info.filename);compressed[info.filename]=compressed_hash(stream,info);copy_original_record(stream,target,info)
     for name in set(payload)-set(z.namelist()):
      if a.font_fallback_overlay and name.startswith(font_fallback.PREFIX):
       # New split parts inherit the original part's APK storage method.
       info=copy.copy(z.getinfo(font_fallback.PREFIX+"10"));info.filename=name;info.orig_filename=name
       target.writestr(info,payload[name])
      else:target.writestr(name,payload[name],compress_type=zipfile.ZIP_DEFLATED)
   common.run([sdk/"zipalign","-f","-p","4",unsigned,aligned],env);unsigned.unlink()
   common.run([sdk/"apksigner","sign","--ks",ROOT/".local-signing/debug.keystore","--ks-pass","pass:android","--key-pass","pass:android","--out",signed,aligned],env);aligned.unlink()
   require(common._certificate(common.run([sdk/"apksigner","verify","--verbose","--print-certs",signed],env))==cert,"Signer differs");common.run([sdk/"zipalign","-c","-p","4",signed],env)
   with zipfile.ZipFile(signed) as z,signed.open("rb") as stream:
    require(len(z.namelist())==len(set(z.namelist())) and {n for n in z.namelist()if not common._signature(n)}==set(inherited)|set(payload),"Entry inventory differs")
    for n,h in {**inherited,**hashes}.items():require(common._hash_entry(z,n)==h,"Entry differs: "+n)
    for n,h in compressed.items():require(compressed_hash(stream,z.getinfo(n))==h,"Compressed inherited entry differs: "+n)
   for f,h in pins.items():require(common.digest(f)==h,"Source changed while packaging")
   require(common.digest(base)==proof["baseAPKSHA256"],"Base changed while packaging")
   result={"status":"PASS_SIGN_ALIGN_SCOPED_OVERLAY_NOT_INSTALLED","binarySHA256":common.digest(signed),"baseAPKSHA256":proof["baseAPKSHA256"],"replacementEntries":hashes,"openingFontChanges":font_changes,"fontFallbackProof":fallback_report,"allOtherEntriesAndCompressedPayloadsExact":True,"managedAssemblyByteExact":common.DLL not in payload,"signatureVerified":True,"zipalignVerified":True,"originalEngineMethodsExact":547,"outputAPK":str(output.relative_to(ROOT))}
   signed.replace(output)
   if signed.with_suffix(".apk.idsig").exists():signed.with_suffix(".apk.idsig").replace(output.with_suffix(".apk.idsig"))
 record.parent.mkdir(parents=True,exist_ok=True);record.write_text(json.dumps(result,indent=2)+"\n");print(json.dumps({k:v for k,v in result.items() if k != "fontFallbackProof"}))
if __name__=="__main__":main()
