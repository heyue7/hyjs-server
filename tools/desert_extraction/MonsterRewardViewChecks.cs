using System;
using System.Collections.Generic;
using LitJson;
using ProtoBuf.Message;
using HYJS.DesertV4;

namespace ProtoBuf.Message
{
    public sealed class SimpleItem { public string id; public int num; }
    public sealed class ResultObject
    {
        public List<SimpleItem> simps=new List<SimpleItem>();
        public List<object> equips=new List<object>(),treas=new List<object>(),wars=new List<object>(),pets=new List<object>();
    }
}

static class MonsterRewardViewChecks
{
    static int count;
    static void Check(bool condition,string message) { count++; if (!condition) throw new Exception(message); }
    static MonsterRewardView Parse(string status,string chips)
    { return MonsterRewardView.FromReceipt("fight",JsonMapper.ToObject("{\"deliveryStatus\":\""+status+"\",\"chips\":"+chips+"}")); }
    static void Reject(string status,string chips)
    {
        bool failed=false; try { Parse(status,chips); } catch (Exception) { failed=true; }
        Check(failed,"invalid receipt accepted");
    }
    static void Main()
    {
        var native=new ResultObject();
        native.simps.Add(new SimpleItem { id="coin",num=10 });
        native.wars.Add(new object());
        var result=Parse("delivered","[0,0,1,0,0,0]");
        Check(Object.ReferenceEquals(native,result.Project("different",native)),"wrong fight must not show chip");
        var display=result.Project("fight",native);
        Check(!Object.ReferenceEquals(native,display),"display must be separate from original inventory response");
        Check(display.simps.Count==2 && display.simps[0].id=="coin" && display.simps[1].id=="item_9703" && display.simps[1].num==1,"ordinary reward plus blue chip");
        Check(native.simps.Count==1 && native.simps[0].id=="coin" && native.wars.Count==1 && display.wars[0]==native.wars[0],"original resources and role stay intact");
        display.simps.RemoveAt(0); Check(native.simps.Count==1,"display list must not mutate inventory list");
        for (int i=0;i<6;i++)
        {
            string[] values={"0","0","0","0","0","0"}; values[i]="1";
            ResultObject color=Parse("delivered","["+String.Join(",",values)+"]").Project("fight",native);
            Check(color.simps[1].id=="item_"+(9701+i),"wrong quality mapping");
        }
        var partial=Parse("pending_delivery","[0,0,0,1,0,0]").Project("fight",native);
        Check(partial.simps.Count==2 && partial.simps[1].id=="item_9704","delivered chip shown even when ordinary reward is pending");
        Check(Object.ReferenceEquals(native,Parse("pending_delivery","[0,0,0,0,0,0]").Project("fight",native)),"old pending receipt has no shown chip");
        Check(Object.ReferenceEquals(native,Parse("lost","[0,0,0,0,0,0]").Project("fight",native)),"defeat has no chip");
        Reject("lost","[1,0,0,0,0,0]"); Reject("delivered","[2,0,0,0,0,0]");
        Reject("delivered","[1,1,0,0,0,0]"); Reject("delivered","[-1,0,0,0,0,0]");
        Reject("delivered","[0,0,0,0,0]"); Reject("unknown","[0,0,0,0,0,0]");
        Console.WriteLine("PASS "+count+" monster reward projection checks");
    }
}
