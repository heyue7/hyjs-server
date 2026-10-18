"""Prepare the approved remote endpoint overlay without changing login or game behavior."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods,operand
from tools.local_asset_ready.remove_gate import exact_body,require
from tools.client_remote.config import load_origin, validate_origin

BASE="d1ec347effaa84356133b7a81532fc79e79031983cbff6ce59e095ddb31fe3cb"
ENTRY="assets/bin/Data/Managed/Assembly-CSharp.dll"
WALLET="assets/local-wallet.json"
def targets_for(origin):
    origin = validate_origin(origin)
    return {"ServerConfig::GetServerUrl":("http://127.0.0.1:8088/sao/",origin+"/sao/"),
         "Battle.FieldPVEWorldBossBattleOver::End":("http://127.0.0.1:8088",origin),
         "Room.RoomNetField::SCCurrentPosition":("http://127.0.0.1:8088",origin),
         "VoicePlugin::InitGVoice":("http://127.0.0.1:8088",origin)}

def sha(raw):return hashlib.sha256(raw).hexdigest()

def verify(before,after,origin):
    targets = targets_for(origin)
    a,b=[dnfile.dnPE(data=raw) for raw in (before,after)]
    left,right=methods(a),methods(b)
    require(len(left)==len(right),"Method count changed")
    for name in ["Field","FieldPtr","MethodPtr","TypeDef"]:
        x,y=getattr(a.net.mdtables,name,None),getattr(b.net.mdtables,name,None)
        require((len(x.rows) if x else 0)==(len(y.rows) if y else 0),"Metadata inventory changed: "+name)
    require(not getattr(b.net.mdtables,"FieldPtr",None),"Old Unity must not gain FieldPtr")
    unchanged=engine=0;changes=[]
    for (name,rid,method),(other,oid,modified) in zip(left,right):
        require((name,rid)==(other,oid) and method.Signature.value==modified.Signature.value and method.struct.Flags==modified.struct.Flags and method.struct.ImplFlags==modified.struct.ImplFlags,"Inherited ABI changed: "+name)
        if not method.Rva:require(not modified.Rva,"Unexpected managed body");continue
        x,y=exact_body(a,method),exact_body(b,modified)
        if name not in targets:
            require(x==y,"Unrelated raw body changed: "+name);unchanged+=1;engine+=name.startswith("Battle.Engine");continue
        old,new=[read_method_body_from_bytes(raw) for raw in (x,y)]
        require(len(old.instructions)==len(new.instructions) and getattr(old.local_var_sig_tok,"value",None)==getattr(new.local_var_sig_tok,"value",None) and old.max_stack==new.max_stack,"Target method contract changed: "+name)
        replacements=0
        # Addresses and method sizes stay fixed: only the operand of one ldstr changes.
        for first,second in zip(old.instructions,new.instructions):
            require((first.offset,first.opcode.name)==(second.offset,second.opcode.name),"Target opcode/layout changed")
            if first.opcode.name=="ldstr":
                oldvalue=a.net.user_strings.get(first.operand.value&0xffffff).value
                newvalue=b.net.user_strings.get(second.operand.value&0xffffff).value
                if oldvalue==targets[name][0]:require(newvalue==targets[name][1],"Remote origin mismatch");replacements+=1
                else:require(oldvalue==newvalue and operand(first.operand)==operand(second.operand),"Unrelated string changed")
            else:require(operand(first.operand)==operand(second.operand),"Unrelated operand/branch changed")
        require(replacements==1,"Expected exactly one remote origin per target")
        def regions(body):return [(h.exception_type,h.try_start,h.try_end,h.handler_start,h.handler_end,h.filter_start,getattr(h.catch_type,"value",None)) for h in body.exception_handlers]
        require(regions(old)==regions(new),"Target exception regions changed")
        changes.append({"method":name,"beforeRawSHA256":sha(x),"afterRawSHA256":sha(y),"oldOrigin":targets[name][0],"remoteOrigin":targets[name][1],"onlyOneStringOperandChanged":True})
    require(engine==547 and len(changes)==4,"Target/engine inventory mismatch")
    for _,_,method in right:
        if not method.Rva:continue
        for instruction in read_method_body_from_bytes(exact_body(b,method)).instructions:
            if instruction.opcode.name=="ldstr":
                value=b.net.user_strings.get(instruction.operand.value&0xffffff).value
                require(origin == "http://127.0.0.1:8088" or "127.0.0.1:8088" not in value,"Executable stale API origin remains")
    return {"originalEngineMethodsUnchanged":engine,"unchangedInheritedRawMethodBodies":unchanged,"allInheritedNonTargetMethodsExact":True,"changes":changes,"onlyFourHTTPOriginLiteralsChanged":True,"noManagedTypesMethodsFieldsAdded":True,"noExecutableStaleAPIOriginRemaining":True}

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk",type=Path,default=ROOT/"build/hyjs-local.apk")
    parser.add_argument("--output-directory",type=Path,required=True)
    parser.add_argument("--origin", help="HTTP(S) API origin; defaults to HYJS_API_ORIGIN or private .env")
    args=parser.parse_args();origin=load_origin(args.origin);require(sha(args.base_apk.read_bytes())==BASE,"Current qualified d1ec base required")
    output=args.output_directory.resolve();require(not output.exists(),"Use a fresh output directory");output.mkdir(parents=True)
    with zipfile.ZipFile(args.base_apk) as apk:before=apk.read(ENTRY);wallet_before=apk.read(WALLET)
    wallet=json.loads(wallet_before);require(wallet=={"origin":"http://127.0.0.1:8088"},"Base wallet config changed");wallet["origin"]=origin
    wallet_after=(json.dumps(wallet,separators=(",",":"))+"\n").encode();(output/"local-wallet.json").write_bytes(wallet_after)
    runtime=ROOT/".tools/native-batch";dotnet=runtime/"dotnet/dotnet";dnlib=runtime/"injector/bin/Debug/net8.0/dnlib.dll";injector=Path(__file__).with_name("Injector.cs")
    def run(name,command):
        result=subprocess.run(["rtk","proxy",*map(str,command)],cwd=ROOT,capture_output=True,text=True,timeout=90);(output/(name+".log")).write_text(result.stdout+result.stderr);require(result.returncode==0,name+" failed; see saved log")
    with tempfile.TemporaryDirectory(prefix="hyjs-remote-overlay-") as temporary:
        folder=Path(temporary);current=folder/"Assembly-CSharp.dll";current.write_bytes(before);shutil.copy2(injector,folder/"Program.cs")
        project=folder/"Patch.csproj";project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run("compile-injector",[dotnet,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"])
        run("patch",[dotnet,folder/"bin/Debug/net8.0/Patch.dll",current,origin,output/"Assembly-CSharp.dll"])
    after=(output/"Assembly-CSharp.dll").read_bytes();checks=verify(before,after,origin)
    result={"schema":"hyjs-remote-origins-overlay-v1","status":"STATIC_VERIFIED_NOT_PACKAGED","baseAPKSHA256":BASE,"origin":origin,"inputManagedSHA256":sha(before),"outputManagedSHA256":sha(after),"inputWalletConfigSHA256":sha(wallet_before),"outputWalletConfigSHA256":sha(wallet_after),"sourcePins":{str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in [Path(__file__).resolve(),injector,Path(__file__).with_name("config.py")]},"walletAndRegistrationAndFeedbackJavaUnchanged":True,"phoneExecuted":False,**checks}
    (output/"manifest.json").write_text(json.dumps(result,ensure_ascii=False,indent=2)+"\n");print(json.dumps({"status":result["status"],"outputManagedSHA256":result["outputManagedSHA256"],"origin":origin}))

if __name__=="__main__":main()
