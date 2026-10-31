using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Writer;

if (args.Length != 2)
    throw new ArgumentException("Expected input and output assembly paths");
var inherited = Environment.GetEnvironmentVariable("HYJS_FORMAL110_INHERITED_ORIGIN");
var origin = Environment.GetEnvironmentVariable("HYJS_FORMAL110_TARGET_ORIGIN");
if (string.IsNullOrWhiteSpace(inherited) || string.IsNullOrWhiteSpace(origin)
    || inherited.Any(char.IsWhiteSpace) || origin.Any(char.IsWhiteSpace)
    || !Uri.TryCreate(origin, UriKind.Absolute, out var uri)
    || (uri.Scheme != "http" && uri.Scheme != "https")
    || string.IsNullOrEmpty(uri.Host) || !string.IsNullOrEmpty(uri.UserInfo)
    || uri.AbsolutePath != "/" || !string.IsNullOrEmpty(uri.Query)
    || !string.IsNullOrEmpty(uri.Fragment) || uri.Port < 1)
    throw new ArgumentException("Invalid inherited or target API origin");
inherited = inherited.TrimEnd('/');
origin = origin.TrimEnd('/');
if (inherited == origin) throw new ArgumentException("Target origin must differ from the inherited origin");

using var module = ModuleDefMD.Load(args[0]);
var targets = new[] {
    ("ServerConfig", "GetServerUrl", inherited + "/sao/", origin + "/sao/"),
    ("Battle.FieldPVEWorldBossBattleOver", "End", inherited, origin),
    ("Room.RoomNetField", "SCCurrentPosition", inherited, origin),
    ("VoicePlugin", "InitGVoice", inherited, origin)
};
foreach (var target in targets) {
    var method = module.GetTypes().Single(t => t.FullName == target.Item1)
        .Methods.Single(m => m.Name == target.Item2);
    var literal = method.Body.Instructions.Single(i =>
        i.OpCode == dnlib.DotNet.Emit.OpCodes.Ldstr && (string)i.Operand == target.Item3);
    literal.Operand = target.Item4;
}
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[1], options);
Console.WriteLine("Updated the four approved client origin literals");
