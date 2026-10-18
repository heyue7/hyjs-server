"""Build original guide restoration and bounded announcement/customer UI hooks."""
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
ENTRY="assets/bin/Data/Managed/Assembly-CSharp.dll"
TARGETS={"StaticData::get_UserName","StaticData::set_UserName","StaticData::get_PassWord","StaticData::set_PassWord","LoginUI::Init","UserInput::Show","UserInput::Hide","UserInput::OnCloseButtonClick","UserInput::OnRegistButtonClick","UserInput::OnLoginButton","LoginUI::ShowNormalUI","Login.LoginManager::InitServerInfo","LoginUI::OnSwitchAccountButtonClick","ManagerController::BackToLogin","Login.LoginUserLogin::UserLogin","Login.LoginUserLogin::UserLoginCallBack"}
def sha(raw):return hashlib.sha256(raw).hexdigest()
def verify(before,after,original=None):
    a,b=[dnfile.dnPE(data=raw) for raw in (before,after)]
    old,new=methods(a),methods(b);require(len(old)==len(new),"Method inventory changed")
    unchanged=engine=0;changes=[]
    for (name,rid,method),(other,oid,modified) in zip(old,new):
        require((name,rid)==(other,oid) and method.Signature.value==modified.Signature.value and method.struct.Flags==modified.struct.Flags and method.struct.ImplFlags==modified.struct.ImplFlags,"Inherited ABI changed: "+name)
        if not method.Rva:require(not modified.Rva,"Unexpected body");continue
        x,y=exact_body(a,method),exact_body(b,modified)
        if name not in TARGETS:
            require(x==y,"Non-target raw body changed: "+name);unchanged+=1;engine+=name.startswith("Battle.Engine")
        else:
            require(x!=y,"Target did not change: "+name)
            changes.append({"method":name,"beforeRawSHA256":sha(x),"afterRawSHA256":sha(y)})
    require(engine==547 and len(changes)==len(TARGETS),"Engine/target inventory differs")
    for i,row in enumerate(a.net.mdtables.Field):
        other=b.net.mdtables.Field.rows[i]
        require(str(row.Name)==str(other.Name) and row.Signature.value==other.Signature.value and row.struct.Flags==other.struct.Flags,"Inherited field changed")
    additions=[str(row.Name) for row in b.net.mdtables.Field.rows[len(a.net.mdtables.Field.rows):]]
    require(not additions,"Old Unity must retain original field table without FieldPtr")
    table={name:m for name,_,m in new}; original_table={name:m for name,_,m in old}
    def instructions(name,pe=b,inventory=table):return read_method_body_from_bytes(exact_body(pe,inventory[name])).instructions
    def symbol(pe,instruction):
        value=getattr(instruction.operand,"value",0);kind=value>>24
        return str(getattr(pe.net.mdtables,{4:"Field",6:"MethodDef",10:"MemberRef"}[kind]).rows[(value&0xffffff)-1].Name) if kind in {4,6,10} else ""
    callback=instructions("Login.LoginUserLogin::UserLoginCallBack")
    first_write=next(i for i,x in enumerate(callback) if x.opcode.name in {"stfld","stsfld"})
    first_literals=[b.net.user_strings.get(x.operand.value&0xffffff).value for x in callback[:first_write] if x.opcode.name=="ldstr"]
    require(first_literals[:2]==["com.hyjs.wallet.LoginCache","accepts"],"Callback must validate request before writes")
    oldcallback=instructions("Login.LoginUserLogin::UserLoginCallBack",a,original_table)
    require([symbol(a,x) for x in oldcallback if x.opcode.name=="stfld"]==["playerId","isFirstCreate"],"Original callback shared-data writes changed")
    require([symbol(a,x) for x in oldcallback if x.opcode.name=="ldfld"]==["kBody","manager","manager","manager","playerId"],"Original callback instance-read contract changed")
    start=instructions("Login.LoginUserLogin::UserLogin")
    clone=next(i for i,x in enumerate(start) if symbol(b,x)=="MemberwiseClone")
    delegate=next(i for i,x in enumerate(start) if x.opcode.name=="ldftn")
    require(clone<delegate and start[delegate-1].opcode.name.startswith("ldloc"),"Original delegate must bind immutable attempt snapshot")
    for name in ["StaticData::get_UserName","StaticData::get_PassWord","StaticData::set_UserName","StaticData::set_PassWord"]:
        require(not any(symbol(b,x) in {"GetString","SetString","GetInt","SetInt"} for x in instructions(name)),"Credential accessor must not use PlayerPrefs")
    require(not any(symbol(b,x)=="Log" for x in start),"Login packet must never log authCode")
    return {"originalEngineMethodsUnchanged":engine,"unchangedInheritedRawMethodBodies":unchanged,"changes":changes,"addedPrivateFields":additions,"allInheritedNonTargetMethodsExact":True,"callbackAttemptGuardBeforeStateWrites":True,"originalDelegateUsesIndependentAttemptSnapshot":True,"originalCallbackInstanceAccessReviewed":True,"legacyCredentialAccessorsMemoryOnly":True,"originalLoginPasswordLogAbsent":True}

def main():
    import os
    from tools.local_wallet.verify import dex_classes
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk",type=Path,required=True)
    parser.add_argument("--base-sha256",required=True)
    parser.add_argument("--output-directory",type=Path,required=True)
    parser.add_argument("--reuse-managed",type=Path,help="Reuse a same-source partial managed output; fresh strict comparison still runs")
    args=parser.parse_args()
    require(sha(args.base_apk.read_bytes())==args.base_sha256,"Base APK SHA differs")
    output=args.output_directory.resolve();require(not output.exists(),"Use a fresh output directory");output.mkdir(parents=True)
    with zipfile.ZipFile(args.base_apk) as apk:
        before=apk.read(ENTRY);base_dex=apk.read("classes2.dex")
    with zipfile.ZipFile(ROOT/"apks/original/10056262.apk") as apk: original=apk.read(ENTRY)
    injector=Path(__file__).with_name("Injector.cs")
    source=Path(__file__).parent/"java/com/hyjs/wallet/LoginCache.java"
    work=ROOT/".tools/native-batch";dotnet=work/"dotnet/dotnet";dnlib=work/"injector/bin/Debug/net8.0/dnlib.dll"
    def run(name,command,env=None):
        result=subprocess.run(["rtk","proxy",*map(str,command)],cwd=ROOT,capture_output=True,text=True,timeout=90,env=env)
        (output/(name+".log")).write_text(result.stdout+result.stderr)
        require(result.returncode==0,name+" failed; see saved log")
    with tempfile.TemporaryDirectory(prefix="hyjs-registration-") as temporary:
        folder=Path(temporary);current=folder/"Assembly-CSharp.dll";current.write_bytes(before);original_file=folder/"original.dll";original_file.write_bytes(original)
        shutil.copy2(injector,folder/"Program.cs");project=folder/"Patch.csproj"
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        if args.reuse_managed:
            verify(before,args.reuse_managed.read_bytes(),original)
            shutil.copy2(args.reuse_managed,output/"Assembly-CSharp.dll")
        else:
            run("compile-injector",[dotnet,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"])
            run("patch",[dotnet,folder/"bin/Debug/net8.0/Patch.dll",current,output/"Assembly-CSharp.dll"])
        sdk=Path.home()/"Library/Android/sdk";jdk=Path("/opt/homebrew/opt/openjdk@21");android=sdk/"platforms/android-33/android.jar"
        env=dict(os.environ,JAVA_HOME=str(jdk),PATH=str(jdk/"bin")+os.pathsep+os.environ.get("PATH",""))
        classes=folder/"classes";dex=folder/"dex";classes.mkdir();dex.mkdir();old_dex=folder/"old-wallet.dex";old_dex.write_bytes(base_dex)
        run("compile-java",[jdk/"bin/javac","-encoding","UTF-8","--release","8","-classpath",android,"-d",classes,source],env)
        run("dex-merge",[sdk/"cmdline-tools/latest/bin/d8","--release","--min-api","23","--lib",android,"--output",dex,old_dex,*sorted(classes.rglob("*.class"))],env)
        require(sorted(p.name for p in dex.iterdir())==["classes.dex"],"Expected one isolated wallet/account dex")
        (output/"classes2.dex").write_bytes((dex/"classes.dex").read_bytes())
    checks=verify(before,(output/"Assembly-CSharp.dll").read_bytes(),original)
    oldclasses=set(dex_classes(base_dex));newclasses=set(dex_classes((output/"classes2.dex").read_bytes()))
    require(oldclasses<newclasses and all(n.startswith("Lcom/hyjs/wallet/LoginCache") for n in newclasses-oldclasses),"Unexpected dex class changes")
    report={"schema":"hyjs-keystore-login-cache-overlay-v1","status":"STATIC_VERIFIED_NOT_PACKAGED","baseAPKSHA256":args.base_sha256,"originalManagedSHA256":sha(original),"inputManagedSHA256":sha(before),"outputManagedSHA256":sha((output/"Assembly-CSharp.dll").read_bytes()),"inputDexSHA256":sha(base_dex),"outputDexSHA256":sha((output/"classes2.dex").read_bytes()),"sourcePins":{str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in [Path(__file__).resolve(),source,injector]},"newDexClasses":sorted(newclasses-oldclasses),"inheritedWalletAndRegistrationDexIsD8Input":True,"walletSourceRecompiled":False,"modernIMEOriginalClassesDexMustRemainExact":True,"phoneExecuted":False,"APKPackaged":False,**checks}
    (output/"manifest.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print(json.dumps({"status":report["status"],"outputManagedSHA256":report["outputManagedSHA256"],"outputDexSHA256":report["outputDexSHA256"],"engineExact":checks["originalEngineMethodsUnchanged"]}))


if __name__=="__main__": main()
