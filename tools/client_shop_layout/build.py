"""Compile a Commerce66-only overlay, preserving Main and all purchase code."""
import sys
import dnfile
import argparse, hashlib, json, subprocess, tempfile, zipfile
from pathlib import Path
HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(HERE.parents[1]))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body
def sha(raw): return hashlib.sha256(raw).hexdigest()
def main():
 p=argparse.ArgumentParser();p.add_argument("--base-apk",type=Path,required=True);p.add_argument("--runtime-root",type=Path,required=True);p.add_argument("--output-directory",type=Path,required=True);a=p.parse_args()
 out=a.output_directory.resolve();assert not out.exists();out.mkdir(parents=True)
 runtime=a.runtime_root;dotnet=runtime/"dotnet/dotnet";csc=next((runtime/"dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
 entry="assets/bin/Data/Managed/HYJS.Commerce66.dll"
 with zipfile.ZipFile(a.base_apk) as z,tempfile.TemporaryDirectory(prefix="hyjs-shop-layout-") as tmp:
  refs=Path(tmp)
  for n in z.namelist():
   if n.startswith("assets/bin/Data/Managed/") and n.endswith(".dll"):(refs/Path(n).name).write_bytes(z.read(n))
  assert (refs/"HYJS.Commerce66.dll").exists(),"Input must already contain feedback66 quota hook"
  helper=out/"HYJS.Commerce66.dll"
  cmd=[dotnet,csc,"/nologo","/target:library","/nostdlib+","/langversion:3","/deterministic+","/codepage:65001","/out:"+str(helper),*["/r:"+str(x) for x in sorted(refs.glob("*.dll")) if x.name!=helper.name],HERE/"RuntimeCommerce.cs"]
  r=subprocess.run(["rtk","proxy",*map(str,cmd)],capture_output=True,text=True,timeout=120);(out/"compile.log").write_text(r.stdout+r.stderr);assert r.returncode==0,r.stdout+r.stderr
  old,new=dnfile.dnPE(data=z.read(entry)),dnfile.dnPE(data=helper.read_bytes())
  assert str(old.net.mdtables.Assembly.rows[0].Name)==str(new.net.mdtables.Assembly.rows[0].Name)
  pairs=list(zip(methods(old),methods(new)));assert len(pairs)==2
  for (name,rid,m),(name2,rid2,n) in pairs:
   assert (name,rid,m.Signature.value)==(name2,rid2,n.Signature.value)
   assert (exact_body(old,m)==exact_body(new,n))==(name.endswith("::Get"))
  report=dict(helperABIPreserved=True,onlyQuotaBodyChanged=True,schema="hyjs-shop-layout-overlay-v1",baseAPKSHA256=sha(a.base_apk.read_bytes()),inputManagedSHA256=sha(z.read("assets/bin/Data/Managed/Assembly-CSharp.dll")),inputHelperSHA256=sha(z.read(entry)),replacementEntries={entry:sha(helper.read_bytes())},payloadFiles={entry:str(helper)},mainAssemblyUnchanged=True,purchaseProtocolUnchanged=True,apkBuilt=False,phoneVerified=False,sourceSHA256=sha((HERE/"RuntimeCommerce.cs").read_bytes()))
  (out/"manifest.json").write_text(json.dumps(report,indent=2)+"\n");print(json.dumps(report))
if __name__=="__main__":main()
