using System;
using System.Collections;
using System.Globalization;
using System.IO;
using LitJson;
using HYJS.DesertV4;

static class TradeJsonChecks
{
    static int count;
    static void Check(bool condition,string message)
    { count++; if (!condition) throw new Exception(message); }
    static JsonData Parse(string raw)
    { return JsonMapper.ToObject(NestedArrayJson.Normalize(raw)); }
    static JsonData Values(JsonData item)
    {
        Check(item!=null && item.IsObject,"nested array wrapper");
        IDictionary row=(IDictionary)item;
        Check(row.Count==1 && row.Contains(NestedArrayJson.Key),"only wrapper key");
        JsonData result=item[NestedArrayJson.Key];
        Check(result!=null && result.IsArray,"wrapper array");
        return result;
    }
    static long Number(JsonData item)
    { return Int64.Parse(item.ToString(),CultureInfo.InvariantCulture); }
    static void ExpectRejected(string raw)
    {
        bool rejected=false;
        try { NestedArrayJson.Normalize(raw); }
        catch (ArgumentException) { rejected=true; }
        Check(rejected,"invalid envelope rejected");
    }
    static void Main(string[] args)
    {
        Check(args.Length==2,"fixture paths");
        JsonData empty=Parse(File.ReadAllText(args[0]));
        Check(Number(empty["code"])==0,"empty result code");
        JsonData e=empty["data"];
        Check((string)e["status"]=="none","empty status");
        Check(Number(e["serverTimeMs"])>0,"empty server time");
        Check(e["offers"].Count==2,"empty offers rows");
        for (int i=0;i<2;i++)
        {
            JsonData values=Values(e["offers"][i]);
            Check(values.Count==6,"empty six colors");
            for (int j=0;j<6;j++) Check(Number(values[j])==0,"empty amounts");
        }
        JsonData full=Parse(File.ReadAllText(args[1]))["data"];
        Check((string)full["status"]=="open","open status");
        Check((string)full["note"]=="brackets [ ] and quote \" and slash \\ preserved","quoted contents");
        Check(full["offers"].Count==2,"open offer rows");
        JsonData one=Values(full["offers"][0]), two=Values(full["offers"][1]);
        Check(one.Count==6 && two.Count==6,"open six colors");
        Check(Number(one[0])==Int64.MaxValue && Number(one[1])==1234567890123L,"int64 amounts");
        Check(Number(two[0])==7 && Number(two[5])==9,"row order");
        JsonData deeply=Parse("{\"x\":[[[]],{\"value\":[1,2]}]}");
        JsonData first=Values(deeply["x"][0]);
        Check(Values(first[0]).Count==0,"deeper empty arrays");
        Check(Number(deeply["x"][1]["value"][1])==2,"array within object");
        ExpectRejected("{\"x\":[1}");
        ExpectRejected("{\"x\":\"unterminated}");
        ExpectRejected(new String(' ',2*1024*1024+1));
        Console.WriteLine("PASS "+count+" actual original LitJson trade response assertions");
    }
}
