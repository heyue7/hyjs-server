// Desktop model using the shipped helper source and rows from the pinned APK.
// This does not load Unity, call the network, or exercise a real device.
using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text;
using HYJS.FragmentBoxes85;

namespace Dict.DataModel
{
    public class TableItem { public string itemId; public int category; }
    public class TableItemUse { public string itemId; public string result; }
}
namespace Dict.Blo
{
    public static class DictItemBlo
    {
        public static readonly Dictionary<string, Dict.DataModel.TableItem> items = new Dictionary<string, Dict.DataModel.TableItem>();
        public static readonly Dictionary<string, List<Dict.DataModel.TableItemUse>> choices = new Dictionary<string, List<Dict.DataModel.TableItemUse>>();
        public static Dict.DataModel.TableItem GetTableItem(string id)
        {
            Dict.DataModel.TableItem item;
            return items.TryGetValue(id, out item) ? item : null;
        }
        public static List<Dict.DataModel.TableItemUse> GetTableItemUseList(string id)
        {
            List<Dict.DataModel.TableItemUse> rows;
            return choices.TryGetValue(id, out rows) ? rows : null;
        }
    }
}
namespace UI
{
    public class Page { public Dictionary<string, string> options = new Dictionary<string, string>(); }
}

static class Program
{
    static int assertions;
    static void Check(bool condition, string message)
    {
        assertions++;
        if (!condition) throw new Exception(message);
    }
    static void LoadTables(string itemPath, string usePath)
    {
        foreach (string line in File.ReadAllLines(itemPath).Skip(5))
        {
            string[] values = line.Split('\t');
            int category;
            if (values.Length > 9 && values[0].Length > 0 && Int32.TryParse(values[9], out category))
                Dict.Blo.DictItemBlo.items[values[0]] = new Dict.DataModel.TableItem { itemId = values[0], category = category };
        }
        foreach (string line in File.ReadAllLines(usePath).Skip(5))
        {
            string[] values = line.Split('\t');
            if (values.Length < 3 || values[0].Length == 0) continue;
            List<Dict.DataModel.TableItemUse> list;
            if (!Dict.Blo.DictItemBlo.choices.TryGetValue(values[1], out list))
                Dict.Blo.DictItemBlo.choices[values[1]] = list = new List<Dict.DataModel.TableItemUse>();
            list.Add(new Dict.DataModel.TableItemUse { itemId = values[1], result = values[2] });
        }
    }
    static UI.Page Open(string options, UI.Page existing)
    {
        UI.Page page = existing ?? new UI.Page();
        page.options.Clear();
        foreach (string part in options.Split('&'))
        {
            string[] pair = part.Split('=');
            if (pair.Length == 2) page.options[pair[0]] = pair[1];
        }
        return page;
    }
    static long AddOne(long selected, long cap) { return selected < cap ? selected + 1 : cap; }
    static long AddTen(long selected, long cap) { return selected + 10 <= cap ? selected + 10 : cap; }

    // Model the relevant original transport sequence. The actual CIL is separately pinned.
    sealed class NativeRequestModel
    {
        public byte[] packet;
        public bool busy;
        public int localCost, localGain, callbacks;
        public bool Send(byte[] encoded)
        {
            if (busy) return false;
            busy = true;
            packet = (byte[])encoded.Clone();
            return true;
        }
        public void Timeout() { Check(busy, "timeout without send"); busy = false; }
        public void Retry() { Check(Send(packet), "exact timeout retry was not accepted"); }
        public void Response(bool success, int cost, int gain)
        {
            Check(busy, "response without an outstanding send");
            busy = false;
            callbacks++;
            if (success) { localCost += cost; localGain += gain; }
        }
    }
    static void Main(string[] args)
    {
        LoadTables(args[0], args[1]);
        string[] expected = { "item_120", "item_121", "item_122", "item_156", "item_348" };
        var actual = Dict.Blo.DictItemBlo.items.Keys.Where(FragmentBoxes.IsEligible).OrderBy(x => x).ToArray();
        Check(actual.SequenceEqual(expected.OrderBy(x => x)), "APK table eligible set");
        Check(!FragmentBoxes.IsEligible(null) && !FragmentBoxes.IsEligible("item_unknown"), "unknown item");
        Dict.Blo.DictItemBlo.items["mixed"] = new Dict.DataModel.TableItem { category = 5 };
        Dict.Blo.DictItemBlo.choices["mixed"] = new List<Dict.DataModel.TableItemUse> {
            new Dict.DataModel.TableItemUse { result = "frag_hero_001,1" },
            new Dict.DataModel.TableItemUse { result = "hero_001,1" } };
        Check(!FragmentBoxes.IsEligible("mixed"), "mixed pool");
        Dict.Blo.DictItemBlo.choices["mixed"][1].result = "frag_hero_001,1;frag_hero_002,1";
        Check(!FragmentBoxes.IsEligible("mixed"), "native list incompatible multi-result row");
        Dict.Blo.DictItemBlo.choices["mixed"][1].result = "frag_hero_001,0";
        Check(!FragmentBoxes.IsEligible("mixed"), "zero reward");
        string target = "item_120";
        foreach (int count in new[] { 1, 10, 32, 100 })
        {
            long cap = FragmentBoxes.Limit(150), selected = 1;
            while (selected + 10 <= count) selected = AddTen(selected, cap);
            while (selected < count) selected = AddOne(selected, cap);
            string options = FragmentBoxes.AppendCount("id=" + target, selected, target);
            UI.Page page = Open(options, null);
            Check(FragmentBoxes.RequestedCount(page, target) == count, "selected count " + count);
            int selectedIndex = count == 32 ? 3 : 1;
            byte[] encoded = Encoding.UTF8.GetBytes(target + ":" + FragmentBoxes.RequestedCount(page, target) + ":" + selectedIndex);
            Check(Encoding.UTF8.GetString(encoded) == target + ":" + count + ":" + selectedIndex, "selected index/count context");
        }
        long stockCap = FragmentBoxes.Limit(32), stockSelected = 1;
        for (int i = 0; i < 12; i++) stockSelected = AddTen(stockSelected, stockCap);
        Check(stockSelected == 32 && AddOne(stockSelected, stockCap) == 32, "stock cap");
        Check(FragmentBoxes.Limit(200) == 100 && FragmentBoxes.Limit(1) == 1, "server cap");
        UI.Page reused = Open(FragmentBoxes.AppendCount("id=" + target, 32, target), null);
        Check(FragmentBoxes.RequestedCount(reused, target) == 32, "first opening");
        Open("id=" + target, reused);
        Check(FragmentBoxes.RequestedCount(reused, target) == 1, "cancel/reopen clears old options");
        Open(FragmentBoxes.AppendCount("id=item_121", 10, "item_121"), reused);
        Check(FragmentBoxes.RequestedCount(reused, "item_121") == 10 && FragmentBoxes.RequestedCount(reused, target) == 1,
              "item context changed");
        UI.Page other = Open(FragmentBoxes.AppendCount("id=" + target, 100, target), null);
        Check(FragmentBoxes.RequestedCount(other, target) == 100 && FragmentBoxes.RequestedCount(reused, "item_121") == 10,
              "separate page contexts");
        string nonTarget = Dict.Blo.DictItemBlo.items.Keys.First(id =>
            Dict.Blo.DictItemBlo.items[id].category == 5 && !FragmentBoxes.IsEligible(id));
        string unchanged = FragmentBoxes.AppendCount("id=" + nonTarget, 32, nonTarget);
        Check(unchanged == "id=" + nonTarget && FragmentBoxes.RequestedCount(Open(unchanged, null), nonTarget) == 1,
              "non-fragment selection box");
        Check(FragmentBoxes.AppendCount("id=" + target, 0, target) == "id=" + target &&
              FragmentBoxes.AppendCount("id=" + target, 101, target) == "id=" + target, "invalid count not forwarded");

        var network = new NativeRequestModel();
        byte[] request = Encoding.UTF8.GetBytes("opaque-serialized-request-32-index-3");
        Check(network.Send(request), "first send");
        Check(!network.Send(request), "busy transport rejects second send");
        network.Timeout();
        Check(network.callbacks == 0 && network.localCost == 0 && network.localGain == 0, "timeout has no local consumption");
        network.Retry();
        Check(network.packet.SequenceEqual(request), "timeout retry retains request bytes");
        network.Response(true, 32, 32);
        Check(network.callbacks == 1 && network.localCost == 32 && network.localGain == 32, "one real response consumption");
        var rejected = new NativeRequestModel();
        Check(rejected.Send(request), "rejection request accepted");
        rejected.Response(false, 32, 32);
        Check(rejected.callbacks == 1 && rejected.localCost == 0 && rejected.localGain == 0, "failure does not consume locally");
        Console.WriteLine("PASS " + assertions + " assertions; original APK tables, compiled helper, desktop-only transport model");
    }
}
