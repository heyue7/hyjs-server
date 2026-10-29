using System;
using System.IO;
using System.Reflection;
using LitJson;

// Exercises the built helper rather than a copy of its parser.
static class TradeControllerParseChecks
{
    static int count;
    static Type normalizer;
    static MethodInfo parser;
    static object owner;
    static void Check(bool condition,string message)
    { count++; if (!condition) throw new Exception(message); }
    static object Parse(string path)
    {
        string raw=File.ReadAllText(path);
        string text=(string)normalizer.GetMethod("Normalize").Invoke(null,new object[]{raw});
        JsonData root=JsonMapper.ToObject(text);
        return parser.Invoke(null,new object[]{owner,root["data"]});
    }
    static void Main(string[] args)
    {
        Check(args.Length>=3,"fixture paths");
        Assembly helper=Assembly.LoadFrom(Path.Combine(AppContext.BaseDirectory,"SoulFlowerDesertV4.dll"));
        normalizer=helper.GetType("HYJS.DesertV4.NestedArrayJson",true);
        Type controller=helper.GetType("HYJS.DesertV4.TradeController",true);
        Type session=helper.GetType("HYJS.DesertV4.FieldWire+Session",true);
        owner=Activator.CreateInstance(session,BindingFlags.Instance|BindingFlags.NonPublic,null,
            new object[]{2L,"0123456789abcdef0123456789abcdef","http://127.0.0.1:28979"},null);
        parser=controller.GetMethod("Parse",BindingFlags.Static|BindingFlags.NonPublic);
        Check(parser!=null,"actual parser");
        object q=Parse(args[0]); Type t=q.GetType();
        Check((string)t.GetField("status").GetValue(q)=="none","empty status");
        q=Parse(args[1]); t=q.GetType();
        Check((string)t.GetField("status").GetValue(q)=="open","open status");
        long[][] offers=(long[][])t.GetField("offers").GetValue(q);
        Check(offers.Length==2 && offers[0].Length==6 && offers[1].Length==6,"matrix");
        Check(offers[0][0]==Int64.MaxValue && offers[0][1]==1234567890123L &&
              offers[1][0]==7 && offers[1][5]==9,"amount and order");
        Check((long)t.GetField("credit").GetValue(q)==Int64.MaxValue,"credit balance");
        bool[] confirmed=(bool[])t.GetField("confirmed").GetValue(q);
        Check(!confirmed[0] && confirmed[1],"confirmations");
        for(int i=2;i<args.Length;i++)
        {
            bool rejected=false;
            try { Parse(args[i]); } catch(TargetInvocationException) { rejected=true; }
            Check(rejected,"invalid quote rejected");
        }
        Console.WriteLine("PASS "+count+" actual compiled TradeController parsing checks");
    }
}
