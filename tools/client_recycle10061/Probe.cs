using System.Reflection;
using System.Runtime.Loader;
using System.Text.Json;
AssemblyLoadContext.Default.Resolving += (c, n) => { var f = Path.Combine(args[0], n.Name + ".dll"); return File.Exists(f) ? c.LoadFromAssemblyPath(f) : null; };
var assembly = AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(args[0], "SoulFlowerRecycle.dll"));
var flow = assembly.GetType("HYJS.Recycle61.Flow")!;
var picker = assembly.GetType("HYJS.Recycle.SelectionControl")!;
var validate = picker.GetMethod("ValidateSelection", BindingFlags.NonPublic | BindingFlags.Static)!;
var selected = flow.GetMethod("Selected", BindingFlags.NonPublic | BindingFlags.Static)!;
var accepted = flow.GetMethod("AcceptedCallback", BindingFlags.NonPublic | BindingFlags.Static)!;
var message = flow.GetMethod("Message", BindingFlags.NonPublic | BindingFlags.Static)!;
var ids = Enumerable.Range(0, 101).Select(x => "guid" + x).ToList();
var visible = new HashSet<string>(ids);
List<string> Validate(List<string> input, int maximum = 100, int slots = 100) => (List<string>)validate.Invoke(null, new object[] { input, visible, maximum, slots })!;
List<string> Snapshot(string[] slots) => (List<string>)selected.Invoke(null, new object[] { slots })!;
bool Reject(Action action) { try { action(); return false; } catch (TargetInvocationException e) { return e.InnerException is InvalidOperationException; } }
foreach (int count in new[] { 0, 1, 4, 5, 40, 99, 100 })
    if (!Validate(ids.Take(count).ToList()).SequenceEqual(ids.Take(count))) throw new Exception("Valid full picker selection differs: " + count);
if (!Reject(() => Validate(ids)) || !Reject(() => Validate(new List<string> { "guid0", "guid0" })) || !Reject(() => Validate(new List<string> { "hidden" })) || !Reject(() => Validate(new List<string> { "" })) || !Reject(() => Validate(ids.Take(100).ToList(), 100, 40))) throw new Exception("Invalid whole reply accepted");
var slots = ids.Take(100).ToArray();
if (!Snapshot(slots).SequenceEqual(slots)) throw new Exception("Request snapshot truncated");
slots[55] = ""; if (Snapshot(slots).Count != 99) throw new Exception("Manual deselection count differs");
slots[55] = slots[99]; if (!Reject(() => Snapshot(slots))) throw new Exception("Duplicate final GUID accepted");
if (!Reject(() => Snapshot(new string[40]))) throw new Exception("Legacy capacity accepted");
foreach (int kind in new[] { 0, 1, 2 })
{
    var body = message.Invoke(null, new object[] { kind, ids.Take(100).ToList() })!;
    var guids = (System.Collections.IList)body.GetType().GetProperty(kind == 2 ? "guids" : "guid")!.GetValue(body)!;
    if (guids.Count != 100 || !guids.Cast<string>().SequenceEqual(ids.Take(100))) throw new Exception("Original protobuf message truncated: " + kind);
}
if (!Reject(() => message.Invoke(null, new object[] { 3, ids.Take(100).ToList() }))) throw new Exception("Unknown protobuf type accepted");
for (int a = 0; a < 2; a++) for (int s = 0; s < 2; s++) for (int o = 0; o < 2; o++)
    if ((bool)accepted.Invoke(null, new object[] { a != 0, s != 0, o != 0 })! != (a == 0 && s == 1 && o == 1)) throw new Exception("Callback once/owner gate differs");
File.WriteAllText(args[1], JsonSerializer.Serialize(new { actualCompiledHelperExecuted = true, validPickerCounts = new[] { 0, 1, 4, 5, 40, 99, 100 }, over100DuplicateHiddenEmptyGuidBadSlotsRejected = true, full100RequestSnapshot = true, actualHeroEquipmentPetOriginalMessagesEach100 = true, unknownMessageTypeRejected = true, manualDeselection99 = true, duplicateRequestRejected = true, callbackOnceOwnerGate = true, unityUIExecuted = false, networkExecuted = false, deviceTested = false }, new JsonSerializerOptions { WriteIndented = true }));
Console.WriteLine("Compiled100 selection/snapshot/callback policy probe passed");
