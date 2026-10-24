"""Package qualified native/runtime overlays with explicit managed method boundaries."""
import argparse,copy,fcntl,hashlib,json,os,re,shutil,sys,tempfile,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools import build_wallet_client as common
from tools.client_branding.package_release import compressed_hash,copy_original_record,require
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body
import dnfile

def apply_compatibility_overlay(base,directory,payload,pins):
 """Reproduce reviewed compatibility bytes before the packaging lock/signing stage.

 Version changes already in payload precede the compatibility AXML edit. This
 function performs read-only validation and mutates only the in-memory payload.
 """
 from tools.client_compat63.axml import patch as patch_axml
 from tools.client_compat63.dex_patch import patch as patch_dex,extract_pins,layout
 from tools.client_compat63.build import dex_classes,BASE_SHA
 directory=directory.resolve()
 require(directory.is_relative_to(ROOT),"Compatibility overlay outside project")
 manifest_file=directory/"manifest.json"
 module=json.loads(manifest_file.read_text())
 require(module.get("schema")=="hyjs-client-compat63-overlay-v1" and module.get("status")=="STATIC_DEX_AND_HOST_PROBE_PASS_NOT_PACKAGED_NOT_DEVICE_TESTED","Unqualified compatibility overlay")
 require(module["baseAPKSHA256"]==BASE_SHA==common.digest(base),"Compatibility formal62 input differs")
 expected_replacements={"AndroidManifest.xml","classes2.dex","assets/local-resource-overrides.json"}
 require(set(module["replacements"])==expected_replacements and set(module["additions"])=={"classes5.dex"},"Compatibility payload scope differs")
 require("classes2.dex" not in payload and "classes5.dex" not in payload and "assets/local-resource-overrides.json" not in payload,"Compatibility overlaps another Java/resource overlay")
 local={}
 for name,entry in {**module["replacements"],**module["additions"]}.items():
  file=(directory/entry["file"]).resolve()
  require(file.is_relative_to(directory) and file.is_file() and common.digest(file)==entry["sha256"],"Compatibility payload pin differs: "+name)
  local[name]=file.read_bytes()
 require(bool(module.get("sourcePins")),"Compatibility source pins missing")
 for name,sha in module["sourcePins"].items():
  file=(ROOT/name).resolve();require(file.is_relative_to(ROOT) and common.digest(file)==sha,"Compatibility source pin differs: "+name)
  require(file not in pins or pins[file]==sha,"Conflicting compatibility source pin");pins[file]=sha
 for name in ("tools/client_compat63/axml.py","tools/client_compat63/dex_patch.py","tools/client_compat63/build.py","tools/client_branding/branding.py"):
  file=ROOT/name;pins[file]=common.digest(file)
 pins[manifest_file]=common.digest(manifest_file)
 with zipfile.ZipFile(base) as archive:
  names=archive.namelist();require(len(names)==len(set(names)),"Duplicate base ZIP entries")
  original_dex={name:common._hash_entry(archive,name) for name in names if re.fullmatch(r"classes(?:[2-9][0-9]*)?\.dex",name)}
  require(original_dex==module["originalDexSHA256"] and set(original_dex)=={"classes.dex","classes2.dex","classes3.dex","classes4.dex"},"Compatibility original DEX pins/layout differ")
  require("classes5.dex" not in names,"Compatibility additive DEX already exists")
  original_config=json.loads(archive.read("assets/local-resource-overrides.json"))
  config=json.loads(local["assets/local-resource-overrides.json"])
  require(extract_pins(archive.read("classes2.dex"))==original_config["files"],"Original DEX/config pin contract differs")
  expected_config=copy.deepcopy(original_config)
  manifest_asset=archive.read("assets/android/smallassetmanifest")
  manifest_sha=hashlib.sha256(manifest_asset).hexdigest()
  targets=[row for row in expected_config["files"] if row["path"]=="smallassetmanifest"]
  require(len(targets)==1 and len(expected_config["files"])==6,"Original six-pin whitelist differs")
  targets[0].update(sha256=manifest_sha,bytes=len(manifest_asset))
  require(config==expected_config,"Compatibility JSON changes outside one manifest Pin")
  rebuilt,dex_proof=patch_dex(archive.read("classes2.dex"),manifest_sha,len(manifest_asset))
  require(rebuilt==local["classes2.dex"] and dex_proof==module["dexProof"],"Compatibility DEX replay/proof differs")
  parsed=layout(local["classes2.dex"])
  require(len(parsed["codes"])==475 and dex_proof["nonClinitMethodsInstructionsEHAndDebugExact"]==473,"Compatibility method scope differs")
  actual_pins=extract_pins(local["classes2.dex"])
  require(actual_pins==config["files"],"Patched DEX/config pins differ")
  assets={}
  for row in actual_pins:
   name="assets/android/"+row["path"]
   require(name in names,"Missing pinned APK asset")
   value=payload.get(name,archive.read(name));sha=hashlib.sha256(value).hexdigest()
   require(len(value)==row["bytes"] and sha==row["sha256"],"Actual final resource asset differs: "+row["path"])
   assets[name]={"bytes":len(value),"sha256":sha}
  untouched={name:common._hash_entry(archive,name) for name in names if name.startswith("lib/") or name.startswith("assets/bin/Data/Managed/")}
  require(untouched==module["unchangedNativeAndManagedEntrySHA256"],"Compatibility native/managed input pins differ")
  require(not any(name.startswith("lib/") for name in payload),"Compatibility release must preserve every native ABI entry")
  base_manifest=archive.read("AndroidManifest.xml")
  reproduced,base_manifest_proof=patch_axml(base_manifest)
  require(reproduced==local["AndroidManifest.xml"] and base_manifest_proof==module["manifestProof"],"Compatibility base Manifest replay differs")
  revised,final_manifest_proof=patch_axml(payload.get("AndroidManifest.xml",base_manifest))
  added=dex_classes(local["classes5.dex"])
  require(added==module["newDexClasses"] and bool(added) and all(name.startswith("Lcom/hyjs/compat/") for name in added),"Compatibility new DEX classes differ")
 # All validation precedes mutation: callers never receive a partial compatibility layer.
 payload.update({name:value for name,value in local.items() if name!="AndroidManifest.xml"})
 payload["AndroidManifest.xml"]=revised
 return {"schema":module["schema"],"overlayManifestSHA256":common.digest(manifest_file),"sourcePinsVerified":True,
  "baseAPKSHA256":BASE_SHA,"inputDEXSHA256":original_dex,"outputClasses2SHA256":hashlib.sha256(rebuilt).hexdigest(),
  "addedClasses5SHA256":hashlib.sha256(local["classes5.dex"]).hexdigest(),"actualResourcePins":actual_pins,
  "actualFinalResourceAssets":assets,"dexReplayProof":dex_proof,"manifestProof":final_manifest_proof,
  "newJavaClasses":added,"inheritedJavaByteExact":False,"allOtherOriginalDEXEntriesByteExact":True,
  "nonClinitInstructionsEHDebugExact":473,"allOriginalJavaAPIAndFlagsExact":True,"allNativeABIEntriesByteExact":True,
  "javaChangeScope":"classes2 LocalAssetOverlay.clinit manifest Pin + absolute data relocation; additive classes5 bootstrap; other DEX bytes exact",
  "deviceTested":False}

def verify_java_class_inventory(archive,payload):
 """Include new DEX entries as well as replacements in duplicate/final-parent checks."""
 from tools.login_ui.build import dex_class_headers
 names={name for name in set(archive.namelist())|set(payload) if re.fullmatch(r"classes(?:[2-9][0-9]*)?\.dex",name)}
 headers={}
 for name in sorted(names):
  current=dex_class_headers(payload[name] if name in payload else archive.read(name))
  require(not(set(headers)&set(current)),"Duplicate Java class across DEX entries")
  headers.update(current)
 for name,(flags,parent) in headers.items():
  require(parent not in headers or not headers[parent][0]&16,"Java superclass is final: "+name+" -> "+str(parent))
 return {"DEXEntries":sorted(names),"classCount":len(headers),"noDuplicateClass":True,"noFinalSuperclass":True}

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument("--base-apk",type=Path,required=True);p.add_argument("--overlay",type=Path,required=True);p.add_argument("--output",type=Path,required=True);p.add_argument("--record",type=Path,required=True);p.add_argument("--opening-font-overlay",type=Path);p.add_argument("--font-fallback-overlay",type=Path);p.add_argument("--dungeon-resource-overlay",type=Path);p.add_argument("--compat-overlay",type=Path,help="Reviewed formal62 startup/resource Pin compatibility overlay; composed after version patch");p.add_argument("--qa-wallet-origin");p.add_argument("--build-code",type=int);p.add_argument("--version-name");a=p.parse_args()
 require((a.build_code is None)==(a.version_name is None),"Build code and version name must be supplied together")
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
 if a.build_code is not None:
  from tools.client_update.package import patch_manifest
  require(a.build_code>0 and bool(a.version_name.strip()),"Invalid Android release version")
  with zipfile.ZipFile(base) as z:payload["AndroidManifest.xml"]=patch_manifest(z.read("AndroidManifest.xml"),a.build_code,a.version_name)
  pins[ROOT/"tools/client_update/package.py"]=common.digest(ROOT/"tools/client_update/package.py")
 pins[Path(__file__).resolve()]=common.digest(Path(__file__).resolve())
 if a.qa_wallet_origin:
  require(proof.get("qaOnly") is True and proof.get("neverPromote") is True and proof.get("qaOrigin")==a.qa_wallet_origin=="http://127.0.0.1:28979","Unqualified QA wallet origin")
  payload["assets/local-wallet.json"]=json.dumps({"origin":a.qa_wallet_origin},separators=(",",":")).encode()
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
 dungeon_report=None
 dungeon_infos={}
 if a.dungeon_resource_overlay:
  from tools.client_dungeon_access import build_resources as dungeon_resources
  directory=a.dungeon_resource_overlay.resolve();require(directory.is_relative_to(ROOT),"Dungeon overlay outside project")
  resource=json.loads((directory/"manifest.json").read_text());require(resource["baseAPKSHA256"]==proof["baseAPKSHA256"],"Dungeon resource base differs")
  expected,dependencies=dungeon_resources.verify(base,Path(resource["donorAPK"]))
  with zipfile.ZipFile(resource["donorAPK"]) as donor:
   dungeon_infos[dungeon_resources.ENTRY]=copy.copy(donor.getinfo(dungeon_resources.ENTRY))
  require(set(expected)==set(resource["payloadFiles"]),"Dungeon resource entries differ")
  for name,data in expected.items():
   require(name not in payload,"Overlapping dungeon payload")
   file=ROOT/resource["payloadFiles"][name];require(file.resolve().is_relative_to(directory)and file.read_bytes()==data,"Dungeon resource payload differs")
   require(hashlib.sha256(data).hexdigest()==resource["replacementEntries"][name],"Dungeon resource hash differs")
   payload[name]=data
  pins[Path(dungeon_resources.__file__).resolve()]=common.digest(Path(dungeon_resources.__file__))
  dungeon_report={"originalMapByteIdentical":True,"onlyRootDependencyDeclarationChanged":True,"previousDeclaredClosureBundles":len(dependencies),"entries":resource["replacementEntries"]}
 compatibility_report=None
 if a.compat_overlay:compatibility_report=apply_compatibility_overlay(base,a.compat_overlay,payload,pins)
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
    java_inventory=verify_java_class_inventory(z,payload)
    original_java_exact=all(name not in payload or hashlib.sha256(payload[name]).hexdigest()==common._hash_entry(z,name) for name in z.namelist() if re.fullmatch(r"classes(?:[2-9][0-9]*)?\.dex",name))
    java_byte_exact=original_java_exact and not any(re.fullmatch(r"classes(?:[2-9][0-9]*)?\.dex",name) and name not in z.namelist() for name in payload)
    require(common._hash_entry(z,common.DLL)==proof["inputManagedSHA256"],"Managed base differs")
    require(common._hash_entry(z,"classes2.dex")==proof["inputDexSHA256"],"Native base differs")
    if common.DLL in payload:
     before,after=[dnfile.dnPE(data=b)for b in (z.read(common.DLL),payload[common.DLL])]
     require(not getattr(after.net.mdtables,"FieldPtr",None) and not getattr(after.net.mdtables,"MethodPtr",None),"Legacy Unity pointer tables are forbidden")
     allowed=set(proof["allowedChangedMethods"]);changed=set();engine=0
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
      elif name in dungeon_infos:target.writestr(copy.copy(dungeon_infos[name]),payload[name])
      else:target.writestr(name,payload[name],compress_type=zipfile.ZIP_DEFLATED)
   common.run([sdk/"zipalign","-f","-p","4",unsigned,aligned],env);unsigned.unlink()
   common.run([sdk/"apksigner","sign","--ks",ROOT/".local-signing/debug.keystore","--ks-pass","pass:android","--key-pass","pass:android","--out",signed,aligned],env);aligned.unlink()
   require(common._certificate(common.run([sdk/"apksigner","verify","--verbose","--print-certs",signed],env))==cert,"Signer differs");common.run([sdk/"zipalign","-c","-p","4",signed],env)
   with zipfile.ZipFile(signed) as z,signed.open("rb") as stream:
    require(len(z.namelist())==len(set(z.namelist())) and {n for n in z.namelist()if not common._signature(n)}==set(inherited)|set(payload),"Entry inventory differs")
    for n,h in {**inherited,**hashes}.items():require(common._hash_entry(z,n)==h,"Entry differs: "+n)
    for n,h in compressed.items():require(compressed_hash(stream,z.getinfo(n))==h,"Compressed inherited entry differs: "+n)
    for name,info in dungeon_infos.items():require(z.getinfo(name).compress_type==info.compress_type,"Original bundle ZIP storage changed")
   for f,h in pins.items():require(common.digest(f)==h,"Source changed while packaging")
   require(common.digest(base)==proof["baseAPKSHA256"],"Base changed while packaging")
   result={"status":"PASS_SIGN_ALIGN_SCOPED_OVERLAY_NOT_INSTALLED","binarySHA256":common.digest(signed),"baseAPKSHA256":proof["baseAPKSHA256"],"replacementEntries":hashes,"openingFontChanges":font_changes,"fontFallbackProof":fallback_report,"dungeonResourceProof":dungeon_report,"compatibilityProof":compatibility_report,"javaClassInventory":java_inventory,"inheritedJavaByteExact":original_java_exact,"javaByteExact":java_byte_exact,"allOtherEntriesAndCompressedPayloadsExact":True,"managedAssemblyByteExact":common.DLL not in payload,"qaOnly":bool(a.qa_wallet_origin),"neverPromote":bool(a.qa_wallet_origin),"signatureVerified":True,"zipalignVerified":True,"originalEngineMethodsExact":547,"outputAPK":str(output.relative_to(ROOT))}
   if a.build_code is not None:result.update({"buildCode":a.build_code,"versionName":a.version_name})
   signed.replace(output)
   if signed.with_suffix(".apk.idsig").exists():signed.with_suffix(".apk.idsig").replace(output.with_suffix(".apk.idsig"))
 record.parent.mkdir(parents=True,exist_ok=True);record.write_text(json.dumps(result,indent=2)+"\n");print(json.dumps({k:v for k,v in result.items() if k != "fontFallbackProof"}))
if __name__=="__main__":main()
