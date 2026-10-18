"""One-method, failure-only Awake End overlay. No APK packaging or phone action."""
import argparse
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
sys.path.insert(0,str(ROOT/'tools'))
from local_asset_ready.remove_gate import exact_body,require,sha
from awake_diff_rows.build import methods,operand,symbol
from build_wallet_client import verify_wallet_upgrade_base

ENTRY='assets/bin/Data/Managed/Assembly-CSharp.dll'
BASE_APK='92d1ed3483f58f0abbc8f5a598396f9a256142111a9b82432e34a1b2b1fdbae3'
BASE_DLL='7342ebf00795a75251aee97bc20ff43cc8ea52eb8a84d71f7414a12feb4820d3'
TARGET='Battle.AwakeCopyBattleOver::OnAskAwakeCopyFightEndCallback'
BODY='6aa6d3af53242bbc0c612aa4225bce40b3738faddd318240650977ffc8958cd3'


def verify(before,after):
    a,b=dnfile.dnPE(data=before),dnfile.dnPE(data=after)
    old,new=methods(a),methods(b)
    require(len(old)==len(new),'Method inventory changed')
    unchanged=engine=changed=0
    for (name,rid,m),(nn,nr,n) in zip(old,new):
        require((name,rid)==(nn,nr) and m.Signature.value==n.Signature.value and m.struct.Flags==n.struct.Flags and m.struct.ImplFlags==n.struct.ImplFlags,'ABI/token/flags changed: '+name)
        if not m.Rva:
            require(not n.Rva,'Body added: '+name)
            continue
        x,y=exact_body(a,m),exact_body(b,n)
        if name!=TARGET:
            require(x==y,'Non-target raw header/IL/EH changed: '+name)
            unchanged+=1;engine+=name.startswith('Battle.Engine')
            continue
        changed+=1
        require(sha(x)==BODY,'Original callback guard differs')
        ox,ny=read_method_body_from_bytes(x),read_method_body_from_bytes(y)
        require(not ox.exception_handlers and not ny.exception_handlers,'Callback EH changed')
        prefix=ny.instructions[:9];suffix=ny.instructions[9:]
        require(len(suffix)==len(ox.instructions),'Original suffix length changed')
        oi={i.offset:k for k,i in enumerate(ox.instructions)};ni={i.offset:k for k,i in enumerate(suffix)}
        for oldi,newi in zip(ox.instructions,suffix):
            require(oldi.opcode.name==newi.opcode.name,'Original opcode changed')
            if oldi.opcode.flow_control.name in {'Branch','Cond_Branch'}:
                require(oi[oldi.operand]==ni[newi.operand],'Original branch changed')
            else:require(operand(oldi.operand)==operand(newi.operand),'Original operand changed')
        require([i.opcode.name for i in prefix]==['ldarg.2','brtrue','ldnull','stsfld','ldc.i4.0','stsfld','call','callvirt','ret'],'Unexpected recovery prefix')
        require(prefix[1].operand==suffix[0].offset,'Successful callback skips original suffix')
        require([symbol(b,prefix[i].operand) for i in [3,5,6,7]]==['lastBattleEndMsg','isLastBattleWin','get_Instance','BattleToAwakeCopy'],'Recovery calls changed')
    require(engine==547 and changed==1,'Original Engine/target inventory differs')
    return {'unchangedRawMethodBodies':unchanged,'originalEngineMethodsUnchanged':engine,'changedMethod':TARGET,'acceptedCallbackInstructionsAndBranchesPreserved':True,'failureOnlyReturnToOriginalRoom':True,'noSynthesizedEndOrResultCallback':True}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk',type=Path,required=True)
    parser.add_argument('--base-proof',type=Path,required=True)
    parser.add_argument('--base-promotion-proof',type=Path,help='Independent installed/current promotion record for a qualified candidate APK')
    parser.add_argument('--output-directory',type=Path,required=True)
    parser.add_argument('--rebind-manifest',action='store_true',help='Reverify an existing overlay DLL and bind the complete qualified base proof without compiling or copying DLLs')
    args=parser.parse_args()
    require(args.output_directory.is_dir() if args.rebind_manifest else not args.output_directory.exists(),'Existing overlay directory required for rebind; fresh directory required for build')
    require(sha(args.base_apk.read_bytes())==BASE_APK,'Current qualified APK bytes differ')
    proof=json.loads(args.base_proof.read_text())
    promotion=None
    if args.base_promotion_proof:
        promotion_path=args.base_promotion_proof.resolve()
        require(promotion_path.is_relative_to(ROOT/'analysis') and promotion_path.is_file(),'Use a saved local base APK promotion record')
        promotion=json.loads(promotion_path.read_text())
    with zipfile.ZipFile(args.base_apk) as apk:
        # 使用统一钱包管道的完整证明校验，签名/zipalign本身不足以证明已继承全部钱包字节。
        verify_wallet_upgrade_base(apk,proof,BASE_APK,promotion)
        before=apk.read(ENTRY)
    require(sha(before)==BASE_DLL,'Current assembly bytes differ')
    if args.rebind_manifest:
        manifest_path=args.output_directory/'manifest.json'
        previous_raw=manifest_path.read_bytes()
        previous=json.loads(previous_raw)
        output=args.output_directory/'Assembly-CSharp.dll'
        require(previous.get('schema')=='local-awake-end-recovery-overlay-v1' and previous.get('baseAPKSHA256')==BASE_APK and previous.get('inputManagedSHA256')==BASE_DLL,'Existing overlay identity differs')
        require(previous.get('outputManagedSHA256')==sha(output.read_bytes()) and previous.get('replacementEntries')=={ENTRY:sha(output.read_bytes())},'Existing overlay DLL differs from its manifest')
        checks=verify(before,output.read_bytes())
        previous.update(checks)
        previous['baseProofSHA256']=sha(args.base_proof.read_bytes())
        if args.base_promotion_proof:
            previous['basePromotionProofSHA256']=sha(args.base_promotion_proof.read_bytes())
        previous['sourcePins']={str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in [Path(__file__).resolve(),Path(__file__).with_name('Injector.cs').resolve()]}
        previous['proofRebind']={'previousManifestSHA256':sha(previous_raw),'previousBaseProofSHA256':json.loads(previous_raw)['baseProofSHA256'],'baseProofPath':str(args.base_proof.resolve().relative_to(ROOT)),'completeWalletBaseReverified':True,'overlayDLLUnchanged':True,'compiled':False,'apkBuilt':False}
        if args.base_promotion_proof:
            previous['proofRebind']['basePromotionProofPath']=str(args.base_promotion_proof.resolve().relative_to(ROOT))
            previous['proofRebind']['basePromotionProofSHA256']=sha(args.base_promotion_proof.read_bytes())
        manifest_path.write_text(json.dumps(previous,ensure_ascii=False,indent=2)+'\n')
        print(json.dumps({'status':'PASS_REBIND_MANIFEST_ONLY','manifest':str(manifest_path),'outputManagedSHA256':previous['outputManagedSHA256'],'baseProofSHA256':previous['baseProofSHA256'],'checks':checks}))
        return
    args.output_directory.mkdir(parents=True)
    runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    with tempfile.TemporaryDirectory(prefix='hyjs-awake-recovery-') as temp:
        folder=Path(temp);(folder/'Assembly-CSharp.dll').write_bytes(before)
        shutil.copy2(Path(__file__).with_name('Injector.cs'),folder/'Program.cs')
        (folder/'Patch.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        def run(argv):subprocess.run(['rtk','proxy',*map(str,argv)],cwd=ROOT,check=True,timeout=90)
        run([dotnet,'build',folder/'Patch.csproj','--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        output=args.output_directory/'Assembly-CSharp.dll'
        run([dotnet,folder/'bin/Debug/net8.0/Patch.dll',folder/'Assembly-CSharp.dll',output])
        checks=verify(before,output.read_bytes())
    pins={str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in [Path(__file__).resolve(),Path(__file__).with_name('Injector.cs').resolve()]}
    manifest={'schema':'local-awake-end-recovery-overlay-v1','baseProofSHA256':sha(args.base_proof.read_bytes()),'originalEngineInstructionBytesAndExceptionRegionsUnchanged':True,'originalEngineMethods':547,'onlyChangedEntries':[TARGET],'allOtherOriginalRawMethodHeadersILAndEHByteIdentical':True,'noNewRuntimeAssembly':True,'playerStateOrBattleResultChanged':False,'status':'PASS_OFFLINE_NOT_PACKAGED','baseAPKSHA256':BASE_APK,'inputManagedSHA256':BASE_DLL,'outputManagedSHA256':sha(output.read_bytes()),'replacementEntries':{ENTRY:sha(output.read_bytes())},'sourcePins':pins,**checks,'phoneExecuted':False,'apkBuilt':False}
    if args.base_promotion_proof:
        manifest['basePromotionProofSHA256']=sha(args.base_promotion_proof.read_bytes())
    (args.output_directory/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(manifest))

if __name__=='__main__':main()
