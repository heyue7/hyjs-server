using dnlib.DotNet;
using dnlib.DotNet.Emit;
using System.Reflection.PortableExecutable;

if (args.Length != 2) throw new ArgumentException("input and output managed DLL required");
var raw = File.ReadAllBytes(args[0]);
using var module = ModuleDefMD.Load(raw);
using var pe = new PEReader(new MemoryStream(raw, false));
var specs = new[] {
    (Owner: "Room.RCUINormalNPC", Name: "OpenPage", Helper: "CatalogueReady", At: 0x15u),
    (Owner: "AwakeDiffItem", Name: "OnChallengeClick", Helper: "EntryReady", At: 0u),
};
foreach (var spec in specs) {
    var method = module.GetTypes().Single(t => t.FullName == spec.Owner)
        .Methods.Single(m => m.Name == spec.Name);
    if (!method.HasBody || method.IsStatic) throw new InvalidOperationException("Original gate shape differs");
    var code = method.Body.Instructions;
    var start = code.Select((x, i) => (x, i)).Single(p => p.x.Offset == spec.At).i;
    bool Call(int offset, OpCode opcode, string owner, string name) =>
        code[start + offset].OpCode == opcode && code[start + offset].Operand is IMethod m &&
        m.DeclaringType.FullName == owner && m.Name == name;
    if (!Call(0, OpCodes.Call, "GameConfig", "get_Instance") ||
        !Call(1, OpCodes.Callvirt, "GameConfig", "get_IsAssetFull") ||
        code[start+2].OpCode != OpCodes.Ldarg_0 ||
        !Call(3, OpCodes.Call, "HYJS.LocalAssets.LocalAssetReady", spec.Helper) ||
        code[start+4].OpCode != OpCodes.Brtrue || code[start+4].Offset != spec.At+16)
        throw new InvalidOperationException("Expected exact inherited full getter / bridge / branch pipeline");
    // No metadata or method writer runs. The exact original PE is copied and
    // only the sixteen-byte gate-producing expression is replaced. The original
    // branch, all later predicates, locals and raw exception sections stay put.
    var rva = (int)method.RVA;
    var section = pe.PEHeaders.SectionHeaders.Single(s => rva >= s.VirtualAddress &&
        rva < s.VirtualAddress + Math.Max(s.VirtualSize, s.SizeOfRawData));
    var methodAt = section.PointerToRawData + rva - section.VirtualAddress;
    var headerSize = (raw[methodAt] & 3) == 2 ? 1 : (BitConverter.ToUInt16(raw, methodAt) >> 12) * 4;
    var at = methodAt + headerSize + (int)spec.At;
    raw[at] = 0x17; // ldc.i4.1: existing brtrue always takes the original continuation.
    Array.Clear(raw, at + 1, 15); // nop; no getter, bridge, quality or party checks.
    Console.WriteLine($"{spec.Owner}::{spec.Name}: IL[{spec.At:x},{spec.At+16:x}) only");
}
File.WriteAllBytes(args[1], raw);
