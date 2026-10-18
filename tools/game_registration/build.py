"""Build a three-handler explicit-registration DLL overlay; no APK/device changes."""
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

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require

TARGETS = {"UserInput::OnRegistButtonClick"}
ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def verify(before, after):
    a, b = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    old, new = methods(a), methods(b)
    require(len(new) == len(old), "Inherited method inventory changed")
    changes = []
    unchanged = engine = 0
    for (name, rid, method), (other, oid, modified) in zip(old, new):
        require((name, rid) == (other, oid) and method.Signature.value == modified.Signature.value and
                method.struct.Flags == modified.struct.Flags and method.struct.ImplFlags == modified.struct.ImplFlags,
                "Inherited ABI/token/flags changed: " + name)
        if not method.Rva:
            require(not modified.Rva, "Unexpected inherited body addition: " + name)
            continue
        x, y = exact_body(a, method), exact_body(b, modified)
        if name not in TARGETS:
            require(x == y, "Non-target raw method changed: " + name)
            unchanged += 1
            engine += name.startswith("Battle.Engine")
            continue
        body = read_method_body_from_bytes(y)
        expected=["ldstr","newobj","dup","ldstr","ldc.i4.1","newarr","dup","ldc.i4.0","call","stelem.ref","callvirt","callvirt","ret"]
        require(not body.exception_handlers and [i.opcode.name for i in body.instructions] == expected, "Unexpected handler shape")
        strings=[b.net.user_strings.get(i.operand.value & 0xffffff).value for i in body.instructions if i.opcode.name=="ldstr"]
        require(strings==["com.hyjs.wallet.GameRegistration","show"], "Wrong native registration entry")
        token=body.instructions[8].operand.value
        require(next(n for n,index,_ in new if index==(token&0xffffff))=="ServerConfig::GetServerUrl", "Registration endpoint is not inherited server address")
        changes.append({"method":name,"beforeRawSHA256":sha(x),"afterRawSHA256":sha(y),"javaClass":strings[0]})
    require(engine==547 and len(changes)==1,"Engine/handler inventory differs")
    return {"originalEngineMethodsUnchanged":engine,"unchangedInheritedRawMethodBodies":unchanged,"changes":changes,"allInheritedNonTargetMethodsExact":True}


def main():
    import os
    from tools.local_wallet.verify import dex_classes
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk",type=Path,required=True)
    parser.add_argument("--base-sha256",required=True)
    parser.add_argument("--output-directory",type=Path,required=True)
    args=parser.parse_args()
    require(sha(args.base_apk.read_bytes())==args.base_sha256,"Base APK SHA differs")
    output=args.output_directory.resolve();require(not output.exists(),"Use a fresh output directory");output.mkdir(parents=True)
    with zipfile.ZipFile(args.base_apk) as apk:
        before=apk.read(ENTRY);base_dex=apk.read("classes2.dex")
    injector=Path(__file__).with_name("Injector.cs")
    source=Path(__file__).parent/"java/com/hyjs/wallet/GameRegistration.java"
    work=ROOT/".tools/native-batch";dotnet=work/"dotnet/dotnet";dnlib=work/"injector/bin/Debug/net8.0/dnlib.dll"
    def run(name,command,env=None):
        result=subprocess.run(["rtk","proxy",*map(str,command)],cwd=ROOT,capture_output=True,text=True,timeout=90,env=env)
        (output/(name+".log")).write_text(result.stdout+result.stderr)
        require(result.returncode==0,name+" failed; see saved log")
    with tempfile.TemporaryDirectory(prefix="hyjs-registration-") as temporary:
        folder=Path(temporary);current=folder/"Assembly-CSharp.dll";current.write_bytes(before)
        shutil.copy2(injector,folder/"Program.cs");project=folder/"Patch.csproj"
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run("compile-injector",[dotnet,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"])
        run("patch",[dotnet,folder/"bin/Debug/net8.0/Patch.dll",current,output/"Assembly-CSharp.dll"])
        sdk=Path.home()/"Library/Android/sdk";jdk=Path("/opt/homebrew/opt/openjdk@21");android=sdk/"platforms/android-33/android.jar"
        env=dict(os.environ,JAVA_HOME=str(jdk),PATH=str(jdk/"bin")+os.pathsep+os.environ.get("PATH",""))
        classes=folder/"classes";dex=folder/"dex";classes.mkdir();dex.mkdir();old_dex=folder/"old-wallet.dex";old_dex.write_bytes(base_dex)
        run("compile-java",[jdk/"bin/javac","-encoding","UTF-8","-source","8","-target","8","-bootclasspath",android,"-d",classes,source],env)
        run("dex-merge",[sdk/"cmdline-tools/latest/bin/d8","--min-api","23","--lib",android,"--output",dex,old_dex,*sorted(classes.rglob("*.class"))],env)
        require(sorted(p.name for p in dex.iterdir())==["classes.dex"],"Expected one isolated wallet/account dex")
        (output/"classes2.dex").write_bytes((dex/"classes.dex").read_bytes())
    checks=verify(before,(output/"Assembly-CSharp.dll").read_bytes())
    oldclasses=set(dex_classes(base_dex));newclasses=set(dex_classes((output/"classes2.dex").read_bytes()))
    require(oldclasses<newclasses and all(n.startswith("Lcom/hyjs/wallet/GameRegistration") for n in newclasses-oldclasses),"Unexpected dex class changes")
    report={"schema":"hyjs-explicit-native-registration-overlay-v2","status":"STATIC_VERIFIED_NOT_PACKAGED","baseAPKSHA256":args.base_sha256,"inputManagedSHA256":sha(before),"outputManagedSHA256":sha((output/"Assembly-CSharp.dll").read_bytes()),"inputDexSHA256":sha(base_dex),"outputDexSHA256":sha((output/"classes2.dex").read_bytes()),"sourcePins":{str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in [Path(__file__).resolve(),source,injector]},"registrationEndpoint":"/game-account/register","registrationIsExplicit":True,"registeredAccountAutomaticallyLoggedIn":False,"passwordPersistenceAdded":False,"newDexClasses":sorted(newclasses-oldclasses),"inheritedWalletDexIsD8Input":True,"walletSourceRecompiled":False,"modernIMEOriginalClassesDexMustRemainExact":True,"phoneExecuted":False,"APKPackaged":False,**checks}
    (output/"manifest.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print(json.dumps({"status":report["status"],"outputManagedSHA256":report["outputManagedSHA256"],"outputDexSHA256":report["outputDexSHA256"],"engineExact":checks["originalEngineMethodsUnchanged"]}))


if __name__=="__main__": main()
