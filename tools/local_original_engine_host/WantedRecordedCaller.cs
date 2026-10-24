// Isolated reflection host: original type8 wrapper, recorded player AI and
// automatic CopyEnemyAI. No original assembly/caller/547 method is modified.
using System;
using System.IO;
using System.Reflection;
using System.Runtime.Loader;
using System.Collections;
using System.Globalization;
using System.Text.Json;

internal static class WantedRecordedCaller
{
    static StreamWriter Output;
    static string Stage;
    static string LastRecord;
    static object Read(string path, Type type, Assembly protobuf)
    {
        var serializer = protobuf.GetType("ProtoBuf.Serializer", true);
        var method = Array.Find(serializer.GetMethods(), m => m.Name == "Deserialize" && m.IsGenericMethodDefinition && m.GetParameters().Length == 1 && m.GetParameters()[0].ParameterType == typeof(Stream));
        using (var stream = File.OpenRead(path)) return method.MakeGenericMethod(type).Invoke(null, new object[] { stream });
    }
    static void Event<T>(string kind, T data, object value)
    {
        if (kind != "original_final_operates") return;
        var row = (IDictionary)value;
        LastRecord = (string)row["protobufBase64"];
        Output.WriteLine(JsonSerializer.Serialize(new { kind, stage=Stage, protobufBase64 = LastRecord, recordAttacker = (bool)row["recordAttacker"], recordDefender = (bool)row["recordDefender"] }));
        Output.Flush();
    }
    static int Main(string[] args)
    {
        if (args.Length != 5) throw new ArgumentException("runtime exact-wire exact-start exact-record output");
        string root = Path.GetFullPath(args[0]);
        CultureInfo.DefaultThreadCurrentCulture = CultureInfo.InvariantCulture;
        CultureInfo.DefaultThreadCurrentUICulture = CultureInfo.InvariantCulture;
        Directory.SetCurrentDirectory(root);
        AssemblyLoadContext.Default.Resolving += (_, name) => {
            string path = Path.Combine(root, name.Name + ".dll");
            return File.Exists(path) ? AssemblyLoadContext.Default.LoadFromAssemblyPath(path) : null;
        };
        var game = AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(root,"Assembly-CSharp.dll"));
        var unity = AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(root,"UnityEngine.dll"));
        var logger = unity.GetType("UnityEngine.Debug",true).GetProperty("logger").GetValue(null);
        logger.GetType().GetProperty("logEnabled").SetValue(logger,false);
        var manager = game.GetType("Dict.DictDataManager",true);
        manager.GetMethod("Init").Invoke(manager.GetProperty("Instance").GetValue(null),new object[]{"tables"});
        var protobuf = AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(root,"Assembly-CSharp-firstpass.dll"));
        // The actual protobuf assembly is resolved from the message's Serializer reference.
        Type wireType = game.GetType("ProtoBuf.Message.SCFightPlayback",true);
        Assembly serializerAssembly = Array.Find(AppDomain.CurrentDomain.GetAssemblies(), a => a.GetType("ProtoBuf.Serializer") != null);
        if (serializerAssembly == null) serializerAssembly = Assembly.Load("protobuf-net");
        object wire = Read(args[1],wireType,serializerAssembly);
        object start = Read(args[2],game.GetType("ProtoBuf.Message.SCRebelFightStart",true),serializerAssembly);
        bool live = args[3].StartsWith("--delay=",StringComparison.Ordinal);
        object record = live ? null : Read(args[3],game.GetType("ProtoBuf.Message.BattleOperate",true),serializerAssembly);
        game.GetType("StaticData",true).GetField("isPveAutoCache").SetValue(null,false);
        Type dataType = game.GetType("BattleData",true);
        object data = Activator.CreateInstance(dataType);
        dataType.GetMethod("SetData",new[]{start.GetType()}).Invoke(data,new[]{start});
        object player = dataType.GetMethod("GetFighter",new[]{typeof(int),typeof(int)}).Invoke(data,new object[]{0,0});
        var caller = AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(root,"LocalOriginalCaller.dll"));
        var run = caller.GetType("OriginalHeadlessPlaybackCaller",true).GetMethod("RunWithInitializedData",BindingFlags.Static|BindingFlags.NonPublic);
        Type eventType = run.GetParameters()[3].ParameterType;
        var method = typeof(WantedRecordedCaller).GetMethod("Event",BindingFlags.Static|BindingFlags.NonPublic).MakeGenericMethod(eventType.GenericTypeArguments[1]);
        Delegate handler = method.CreateDelegate(eventType);
        using (Output = new StreamWriter(args[4])) {
            if (live) {
                if ((bool)player.GetType().GetField("isAuto").GetValue(player)) throw new InvalidOperationException("Typed Wanted source must start manual");
                Stage="live-manual-then-auto";
                var delayed=caller.GetType("OriginalHeadlessPlaybackCaller",true).GetMethod("RunWithManualAutoDelay",BindingFlags.Static|BindingFlags.NonPublic);
                bool liveWon=(bool)delayed.Invoke(null,new object[]{wire,null,null,handler,true,false,data,Double.Parse(args[3].Substring(8),CultureInfo.InvariantCulture)});
                Output.WriteLine(JsonSerializer.Serialize(new{kind="result",stage=Stage,attackerWon=liveWon}));
                string source=args[4]+".source-record.pb";
                File.WriteAllBytes(source,Convert.FromBase64String(LastRecord));
                record=Read(source,game.GetType("ProtoBuf.Message.BattleOperate",true),serializerAssembly);
                data=Activator.CreateInstance(dataType);
                dataType.GetMethod("SetData",new[]{start.GetType()}).Invoke(data,new[]{start});
                player=dataType.GetMethod("GetFighter",new[]{typeof(int),typeof(int)}).Invoke(data,new object[]{0,0});
            }
            Stage="recorded-player-automatic-enemy";
            wireType.GetProperty("optRecord").SetValue(wire,record);
            dataType.GetField("isPlayBack").SetValue(data,true);
            dataType.GetField("operateRecord").SetValue(data,record);
            player.GetType().GetField("isAuto").SetValue(player,true);
            bool won = (bool)run.Invoke(null,new object[]{wire,null,null,handler,false,false,data});
            Output.WriteLine(JsonSerializer.Serialize(new{kind="result",stage=Stage,attackerWon=won}));
        }
        return 0;
    }
}
