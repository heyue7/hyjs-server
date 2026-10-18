"""Compile a named local PCM bridge and patch only the original voice entries.

Default disabled, with no installation or APK packaging. Root owns device operations.
"""
import argparse, hashlib, json, shutil, subprocess, tempfile
from urllib.parse import urlsplit
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
ROOT=Path(__file__).resolve().parents[2]
WORK=ROOT/'.tools/native-batch'
OWN={'InitGVoice','StartRecord','StopRecord','ClearRecordDelage','PlayRecord','DownLoadFile','Play','StopPlay','DeleFile','Tick'}
def run(args):
    result=subprocess.run(['rtk','proxy',*[str(x) for x in args]],check=True,capture_output=True,text=True)
    if result.stdout:print(result.stdout.strip())
def verify(original,modified,allowed_entries=None):
    allowed_entries = allowed_entries or {('VoicePlugin', name) for name in OWN}
    before=dnfile.dnPE(data=original);after=dnfile.dnPE(data=modified);count=0;engine_count=0;patched=[]
    for index,owner in enumerate(before.net.mdtables.TypeDef.rows):
        new=after.net.mdtables.TypeDef.rows[index]
        assert (str(owner.TypeNamespace),str(owner.TypeName))==(str(new.TypeNamespace),str(new.TypeName))
        for entry in owner.MethodList:
            row=entry.row;updated=after.net.mdtables.MethodDef.rows[entry.row_index-1]
            assert str(row.Name)==str(updated.Name) and row.Signature.value==updated.Signature.value
            if not row.Rva:continue
            identity=(str(owner.TypeName),str(row.Name))
            if identity in allowed_entries:patched.append(identity);continue
            a=read_method_body_from_bytes(before.get_data(row.Rva));b=read_method_body_from_bytes(after.get_data(updated.Rva))
            def token(pe, value):
                kind=value>>24;index=(value&0xffffff)-1
                if kind==0x70:return ('string',pe.net.user_strings.get(value&0xffffff).value)
                if kind==0x1b:return ('typespec',pe.net.mdtables.TypeSpec.rows[index].Signature.value)
                if kind==0x11:return ('signature',pe.net.mdtables.StandAloneSig.rows[index].Signature.value)
                if kind==0x2b:
                    method=pe.net.mdtables.MethodSpec.rows[index];tablekind={'MethodDef':0x06,'MemberRef':0x0a}[method.Method.table.name]
                    return ('methodspec',token(pe,(tablekind<<24)|method.Method.row_index),method.Instantiation.value)
                if kind==0x0a:
                    member=pe.net.mdtables.MemberRef.rows[index];parent=member.Class.row
                    identity=(str(getattr(parent,'TypeNamespace','')),str(getattr(parent,'TypeName','')),getattr(getattr(parent,'Signature',None),'value',None))
                    return ('member',identity,str(member.Name),member.Signature.value)
                return value
            def instructions(pe, body):
                positions={x.offset:i for i,x in enumerate(body.instructions)}
                result=[]
                for x in body.instructions:
                    name=str(x.opcode.name);operand=x.operand
                    if name.startswith(('br','beq','bge','bgt','ble','blt','bne','leave')):operand=positions[operand]
                    elif name=='switch':operand=tuple(positions[y] for y in operand)
                    elif hasattr(operand,'value'):operand=token(pe,operand.value)
                    else:operand=str(operand)
                    result.append((name,operand))
                return result
            assert instructions(before,a)==instructions(after,b),(str(owner.TypeName),str(row.Name))
            if str(owner.TypeNamespace).startswith('Battle.Engine'):
                engine_count+=1
                assert before.get_data(row.Rva+a.header_size,a.code_size)==after.get_data(updated.Rva+b.header_size,b.code_size),('original engine bytes',str(owner.TypeName),str(row.Name))
            shape=lambda x:[(h.exception_type,h.try_start,h.try_end,h.handler_start,h.handler_end,h.filter_start,getattr(h.catch_type,'value',None)) for h in x.exception_handlers]
            assert shape(a)==shape(b);count+=1
    assert set(patched)==allowed_entries
    entries=sorted(name for owner,name in patched) if all(owner=='VoicePlugin' for owner,name in patched) else sorted(owner+'::'+name for owner,name in patched)
    return {'originalUnmodifiedMethodBodies':count,'originalEngineInstructionBytesAndExceptionRegionsUnchanged':True,'originalEngineMethods':engine_count,'onlyChangedEntries':entries}
def main():
    parser=argparse.ArgumentParser();parser.add_argument('--input',type=Path,required=True);parser.add_argument('--output-directory',type=Path,required=True);parser.add_argument('--origin',required=True);parser.add_argument('--enable-local-voice',action='store_true');args=parser.parse_args()
    origin=urlsplit(args.origin)
    if origin.scheme not in ('http','https') or not origin.hostname or origin.username or origin.password or origin.query or origin.fragment or origin.path not in ('','/') :raise ValueError('explicit public voice server origin required')
    if args.output_directory.exists() and any(args.output_directory.iterdir()):raise ValueError('output directory must be empty')
    args.output_directory.mkdir(parents=True,exist_ok=True)
    dotnet=WORK/'dotnet/dotnet';compiler=next((WORK/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs=sorted((ROOT/'analysis/extracted/assets/bin/Data/Managed').glob('*.dll'))
    bridge=args.output_directory/'LocalVoicePCM.dll'
    run([dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(bridge),*['/reference:'+str(x) for x in refs],ROOT/'tools/p10_voice/LocalVoiceBridge.cs'])
    with tempfile.TemporaryDirectory(prefix='hyjs-local-voice-injector-') as temp:
        temp=Path(temp);shutil.copy2(ROOT/'tools/p10_voice/VoiceInjector.cs',temp/'Program.cs')
        dnlib=WORK/'injector/bin/Debug/net8.0/dnlib.dll'
        (temp/'VoiceInjector.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run([dotnet,'build',temp/'VoiceInjector.csproj','--nologo','--verbosity','quiet'])
        output=args.output_directory/'Assembly-CSharp.dll'
        run([dotnet,temp/'bin/Debug/net8.0/VoiceInjector.dll',args.input,bridge,output,args.origin,'true' if args.enable_local_voice else 'false'])
    proof=verify(args.input.read_bytes(),output.read_bytes());proof.update(adapter='local-original-chat-audio2-wav-v1',enabled=args.enable_local_voice,originalGCloudAuthenticationRestored=False,humanMicrophoneAndPlaybackVerified=False,inputSHA256=hashlib.sha256(args.input.read_bytes()).hexdigest(),outputSHA256=hashlib.sha256(output.read_bytes()).hexdigest(),bridgeSHA256=hashlib.sha256(bridge.read_bytes()).hexdigest())
    (args.output_directory/'manifest.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps(proof))
if __name__=='__main__':main()
