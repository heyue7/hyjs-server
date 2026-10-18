using System;
using System.Linq;
using dnlib.DotNet;
using dnlib.DotNet.Emit;
using dnlib.DotNet.Writer;

if (args.Length != 3) throw new ArgumentException("input DLL, bridge DLL, output DLL required");
using var module = ModuleDefMD.Load(args[0]);
using var bridge = ModuleDefMD.Load(args[1]);
var close = module.GetTypes().Single(t => t.FullName == "UI.EquipRemodelPage").Methods.Single(m => m.Name == "DoClose");
if (close.Body.ExceptionHandlers.Count != 0 || close.Body.Instructions.Count != 1 || close.Body.Instructions[0].OpCode != OpCodes.Ret)
    throw new Exception("Review changed original weapon remodel close entry");
var entry = bridge.GetTypes().Single(t => t.FullName == "HYJS.WeaponRefresh.WeaponRefresh").Methods.Single(m => m.Name == "AfterClose");
close.Body = new CilBody();
close.Body.Instructions.Add(Instruction.Create(OpCodes.Call, module.Import(entry)));
close.Body.Instructions.Add(Instruction.Create(OpCodes.Ret));
var options = new ModuleWriterOptions(module);
options.MetadataOptions.Flags |= MetadataFlags.PreserveAll | MetadataFlags.KeepOldMaxStack;
module.Write(args[2], options);
Console.WriteLine("Patched only original EquipRemodelPage.DoClose; no inventory or server mutation");
