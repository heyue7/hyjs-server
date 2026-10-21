using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Writer;

if(args.Length!=4)throw new ArgumentException("input DLL, inherited origin, QA origin and output DLL required");
using var module=ModuleDefMD.Load(args[0]);
var inherited=args[1];
var origin=args[2];
if(string.IsNullOrWhiteSpace(origin)||origin.Any(char.IsWhiteSpace)
    ||!Uri.TryCreate(origin,UriKind.Absolute,out var uri)
    ||(uri.Scheme!="http"&&uri.Scheme!="https")||string.IsNullOrEmpty(uri.Host)
    ||!string.IsNullOrEmpty(uri.UserInfo)||uri.AbsolutePath!="/"
    ||!string.IsNullOrEmpty(uri.Query)||!string.IsNullOrEmpty(uri.Fragment)||uri.Port<1)
    throw new ArgumentException("Use an HTTP(S) origin with host and optional port only");
origin=origin.TrimEnd('/');
var targets=new[]{
    ("ServerConfig","GetServerUrl",inherited+"/sao/",origin+"/sao/"),
    ("Battle.FieldPVEWorldBossBattleOver","End",inherited,origin),
    ("Room.RoomNetField","SCCurrentPosition",inherited,origin),
    ("VoicePlugin","InitGVoice",inherited,origin)
};
foreach(var target in targets){
    var method=module.GetTypes().Single(t=>t.FullName==target.Item1).Methods.Single(m=>m.Name==target.Item2);
    var literal=method.Body.Instructions.Single(i=>i.OpCode==dnlib.DotNet.Emit.OpCodes.Ldstr&&(string)i.Operand==target.Item3);
    literal.Operand=target.Item4;
}
var options=new ModuleWriterOptions(module);
options.MetadataOptions.Flags|=MetadataFlags.PreserveAll|MetadataFlags.KeepOldMaxStack;
module.Write(args[3],options);
Console.WriteLine("Changed only the four approved original/local HTTP origin literals");
