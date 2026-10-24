using System;
using System.IO;
using System.Reflection;
using System.Runtime.Loader;
using System.Collections.Generic;
using System.Globalization;
using System.Text.Json;

internal static class RoundLiveAuto66Host
{
    static StreamWriter Output;
    static MethodInfo FormatEvent, FormatState;
    static string LastState;
    static void Write(string line) {
        Output.WriteLine(line);
        if(Output.BaseStream.Position > 40*1024*1024) throw new InvalidOperationException("bounded output exceeded");
    }
    static void Event<T>(string kind,T data,object value) {
        object payload=JsonDocument.Parse((string)FormatEvent.Invoke(null,new[]{value})).RootElement;
        var type=data.GetType();
        int round=(int)type.GetField("roundNum").GetValue(data);
        double clock=(double)type.GetField("roundTime").GetValue(data);
        Write(JsonSerializer.Serialize(new{kind,round,time=clock,data=payload}));
    }
    static void Snapshot<T>(T data) {
        string state=(string)FormatState.Invoke(null,new object[]{data});
        if(state==LastState)return;LastState=state;
        Write("{\"kind\":\"state\",\"state\":"+state+"}");
    }
    static object Read(string path,Type type,Assembly serializer) {
        var method=Array.Find(serializer.GetType("ProtoBuf.Serializer",true).GetMethods(),m=>m.Name=="Deserialize"&&m.IsGenericMethodDefinition&&m.GetParameters().Length==1&&m.GetParameters()[0].ParameterType==typeof(Stream));
        using var stream=File.OpenRead(path);return method.MakeGenericMethod(type).Invoke(null,new object[]{stream});
    }
    static int Main(string[] args) {
        if(args.Length!=6)throw new ArgumentException("runtime caller wire start controls output");
        string runtime=Path.GetFullPath(args[0]);Directory.SetCurrentDirectory(runtime);
        CultureInfo.DefaultThreadCurrentCulture=CultureInfo.InvariantCulture;CultureInfo.DefaultThreadCurrentUICulture=CultureInfo.InvariantCulture;
        AssemblyLoadContext.Default.Resolving+=(_,name)=>{string p=Path.Combine(runtime,name.Name+".dll");return File.Exists(p)?AssemblyLoadContext.Default.LoadFromAssemblyPath(p):null;};
        var game=AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(runtime,"Assembly-CSharp.dll"));
        var unity=AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(runtime,"UnityEngine.dll"));
        var logger=unity.GetType("UnityEngine.Debug",true).GetProperty("logger").GetValue(null);logger.GetType().GetProperty("logEnabled").SetValue(logger,false);
        var manager=game.GetType("Dict.DictDataManager",true);manager.GetMethod("Init").Invoke(manager.GetProperty("Instance").GetValue(null),new object[]{"tables"});
        var serializer=AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(runtime,"Assembly-CSharp-firstpass.dll"));
        object wire=Read(args[2],game.GetType("ProtoBuf.Message.SCFightPlayback",true),serializer);
        object start=Read(args[3],game.GetType("ProtoBuf.Message.SCRushFightStart",true),serializer);
        var caller=AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.GetFullPath(args[1]));
        var batch=caller.GetType("NativeBatchRunner",true);FormatEvent=batch.GetMethod("EventData",BindingFlags.NonPublic|BindingFlags.Static);FormatState=batch.GetMethod("State",BindingFlags.NonPublic|BindingFlags.Static);
        var method=caller.GetType("OriginalHeadlessPlaybackCaller",true).GetMethod("RunRushRoundLiveAuto66");
        var parameters=method.GetParameters();
        Type dataType=parameters[4].ParameterType.GenericTypeArguments[0];
        var eventMethod=typeof(RoundLiveAuto66Host).GetMethod("Event",BindingFlags.NonPublic|BindingFlags.Static).MakeGenericMethod(dataType);
        var snapshotMethod=typeof(RoundLiveAuto66Host).GetMethod("Snapshot",BindingFlags.NonPublic|BindingFlags.Static).MakeGenericMethod(dataType);
        var controls=new Dictionary<int,double>();
        foreach(var item in JsonDocument.Parse(File.ReadAllText(args[4])).RootElement.EnumerateArray())controls.Add(item.GetProperty("roundNum").GetInt32(),item.GetProperty("roundTime").GetDouble());
        using(Output=new StreamWriter(args[5])) {
            bool won=(bool)method.Invoke(null,new object[]{wire,start,controls,null,snapshotMethod.CreateDelegate(parameters[4].ParameterType),eventMethod.CreateDelegate(parameters[5].ParameterType)});
            Write(JsonSerializer.Serialize(new{kind="result",attackerWon=won}));
        }
        return 0;
    }
}
