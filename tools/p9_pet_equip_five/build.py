"""Pinned non-Engine GetFiveCost overlay. Never build/install an APK."""
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
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from local_asset_ready.remove_gate import exact_body, require, sha

ROOT = Path(__file__).resolve().parents[2]
BASE_APK = '692767bc2d23ac79f00be13632551b85f6938ea6bd790ec3deea46468f30ec43'
BASE_DLL = '0be3d908fdec86aecc588c024900f7068e3bf0ef578b1cf8fb497532ed4b67b6'
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
TARGET = 'UI.PetEquipStrengthTab::GetFiveCost'


def methods(pe):
    return [(str(t.TypeNamespace)+'.'+str(t.TypeName)+'::'+str(m.row.Name), m.row_index,m.row)
            for t in pe.net.mdtables.TypeDef for m in t.MethodList]


def symbol(pe, token):
    value=token.value; table={0x04:'Field',0x06:'MethodDef',0x0a:'MemberRef'}[value >> 24]
    return str(getattr(pe.net.mdtables,table).rows[(value & 0xffffff)-1].Name)


def instruction_key(pe, body, i):
    op=i.operand
    offsets={x.offset:n for n,x in enumerate(body.instructions)}
    if i.opcode.flow_control.name in {'Branch','Cond_Branch'}:
        op=offsets[op] if isinstance(op,int) else op
    elif hasattr(op,'value'): op=op.value
    elif hasattr(op,'index'): op=op.index
    return [i.opcode.name,op]


def verify(before_raw,after_raw):
    require(sha(before_raw)==BASE_DLL,'Exact installed qualified managed base required')
    before,after=dnfile.dnPE(data=before_raw),dnfile.dnPE(data=after_raw)
    a,b=methods(before),methods(after)
    require(len(a)==len(b),'No added/removed methods')
    count=engine=0; target=None
    for (name,rid,old),(newname,newrid,new) in zip(a,b):
        require((name,rid)==(newname,newrid) and old.Signature.value==new.Signature.value and old.struct.Flags==new.struct.Flags and old.struct.ImplFlags==new.struct.ImplFlags,'Method ABI/token/flags differ '+name)
        if not old.Rva:
            require(not new.Rva,'Bodyless method changed'); continue
        oldraw,newraw=exact_body(before,old),exact_body(after,new)
        if name!=TARGET:
            require(oldraw==newraw,'Non-target raw header/IL/EH changed '+name)
            count+=1; engine+=name.startswith('Battle.Engine')
        else:
            target=(new,newraw)
            ob,nb=read_method_body_from_bytes(oldraw),read_method_body_from_bytes(newraw)
            require(not ob.exception_handlers and not nb.exception_handlers,'Target EH unexpectedly changed')
            # Every old instruction follows the new prefix; only loop constant becomes local4.
            suffix=nb.instructions[-len(ob.instructions):]
            require(len(suffix)==len(ob.instructions),'Lost original loop')
            old_indexes={x.offset:n for n,x in enumerate(ob.instructions)}
            new_indexes={x.offset:n for n,x in enumerate(suffix)}
            altered=[]
            for oi,ni in zip(ob.instructions,suffix):
                if oi.offset==0x72:
                    require(oi.opcode.name=='ldc.i4.5' and ni.opcode.name=='ldloc' and ni.operand.index==4,'Expected only loop bound local4')
                    altered.append(oi.offset); continue
                require(oi.opcode.name==ni.opcode.name,'Old opcode changed')
                if oi.opcode.flow_control.name in {'Branch','Cond_Branch'}:
                    require(old_indexes[oi.operand]==new_indexes[ni.operand],'Old loop branch target changed')
                else:
                    def operand(x):
                        return x.value if hasattr(x,'value') else x.index if hasattr(x,'index') else x
                    require(operand(oi.operand)==operand(ni.operand),'Original loop operand changed')
            require(altered==[0x72],'Expected one loop bound replacement')
            details=dict(method=TARGET,token=hex(0x06000000|rid),inputRawMethodSHA256=sha(oldraw),outputRawMethodSHA256=sha(newraw),
                originalCodeBytes=ob.code_size,outputCodeBytes=nb.code_size,newPrefixInstructionCount=len(nb.instructions)-len(ob.instructions),
                originalLoopInstructionsPreservedExceptBound=True,targetEHCount=0)
    require(engine==547 and target is not None,'All inherited Engine bodies required')
    return dict(originalEngineMethods=engine,unchangedRawMethodBodies=count,
                allOtherOriginalRawMethodHeadersILAndEHByteIdentical=True,methodTokensAndABIPreserved=True,
                onlyChangedMethods=[TARGET],target=details),after,target[1]


def evaluate(pe,raw,self_obj,advance_rows,strength_rows):
    """Interpret emitted target IL with only its existing table/string calls modeled.

    This tests emitted control flow and fees; it does not claim Unity/native UI execution.
    """
    instructions=read_method_body_from_bytes(raw).instructions
    offsets={i.offset:n for n,i in enumerate(instructions)}
    stack=[]; local=[None]*6; pc=0; fetched=[]
    for _ in range(3000):
        i=instructions[pc]; pc+=1; op=i.opcode.name; arg=i.operand
        if op=='nop': pass
        elif op=='ldarg.0': stack.append(self_obj)
        elif op.startswith('ldc.i4.'):
            tail=op[7:]; stack.append(int(arg) if tail=='s' else -1 if tail=='m1' else int(tail))
        elif op=='ldc.i4': stack.append(arg)
        elif op.startswith('ldloc'):
            n=int(op[-1]) if op[-1].isdigit() else arg.index; stack.append(local[n])
        elif op.startswith('stloc'):
            n=int(op[-1]) if op[-1].isdigit() else arg.index; local[n]=stack.pop()
        elif op=='ldfld': stack.append(stack.pop()[symbol(pe,arg)])
        elif op=='dup': stack.append(stack[-1])
        elif op=='pop': stack.pop()
        elif op in {'add','sub'}:
            b,a=stack.pop(),stack.pop();stack.append(a+b if op=='add' else a-b)
        elif op=='ldstr': stack.append(pe.net.user_strings.get(arg.value&0xffffff).value)
        elif op=='newarr': stack.append([None]*stack.pop())
        elif op=='stelem.i2':
            v,idx,arr=stack.pop(),stack.pop(),stack.pop();arr[idx]=v
        elif op=='ldelem.ref':
            idx,arr=stack.pop(),stack.pop();stack.append(arr[idx])
        elif op in {'call','callvirt'}:
            name=symbol(pe,arg)
            if name=='GetCurrPet': stack.append(stack.pop()['currentPet'])
            elif name in {'GetPetEquipAdvance','GetPetEquipStrength'}:
                level,equip=stack.pop(),stack.pop()
                if name=='GetPetEquipStrength': fetched.append(level)
                stack.append((advance_rows if name=='GetPetEquipAdvance' else strength_rows).get((equip,level)))
            elif name=='Split':
                chars,s=stack.pop(),stack.pop(); stack.append(s.split(chr(chars[0])))
            elif name=='Equals':
                b,a=stack.pop(),stack.pop();stack.append(a==b)
            elif name=='Parse': stack.append(int(stack.pop()))
            else: raise AssertionError('Unmodeled emitted call '+name)
        elif op=='br': pc=offsets[arg]
        elif op in {'brtrue','brfalse'}:
            value=bool(stack.pop())
            if value==(op=='brtrue'): pc=offsets[arg]
        elif op in {'ble','bge','blt','bgt'}:
            b,a=stack.pop(),stack.pop()
            if {'ble':a<=b,'bge':a>=b,'blt':a<b,'bgt':a>b}[op]:pc=offsets[arg]
        elif op=='ret':
            result=stack.pop();require(not stack,'Emitted return stack unbalanced');return result,fetched
        else: raise AssertionError('Unmodeled emitted opcode '+op)
    raise AssertionError('Emitted IL exceeded step bound')


def matrix(pe,body):
    def rows(name):
        lines=(ROOT/f'analysis/tables/Table{name}.tsv').read_text().splitlines()
        return [dict(zip(lines[1].split('\t'),line.split('\t'))) for line in lines[5:] if line]
    advance={(r['equipId'],int(r['level'])):{k:int(v) if k in {'needStrengthLevel','maxStrengthLevel'} else v for k,v in r.items()} for r in rows('PetEquipAdvance')}
    strength={(r['equipId'],int(r['level'])):r for r in rows('PetEquipStrength')}
    equips=rows('PetEquip');require(len(equips)==76,'Original 76 equipment rows required')
    count=0;steps_seen=set()
    for e in equips:
        key=e['id']
        for adv in range(13):
            cap=advance[key,adv or 1]['maxStrengthLevel' if adv else 'needStrengthLevel']
            for pet_limit in [cap,cap-1]:
                for steps in range(6):
                    current=pet_limit-steps
                    obj={'petEquip':{'equipId':key,'strLevel':current,'advanLevel':adv,'maxStrLevel':100},'page':{'currentPet':{'level':pet_limit}}}
                    total,got=evaluate(pe,body,obj,advance,strength)
                    expected=list(range(current+1,current+steps+1))
                    expected_cost=sum(int(strength[key,n]['cost'].split(',')[1]) for n in expected)
                    require(got==expected and total==expected_cost,f'Emitted fees differ: {key}/{adv}/{pet_limit}/{current}')
                    count+=1;steps_seen.add(steps)
    edge_count=0
    for e in equips:
        key=e['id']
        for pet_level,current,adv,expected_steps in [(20,0,0,5),(20,11,0,0),(20,8,0,2),(9,6,0,3),(100,98,12,2),(100,100,12,0)]:
            obj={'petEquip':{'equipId':key,'strLevel':current,'advanLevel':adv,'maxStrLevel':100},'page':{'currentPet':{'level':pet_level}}}
            total,got=evaluate(pe,body,obj,advance,strength)
            expected=list(range(current+1,current+expected_steps+1))
            require(got==expected and total==sum(int(strength[key,n]['cost'].split(',')[1]) for n in expected),'Emitted boundary quote differs')
            edge_count+=1
        obj={'petEquip':{'equipId':key,'strLevel':0,'advanLevel':99,'maxStrLevel':100},'page':{'currentPet':{'level':20}}}
        require(evaluate(pe,body,obj,advance,strength)==(2147483647,[]),'Missing configuration must not produce cheap quote')
        edge_count+=1
    return dict(status='PASS',method='emitted IL interpretation with original table rows and explicit table/string call model',
                cases=count,edgeCases=edge_count,rawMaximumCrossingStillRejectedByUnchangedIsCanStrength=True,equipmentIDs=76,advanceStates=13,actualSteps=sorted(steps_seen),
                nativeUnityExecuted=False,phoneUI=False,originalGateConsumerMethodByteIdentical=True)


def run(args):
    subprocess.run(['rtk','proxy',*map(str,args)],cwd=ROOT,check=True)


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--base-proof',type=Path,required=True)
    p.add_argument('--output-directory',type=Path,required=True);args=p.parse_args()
    require(not args.output_directory.exists(),'Fresh output directory required')
    apk_sha=hashlib.file_digest(args.base_apk.open('rb'),'sha256').hexdigest()
    proofraw=args.base_proof.read_bytes();proof=json.loads(proofraw)
    require(apk_sha==BASE_APK and proof.get('binarySHA256')==apk_sha,'Pinned installed APK/proof required')
    require(proof.get('signatureVerified') and proof.get('zipalignVerified'),'Base qualification required')
    with zipfile.ZipFile(args.base_apk) as z:
        original=z.read(ENTRY)
        require(sha(original)==BASE_DLL,'Preserve awake/wallet patched assembly')
        for key,value in proof['qualifiedWalletEntries'].items():require(sha(z.read(key))==value,'Inherited entry differs '+key)
    args.output_directory.mkdir(parents=True)
    runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    with tempfile.TemporaryDirectory(prefix='hyjs-p9-five-') as temporary:
        folder=Path(temporary);incoming=folder/'Assembly-CSharp.dll';incoming.write_bytes(original)
        shutil.copy2(Path(__file__).with_name('Injector.cs'),folder/'Program.cs')
        project=folder/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run([dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        output=args.output_directory/'Assembly-CSharp.dll'
        run([dotnet,folder/'bin/Debug/net8.0/Patch.dll',incoming,output])
        checks,pe,body=verify(original,output.read_bytes())
        cases=matrix(pe,body)
    require(hashlib.file_digest(args.base_apk.open('rb'),'sha256').hexdigest()==apk_sha,'Input APK changed')
    manifest=dict(schema='p9-equipment-five-ui-overlay-v1',status='PASS_OFFLINE_NOT_PACKAGED',**checks,
        baseAPKSHA256=apk_sha,baseProof=str(args.base_proof),baseProofSHA256=sha(proofraw),
        inputManagedSHA256=sha(original),outputManagedSHA256=sha(output.read_bytes()),replacementEntries={ENTRY:sha(output.read_bytes())},
        correctness=cases,localPolicy='local-pet-equipment-five-bounded-v1',apkBuilt=False,phoneExecuted=False,
        inheritedAwakeWalletAndIMEPreserved=True,noNewRuntimeAssembly=True,
        sourcePins={str(f.relative_to(ROOT)):sha(f.read_bytes()) for f in [Path(__file__).resolve(),Path(__file__).with_name('Injector.cs').resolve(),ROOT/'tools/local_asset_ready/remove_gate.py']},
        tablePins={n:sha((ROOT/f'analysis/tables/Table{n}.tsv').read_bytes()) for n in ['PetEquip','PetEquipStrength','PetEquipAdvance']})
    (args.output_directory/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(manifest,ensure_ascii=False))

if __name__=='__main__':main()
