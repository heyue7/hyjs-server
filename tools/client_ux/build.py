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
GUIDES={"GuideManager::OnLogin","GuideManager::OnLevelUp","GuideManager::OnQuestComplete"}
PREFIXES={"UI.AnnouncePage::DoOpen","SystemAnnouncePage::Show"}
FEEDBACK="UI.KeFuPage::DoOpen"
ACHIEVEMENT="UI.AchievementPage::<CreateAchievementList>m__BE"
TARGETS=GUIDES|PREFIXES|{FEEDBACK,ACHIEVEMENT}
def sha(raw):return hashlib.sha256(raw).hexdigest()

def shape(body,omit):
    kept=[i for k,i in enumerate(body.instructions) if k not in omit]
    offsets={i.offset:k for k,i in enumerate(kept)}
    offsets[body.header_size+body.code_size]=len(kept)
    def index(offset):
        return offsets[next(k for k in sorted(offsets) if k>=offset)]
    result=[]
    for i in kept:
        value=i.operand
        if i.opcode.flow_control.name in {"Branch","Cond_Branch"}:
            value=tuple(index(x) for x in value) if i.opcode.name=="switch" else index(value)
        else:value=operand(value)
        result.append((i.opcode.name,value))
    handlers=[(h.exception_type,*[index(v+body.header_size) for v in (h.try_start,h.try_end,h.handler_start,h.handler_end)],h.filter_start if h.filter_start<0 else index(h.filter_start+body.header_size),getattr(h.catch_type,"value",None)) for h in body.exception_handlers]
    return result,handlers

def verify(before,after,original):
    a,b,o=[dnfile.dnPE(data=raw) for raw in (before,after,original)]
    old,new,orig=methods(a),methods(b),{n:m for n,_,m in methods(o)}
    require(len(old)==len(new),"Inherited method inventory changed")
    unchanged=engine=0;changes=[]
    for (name,rid,method),(other,oid,modified) in zip(old,new):
        require((name,rid)==(other,oid) and method.Signature.value==modified.Signature.value and method.struct.Flags==modified.struct.Flags and method.struct.ImplFlags==modified.struct.ImplFlags,"Inherited ABI changed: "+name)
        if not method.Rva:require(not modified.Rva,"Unexpected method body added");continue
        x,y=exact_body(a,method),exact_body(b,modified)
        if name not in TARGETS:
            require(x==y,"Non-target raw body changed: "+name);unchanged+=1;engine+=name.startswith("Battle.Engine");continue
        left,right=read_method_body_from_bytes(x),read_method_body_from_bytes(y)
        row={"method":name,"beforeRawSHA256":sha(x),"afterRawSHA256":sha(y)}
        if name in GUIDES:
            source=read_method_body_from_bytes(exact_body(o,orig[name]))
            require(shape(source,set())==shape(right,set()),"Restored original guide CIL/control flow/EH differs: "+name)
            require(source.max_stack==right.max_stack,"Restored guide max stack differs")
            require(bool(source.local_var_sig_tok)==bool(right.local_var_sig_tok),"Restored guide locals presence differs")
            if source.local_var_sig_tok:
                require(o.net.mdtables.StandAloneSig.rows[(source.local_var_sig_tok.value&0xffffff)-1].Signature.value==b.net.mdtables.StandAloneSig.rows[(right.local_var_sig_tok.value&0xffffff)-1].Signature.value,"Restored original guide local signature differs")
            row.update(originalInstructionAndExceptionRegionsExact=True,originalRawSHA256=sha(exact_body(o,orig[name])))
        elif name in PREFIXES:
            count=len(right.instructions)-len(left.instructions);require(count==8,"Unexpected announcement prefix size")
            require(shape(left,set())==shape(right,set(range(count))),"Original announcement flow changed")
            literals=[b.net.user_strings.get(i.operand.value&0xffffff).value for i in right.instructions[:count] if i.opcode.name=="ldstr"]
            require(literals==["com.hyjs.wallet.ClientFeedback","prepareNotices"],"Unexpected announcement hook")
            row.update(originalInstructionsAndExceptionRegionsPreserved=True,prefixInstructions=count)
        elif name==ACHIEVEMENT:
            count=len(right.instructions)-len(left.instructions);require(count==7,"Unexpected achievement empty notice size")
            begin=next(k for k,i in enumerate(right.instructions) if i.opcode.name=="ldstr" and b.net.user_strings.get(i.operand.value&0xffffff).value=="成就奖励待配置")-4
            require(shape(left,set())==shape(right,set(range(begin,begin+count))),"Original achievement callback/EH changed")
            row.update(originalInstructionsAndExceptionRegionsPreserved=True,emptyNotice="成就奖励待配置")
        else:
            require(len(right.instructions)==21 and not right.exception_handlers,"Unexpected feedback hook")
            literals=[b.net.user_strings.get(i.operand.value&0xffffff).value for i in right.instructions if i.opcode.name=="ldstr"]
            require(literals==["com.hyjs.wallet.ClientFeedback","show"],"Unexpected feedback entry")
            row.update(originalPageOpenRetained=True,oldRetiredWebViewReplaced=True)
        changes.append(row)
    require(engine==547 and len(changes)==len(TARGETS),"Engine/target inventory differs")
    return {"originalEngineMethodsUnchanged":engine,"unchangedInheritedRawMethodBodies":unchanged,"changes":changes,"allInheritedNonTargetMethodsExact":True,"originalTownGuideHandlersRestored":3,"allOtherTownGuideAndRecruitCompatibilityMethodsUnchanged":True}

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
    source=Path(__file__).parent/"java/com/hyjs/wallet/ClientFeedback.java"
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
            run("patch",[dotnet,folder/"bin/Debug/net8.0/Patch.dll",current,original_file,output/"Assembly-CSharp.dll"])
        sdk=Path.home()/"Library/Android/sdk";jdk=Path("/opt/homebrew/opt/openjdk@21");android=sdk/"platforms/android-33/android.jar"
        env=dict(os.environ,JAVA_HOME=str(jdk),PATH=str(jdk/"bin")+os.pathsep+os.environ.get("PATH",""))
        classes=folder/"classes";dex=folder/"dex";classes.mkdir();dex.mkdir();old_dex=folder/"old-wallet.dex";old_dex.write_bytes(base_dex)
        run("compile-java",[jdk/"bin/javac","-encoding","UTF-8","--release","8","-classpath",android,"-d",classes,source],env)
        run("dex-merge",[sdk/"cmdline-tools/latest/bin/d8","--release","--min-api","23","--lib",android,"--output",dex,old_dex,*sorted(classes.rglob("*.class"))],env)
        require(sorted(p.name for p in dex.iterdir())==["classes.dex"],"Expected one isolated wallet/account dex")
        (output/"classes2.dex").write_bytes((dex/"classes.dex").read_bytes())
    checks=verify(before,(output/"Assembly-CSharp.dll").read_bytes(),original)
    oldclasses=set(dex_classes(base_dex));newclasses=set(dex_classes((output/"classes2.dex").read_bytes()))
    require(oldclasses<newclasses and all(n.startswith("Lcom/hyjs/wallet/ClientFeedback") for n in newclasses-oldclasses),"Unexpected dex class changes")
    report={"schema":"hyjs-original-guides-notice-feedback-overlay-v1","status":"STATIC_VERIFIED_NOT_PACKAGED","baseAPKSHA256":args.base_sha256,"originalManagedSHA256":sha(original),"inputManagedSHA256":sha(before),"outputManagedSHA256":sha((output/"Assembly-CSharp.dll").read_bytes()),"inputDexSHA256":sha(base_dex),"outputDexSHA256":sha((output/"classes2.dex").read_bytes()),"sourcePins":{str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in [Path(__file__).resolve(),source,injector]},"newDexClasses":sorted(newclasses-oldclasses),"inheritedWalletAndRegistrationDexIsD8Input":True,"walletSourceRecompiled":False,"modernIMEOriginalClassesDexMustRemainExact":True,"phoneExecuted":False,"APKPackaged":False,**checks}
    (output/"manifest.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print(json.dumps({"status":report["status"],"outputManagedSHA256":report["outputManagedSHA256"],"outputDexSHA256":report["outputDexSHA256"],"engineExact":checks["originalEngineMethodsUnchanged"]}))


if __name__=="__main__": main()
