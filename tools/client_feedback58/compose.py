"""Compose reviewed batch 55/56/57/58 overlays from their fixed input hashes."""
import argparse, hashlib, json, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path
import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_arena_state.build import methods, exact_body

MAIN = "assets/bin/Data/Managed/Assembly-CSharp.dll"
def sha(data): return hashlib.sha256(data).hexdigest()
def read(directory): return json.loads((directory / "manifest.json").read_text())
def require(value, message):
    if not value: raise ValueError(message)

def qa_main(before, base, origin, out):
    from tools.client_remote import build as remote
    with zipfile.ZipFile(base) as z: old_origin = json.loads(z.read("assets/local-wallet.json"))["origin"].rstrip("/")
    targets = {name:(old.replace("http://127.0.0.1:8088", old_origin), new)
               for name,(old,new) in remote.targets_for(origin).items()}
    remote.targets_for = lambda value: targets
    runtime = ROOT / ".tools/native-batch"
    with tempfile.TemporaryDirectory(prefix="hyjs58-qa-origin-") as t:
        work = Path(t); (work / "before.dll").write_bytes(before)
        (work / "Program.cs").write_text((ROOT / "tools/client_remote/Injector.cs").read_text().replace("http://127.0.0.1:8088", old_origin))
        project = work / "Patch.csproj"
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(runtime / 'injector/bin/Debug/net8.0/dnlib.dll')+'</HintPath></Reference></ItemGroup></Project>')
        for label,cmd in [("compile",[runtime / "dotnet/dotnet", "build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"]),("inject",[runtime / "dotnet/dotnet",work / "bin/Debug/net8.0/Patch.dll",work / "before.dll",origin,work / "after.dll"])]:
            result = subprocess.run(["rtk","proxy",*map(str,cmd)],capture_output=True,text=True,timeout=120)
            (out / ("qa-"+label+".log")).write_text(result.stdout+result.stderr)
            require(result.returncode == 0, "QA origin "+label+" failed")
        after = (work / "after.dll").read_bytes()
    return after, remote.verify(before,after,origin)

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--manifest-overlay",type=Path,required=True)
    p.add_argument("--output-directory",type=Path,required=True)
    p.add_argument("--qa-origin")
    p.add_argument("--selection-overlay",type=Path)
    a = p.parse_args(); source = Path(__file__).resolve().parent
    catalogue = json.loads((source / "catalogue.json").read_text())
    base = ROOT / catalogue["baseAPK"]; out = a.output_directory.resolve()
    require(out.is_relative_to(ROOT) and not out.exists(), "Fresh project output required")
    require(hashlib.file_digest(base.open("rb"),"sha256").hexdigest() == catalogue["baseAPKSHA256"], "Base APK differs")
    out.mkdir(parents=True)
    pins = {}; evidence = []; helpers = {}; allowed_tokens = set()
    with zipfile.ZipFile(base) as z: original = z.read(MAIN); original_dex = z.read("classes2.dex")
    current = original
    first = ROOT / catalogue["managedChain"][0]
    reference = json.loads((first / "verification.json").read_text())
    require(reference["inputSHA256"] == sha(current), "Newbie reference input differs")
    current = (first / "Assembly-CSharp.dll").read_bytes()
    require(reference["outputSHA256"] == sha(current), "Newbie reference output differs")
    for name in catalogue["managedChain"][1:] + [str(a.manifest_overlay.resolve().relative_to(ROOT))]:
        directory = ROOT / name; item = read(directory)
        require(item["baseAPKSHA256"] == catalogue["baseAPKSHA256"] and item["inputManagedSHA256"] == sha(current), "Managed overlay chain differs: "+name)
        current = (directory / "Assembly-CSharp.dll").read_bytes()
        require(item.get("outputManagedSHA256",item["replacementEntries"][MAIN]) == sha(current), "Managed overlay payload differs: "+name)
        pins.update(item.get("sourcePins",{}))
        if "sourceSHA256" in item: pins.update({"tools/client_bulk_purchase/"+file:digest for file,digest in item["sourceSHA256"].items()})
        for entry,digest in item["replacementEntries"].items():
            if entry == MAIN: continue
            file = directory / entry.rsplit("/",1)[-1]
            require(sha(file.read_bytes()) == digest, "Managed helper differs: "+entry); helpers[entry] = file
        evidence.append({"overlay":name,"manifestSHA256":sha((directory/"manifest.json").read_bytes())})
        if "changedMethodTokens" in item: allowed_tokens.update(int(token,16) for token in item["changedMethodTokens"])
        else:
            wanted = set(item["allowedChangedMethods"])
            for name2,rid,method in methods(dnfile.dnPE(data=original)):
                if name2 in wanted:
                    # RCAIMonster.Init has two overloads; the pinned dungeon list selects one.
                    if name2 == "Room.RCAIMonster::Init":
                        target = next(x for x in json.loads((ROOT / "tools/client_dungeon_access/targets.json").read_text())["targets"] if x.get("fullName") == name2)
                        if rid != (int(target["token"],16)&0xffffff): continue
                    allowed_tokens.add(0x06000000|rid)
    for key in ["ggoOverlay"]:
        directory = ROOT / catalogue[key]; item = read(directory); pins.update(item["sourcePins"])
        for entry,file in item["payloadFiles"].items():
            path = ROOT / file; require(sha(path.read_bytes()) == item["replacementEntries"][entry],"Helper differs"); helpers[entry] = path
    newbie = ROOT / "analysis/dungeon56-20261023/newbie-release-helper"; item = read(newbie)
    pins.update(item["sourcePins"]); file = newbie / "HYJS.NewbieSign.dll"
    require(sha(file.read_bytes()) == item["helperSHA256"],"Release newbie helper differs"); helpers["assets/bin/Data/Managed/HYJS.NewbieSign.dll"] = file
    native = ROOT / catalogue["javaOverlay"]; item = read(native); pins.update(item["sourcePins"])
    require(item["baseAPKSHA256"] == catalogue["baseAPKSHA256"] and item["inputDexSHA256"] == sha(original_dex) and item["inheritedJavaPublicProtectedABIExact"], "Native overlay base/ABI differs")
    dex = (native / "classes2.dex").read_bytes(); require(sha(dex) == item["outputDexSHA256"],"Native DEX differs")
    if a.selection_overlay:
        directory=a.selection_overlay.resolve(); item=read(directory)
        require(directory.is_relative_to(ROOT) and item['baseAPKSHA256']==catalogue['baseAPKSHA256'] and item['inputManagedSHA256']==sha(current),'Selection overlay input differs')
        current=(directory/'Assembly-CSharp.dll').read_bytes()
        require(sha(current)==item['outputManagedSHA256'],'Selection managed payload differs')
        allowed_tokens.update(int(token,16) for token in item['changedMethodTokens'])
        for entry,digest in item['replacementEntries'].items():
            if entry==MAIN: continue
            file=directory/entry.rsplit('/',1)[-1]
            require(sha(file.read_bytes())==digest,'Selection helper differs: '+entry); helpers[entry]=file
        pins.update({'tools/client_one_key59/'+file:digest for file,digest in item['sourceSHA256'].items()})
        pins['tools/client_recycle_selection/Selection.cs']=item['recycleSourceSHA256']
        evidence.append({'overlay':str(directory.relative_to(ROOT)),'manifestSHA256':sha((directory/'manifest.json').read_bytes())})
    qa_proof = None
    if a.qa_origin:
        require(a.qa_origin == "http://127.0.0.1:28979","Use the owned QA origin")
        before_qa = current; current,qa_proof = qa_main(current,base,a.qa_origin,out)
        before_rows,after_rows = methods(dnfile.dnPE(data=before_qa)),methods(dnfile.dnPE(data=current))
        lp,rp = dnfile.dnPE(data=before_qa),dnfile.dnPE(data=current)
        for (name,rid,m),(_,_,n) in zip(before_rows,after_rows):
            if m.Rva and exact_body(lp,m) != exact_body(rp,n): allowed_tokens.add(0x06000000|rid)
        pins["tools/client_remote/Injector.cs"] = sha((ROOT / "tools/client_remote/Injector.cs").read_bytes())
        pins["tools/client_remote/build.py"] = sha((ROOT / "tools/client_remote/build.py").read_bytes())
    left,right = dnfile.dnPE(data=original),dnfile.dnPE(data=current)
    changed = []; actual_tokens = set(); engine = 0
    old,new = methods(left),methods(right); require(len(old)==len(new),"Main method count differs")
    for (name,rid,m),(other,rid2,n) in zip(old,new):
        require((name,rid,m.Signature.value)==(other,rid2,n.Signature.value),"Main ABI differs")
        if not m.Rva: continue
        if exact_body(left,m)!=exact_body(right,n): changed.append(name); actual_tokens.add(0x06000000|rid)
        elif name.startswith("Battle.Engine"): engine += 1
    require(actual_tokens==allowed_tokens and engine==547,"Combined managed boundary differs")
    require(not getattr(right.net.mdtables,"FieldPtr",None) and not getattr(right.net.mdtables,"MethodPtr",None),"Legacy Unity pointer tables are forbidden")
    require(not any(t.ResolutionScope.table.name=="ModuleRef" and str(t.ResolutionScope.row.Name).startswith(("HYJS.","SoulFlowerPayment")) for t in right.net.mdtables.TypeRef.rows),"Invalid helper-module type scope")
    if a.selection_overlay:
        host_types={(str(t.TypeNamespace),str(t.TypeName)) for t in right.net.mdtables.TypeDef.rows}
        require(not any(t.ResolutionScope.table.name=='ModuleRef' and (str(t.TypeNamespace),str(t.TypeName)) in host_types for t in right.net.mdtables.TypeRef.rows),'Host type imported through foreign module')
    for name,digest in pins.items(): require(sha((ROOT/name).read_bytes())==digest,"Source changed: "+name)
    for name in [str(Path(__file__).relative_to(ROOT)),str((source/"catalogue.json").relative_to(ROOT)),"tools/client_newbie_sign/RepairReferences.cs"]: pins[name]=sha((ROOT/name).read_bytes())
    (out/"Assembly-CSharp.dll").write_bytes(current); (out/"classes2.dex").write_bytes(dex)
    payload = {MAIN:str((out/"Assembly-CSharp.dll").relative_to(ROOT))}
    for entry,file in helpers.items(): payload[entry]=str(file.relative_to(ROOT))
    report = dict(schema="hyjs-feedback58-composed-v1",baseAPKSHA256=catalogue["baseAPKSHA256"],inputManagedSHA256=sha(original),outputManagedSHA256=sha(current),inputDexSHA256=sha(original_dex),outputDexSHA256=sha(dex),managedOnly=False,inheritedJavaPublicProtectedABIExact=True,payloadFiles=payload,replacementEntries={entry:sha((ROOT/file).read_bytes())for entry,file in payload.items()},sourcePins=pins,allowedChangedMethods=sorted(changed),originalEngineMethodsExact=engine,managedChain=evidence,qaOnly=bool(a.qa_origin),neverPromote=bool(a.qa_origin),qaOrigin=a.qa_origin,qaOriginProof=qa_proof)
    (out/"manifest.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print(json.dumps(dict(mainSHA256=sha(current),managedChanges=len(changed),engineMethods=engine,helpers=len(helpers),qaOnly=bool(a.qa_origin))))

if __name__=="__main__": main()
