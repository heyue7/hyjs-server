"""Patch only the king selector description cells in the exact official 78 APK."""
import argparse, hashlib, json, sys, zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT))
from tools.client_feedback78 import build_os_selector_text as cells
BASE_SHA="fe67aea7d584199119a04f10edcd8baa54207a70717e295d377ce3dd67b8affc"
OLD="可选择一名王者角色：双刀桐人、ALO桐人、亚丝娜、诗乃。"
SOURCE=ROOT/"hyjs-server/service/data/growth74_resources.json"
SPECS=[("tableitem","itemId","item_9761","intro"),("tabletext","id","desc_item_9761","text")]
def digest(p):
 with Path(p).open("rb")as f:return hashlib.file_digest(f,"sha256").hexdigest()
def patched(raw,key,identity,column,description):
 old=cells.OLD_DESCRIPTION
 try:
  cells.OLD_DESCRIPTION=OLD
  return cells.patch(raw,key,identity,column,description)
 finally:cells.OLD_DESCRIPTION=old
def prepare(base,out):
 assert digest(base)==BASE_SHA and not out.exists();out.mkdir(parents=True)
 source=json.loads(SOURCE.read_text());item=next(x for x in source["items"]if x["itemId"]=="item_9761");uses=[x for x in source["uses"]if x["itemId"]=="item_9761"]
 proofs={};files={}
 with zipfile.ZipFile(base)as apk:
  actual=[x for x in cells.rows(apk.read("assets/android/tabledata/tableitemuse"))if x["itemId"]=="item_9761"]
  fields=("id","itemId","result","odds","category");assert [{k:x[k]for k in fields}for x in uses]==[{k:x[k]for k in fields}for x in actual]
  heroes={x["id"]:x for x in cells.rows(apk.read("assets/android/tabledata/tablehero"))};texts={x["id"]:x["text"]for x in cells.rows(apk.read("assets/android/tabledata/tabletext"))}
  names=[texts[heroes[x["result"].split(",")[0]]["name"]]for x in actual];description="可选择一名王者角色："+"、".join(names)+"。"
  assert item["desc"]==item["intro"]==description
  for name,key,identity,column in SPECS:
   entry="assets/android/tabledata/"+name;raw,proof=patched(apk.read(entry),key,identity,column,description);(out/name).write_bytes(raw);files[entry]=str((out/name).relative_to(ROOT));proofs[entry]=proof
  main=apk.read("assets/bin/Data/Managed/Assembly-CSharp.dll");dex=apk.read("classes2.dex")
 report={"schema":"hyjs-king-selector86-description-v1","baseAPKSHA256":BASE_SHA,"description":description,"optionNames":names,"payloadFiles":files,"proofs":proofs,"sourcePins":{str(p.relative_to(ROOT)):digest(p)for p in [Path(__file__).resolve(),SOURCE,Path(cells.__file__)]}}
 (out/"manifest.json").write_text(json.dumps(report,ensure_ascii=False,indent=2))
 proof={"schema":"hyjs-feedback86-description-only-v1","managedOnly":True,"inheritedJavaByteExact":True,"inheritedJavaPublicProtectedABIExact":True,"managedAssemblyByteExact":True,"baseAPKSHA256":BASE_SHA,"inputManagedSHA256":hashlib.sha256(main).hexdigest(),"inputDexSHA256":hashlib.sha256(dex).hexdigest(),"sourcePins":report["sourcePins"],"payloadFiles":{}}
 (out/"managed-proof").mkdir();(out/"managed-proof/manifest.json").write_text(json.dumps(proof,indent=2));print(json.dumps({"prepared":True,"description":description,"actualOptionNames":names},ensure_ascii=False))
def apply_overlay(base,out,payload,pins):
 report=json.loads((out/"manifest.json").read_text());assert report["schema"]=="hyjs-king-selector86-description-v1" and report["baseAPKSHA256"]==digest(base)==BASE_SHA
 for f,h in report["sourcePins"].items():assert digest(ROOT/f)==h;pins[ROOT/f]=h
 with zipfile.ZipFile(base)as apk:
  for name,key,identity,column in SPECS:
   entry="assets/android/tabledata/"+name;raw,proof=patched(apk.read(entry),key,identity,column,report["description"]);assert proof==report["proofs"][entry] and raw==(out/name).read_bytes() and entry not in payload;payload[entry]=raw;pins[out/name]=digest(out/name)
 pins[out/"manifest.json"]=digest(out/"manifest.json");return {"description":report["description"],"optionNames":report["optionNames"],"otherCellsExact":True}
if __name__=="__main__":
 p=argparse.ArgumentParser();p.add_argument("--base-apk",type=Path,required=True);p.add_argument("--output-directory",type=Path,required=True);a=p.parse_args();prepare(a.base_apk.resolve(),a.output_directory.resolve())
