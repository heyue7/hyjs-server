"""Compose two bounded view-controller guards on the final qualified managed DLL."""
import argparse,hashlib,json,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/"tools"))
from awake_diff_rows.build import methods,operand
from local_asset_ready.remove_gate import exact_body,require
TARGETS={"Battle.View.BattleViewCtrl::IsCanTransfer":6,"Battle.View.BattleViewCtrl::AskTransEnd":8}
ORIGINAL_TARGET_BODY_PINS={'Battle.View.BattleViewCtrl::IsCanTransfer': '075a9d7446b99ad8d9aa66a05da3157f430a2a8024e8d0d4f6e0e97619d71c5b', 'Battle.View.BattleViewCtrl::AskTransEnd': '8c8cb47d54c948cd64a69b7a85fec9f9ca1512bd63e55f3e58ccd411e53e099a'}
def sha(raw):return hashlib.sha256(raw).hexdigest()
def verify(before,after):
    a,b=dnfile.dnPE(data=before),dnfile.dnPE(data=after)
    old,new=methods(a),methods(b)
    require(len(old)==len(new),"Method inventory changed")
    unchanged=engine=0;changes=[]
    for (name,rid,m),(nn,nr,n) in zip(old,new):
        require((name,rid)==(nn,nr) and m.Signature.value==n.Signature.value and m.struct.Flags==n.struct.Flags and m.struct.ImplFlags==n.struct.ImplFlags,"ABI/token/flags changed: "+name)
        if not m.Rva: require(not n.Rva,"Body added: "+name);continue
        x,y=exact_body(a,m),exact_body(b,n)
        if name not in TARGETS:
            require(x==y,"Non-target raw header/IL/EH changed: "+name)
            unchanged+=1;engine+=name.startswith("Battle.Engine")
            continue
        ox,ny=read_method_body_from_bytes(x),read_method_body_from_bytes(y)
        require(not ox.exception_handlers and not ny.exception_handlers,"Guard EH differs")
        require(sha(x)==ORIGINAL_TARGET_BODY_PINS[name],"Original target body differs/already patched: "+name)
        count=TARGETS[name];prefix=ny.instructions[:count];suffix=ny.instructions[count:]
        require(len(suffix)==len(ox.instructions),"Original suffix length changed")
        oi={i.offset:k for k,i in enumerate(ox.instructions)};ni={i.offset:k for k,i in enumerate(suffix)}
        for oldi,newi in zip(ox.instructions,suffix):
            require(oldi.opcode.name==newi.opcode.name,"Original opcode changed: "+name)
            if oldi.opcode.flow_control.name in {"Branch","Cond_Branch"}:require(oi[oldi.operand]==ni[newi.operand],"Original branch changed")
            else:require(operand(oldi.operand)==operand(newi.operand),"Original operand changed")
        expected=["ldarg.0","ldfld","ldc.i4.1","ble","ldc.i4.0","ret"] if count==6 else ["ldarg.0","ldfld","ldc.i4.1","bgt","ldarg.0","ldfld","brfalse","ret"]
        require([i.opcode.name for i in prefix]==expected,"Unexpected guard prefix")
        require(prefix[3 if count==6 else 6].operand==suffix[0].offset,"Original allowed branch lost")
        if count==8:require(prefix[3].operand==prefix[7].offset,"Rejected submit does not return")
        changes.append(dict(method=name,beforeRawSHA256=sha(x),afterRawSHA256=sha(y),originalInstructionsAndBranchesPreserved=True))
    require(engine==547 and len(changes)==2,"Original Engine/target inventory differs")
    return dict(unchangedRawMethodBodies=unchanged,originalEngineMethodsUnchanged=engine,changes=changes)
def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument("--input-managed",type=Path,required=True);p.add_argument("--output-directory",type=Path,required=True);a=p.parse_args()
    out=a.output_directory.resolve();require(not out.exists(),"Use fresh output directory");out.mkdir(parents=True)
    before=a.input_managed.read_bytes();dotnet=ROOT/".tools/native-batch/dotnet/dotnet";dnlib=ROOT/".tools/native-batch/injector/bin/Debug/net8.0/dnlib.dll"
    with tempfile.TemporaryDirectory(prefix="hyjs-round-one-formation-") as temporary:
        folder=Path(temporary)
        for name in ["FormationGuardProgram.cs","FormationGuard.cs"]:shutil.copy2(Path(__file__).with_name(name),folder/name)
        (folder/"Patch.csproj").write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable><NoWarn>SYSLIB0050</NoWarn></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        def run(name,argv):
            r=subprocess.run(["rtk","proxy",*map(str,argv)],cwd=ROOT,capture_output=True,text=True,timeout=90)
            (out/(name+".log")).write_text(r.stdout+r.stderr);require(r.returncode==0,name+" failed; see saved log")
        run("compile",[dotnet,"build",folder/"Patch.csproj","--nologo","--verbosity","quiet","-p:NuGetAudit=false"])
        output=out/"Assembly-CSharp.dll"
        run("patch",[dotnet,folder/"bin/Debug/net8.0/Patch.dll",a.input_managed.resolve(),output])
        checks=verify(before,output.read_bytes())
        run("managed-guard-probe",[dotnet,folder/"bin/Debug/net8.0/Patch.dll","--probe",output,ROOT/"analysis/extracted/assets/bin/Data/Managed",out/"guard-states.txt"])
    sources=[Path(__file__).resolve(),Path(__file__).with_name("FormationGuard.cs"),Path(__file__).with_name("FormationGuardProgram.cs")]
    report=dict(schema="local-first-round-formation-view-guard-v1",status="PASS_OFFLINE_NOT_PACKAGED",inputManagedSHA256=sha(before),outputManagedSHA256=sha(output.read_bytes()),sourcePins={str(s.relative_to(ROOT)):sha(s.read_bytes())for s in sources},allowedChangedMethods=list(TARGETS),replacementEntries={"assets/bin/Data/Managed/Assembly-CSharp.dll":sha(output.read_bytes())},actualManagedGuardStates=16,rejectedSubmitsWithoutSceneOrEngine=12,original547EngineMethodsChanged=False,phoneExecuted=False,APKPackaged=False,**checks)
    (out/"manifest.json").write_text(json.dumps(report,indent=2)+"\n");print(json.dumps({k:v for k,v in report.items()if k not in ["changes","sourcePins"]}))
if __name__=="__main__":main()
