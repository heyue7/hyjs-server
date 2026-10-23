using System.Reflection;
using System.Runtime.Loader;
using System.Text.Json;
if(args.Length!=2) throw new Exception("managedDirectory outputJSON");
AssemblyLoadContext.Default.Resolving += (c,n) => {var p=Path.Combine(args[0],n.Name+".dll");return File.Exists(p)?c.LoadFromAssemblyPath(p):null;};
var first=AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(args[0],"Assembly-CSharp-firstpass.dll"));
var main=AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(args[0],"Assembly-CSharp.dll"));
var model=first.GetType("ProtoBuf.Meta.RuntimeTypeModel")!;var instance=model.GetProperty("Default",BindingFlags.Public|BindingFlags.Static)!.GetValue(null)!;
// Shipped Unity protobuf-net has no dynamic AutoCompile property.
var serialize=instance.GetType().GetMethods().Single(m=>m.Name=="Serialize" && m.GetParameters().Length==2 && m.GetParameters()[0].ParameterType==typeof(Stream));
var results=new List<object>();
var extensible=first.GetType("ProtoBuf.Extensible")!;
var append=extensible.GetMethods().Single(m=>m.Name=="AppendValue"&&m.IsGenericMethodDefinition&&m.GetParameters().Length==3);
var values=extensible.GetMethods().Single(m=>m.Name=="GetValues"&&m.IsGenericMethodDefinition&&m.GetParameters().Length==2);
byte[] Encode(object value){using var stream=new MemoryStream();serialize.Invoke(instance,new[]{stream,value});return stream.ToArray();}
object Decode(Type t,byte[] raw){using var stream=new MemoryStream(raw);return first.GetType("ProtoBuf.Serializer")!.GetMethods().Single(m=>m.Name=="Deserialize"&&m.IsGenericMethodDefinition&&m.GetParameters().Length==1&&m.GetParameters()[0].ParameterType==typeof(Stream)).MakeGenericMethod(t).Invoke(null,new object[]{stream})!;}
foreach(var typeName in new[]{"ProtoBuf.Message.CSGetOpenServerReward","ProtoBuf.Message.CSUFPlayerBuyItem"}) {
 var t=main.GetType(typeName)!;if(t.GetField("hyjsBatchCount")!=null)throw new Exception("Probe requires original no-added-field class");
 var baseline=Activator.CreateInstance(t)!;
 if(typeName.Contains("OpenServer"))t.GetProperty("itemId")!.SetValue(baseline,7);else {t.GetProperty("shopItemId")!.SetValue(baseline,"offer");t.GetProperty("npcId")!.SetValue(baseline,"npc");}
 var zero=Encode(baseline);append.MakeGenericMethod(typeof(int)).Invoke(null,new object[]{baseline,100,10});var ten=Encode(baseline);
 var suffix=new byte[]{0xa0,0x06,0x0a};if(ten.Length!=zero.Length+3 || !ten.AsSpan(0,zero.Length).SequenceEqual(zero) || !ten.AsSpan(zero.Length).SequenceEqual(suffix))throw new Exception("wire tag100 differs");
 var decoded=Decode(t,ten);var got=((System.Collections.IEnumerable)values.MakeGenericMethod(typeof(int)).Invoke(null,new object[]{decoded,100})!).Cast<int>().ToArray();
 if(!got.SequenceEqual(new[]{10})||!Encode(decoded).SequenceEqual(ten))throw new Exception("count extension roundtrip failed");
 append.MakeGenericMethod(typeof(int)).Invoke(null,new object[]{decoded,100,11});var duplicate=((System.Collections.IEnumerable)values.MakeGenericMethod(typeof(int)).Invoke(null,new object[]{decoded,100})!).Cast<int>().ToArray();
 if(duplicate.Length==0)throw new Exception("duplicate count unreadable");
 results.Add(new{type=typeName,zeroHex=Convert.ToHexString(zero),tenHex=Convert.ToHexString(ten),tag100Present=true,zeroOmitted=true,originalClassWithoutAddedFields=true,byteRoundtrip=true,duplicateReadValues=duplicate});
}
var resource=main.GetType("ProtoBuf.Message.BaseResource")!;
var listType=typeof(List<>).MakeGenericType(resource);var curve=(System.Collections.IList)Activator.CreateInstance(listType)!;
foreach(var price in new[]{2,3,5}){var r=Activator.CreateInstance(resource)!;resource.GetProperty("id")!.SetValue(r,"diamond");resource.GetProperty("num")!.SetValue(r,price);curve.Add(r);}
foreach(var name in new[]{"ProtoBuf.Message.ActivityShopItem","ProtoBuf.Message.ShopItem"}) {
 var t=main.GetType(name)!;if(t.GetField("hyjsBatchCosts")!=null)throw new Exception("Probe requires original no-added-field class");
 var offer=Activator.CreateInstance(t)!;if(name.EndsWith("ActivityShopItem"))t.GetProperty("index")!.SetValue(offer,1);else t.GetProperty("id")!.SetValue(offer,"sku");
 using var wire=new MemoryStream();wire.Write(Encode(offer));
 // Independently construct repeated field100 wire, not a modified message-class serializer.
 foreach(var r in curve){var body=Encode(r!);if(body.Length>=128)throw new Exception("probe fixture too long");wire.Write(new byte[]{0xa2,0x06,(byte)body.Length});wire.Write(body);}
 var bytes=wire.ToArray();var decoded=Decode(t,bytes);var merged=((System.Collections.IEnumerable)values.MakeGenericMethod(resource).Invoke(null,new object[]{decoded,100})!).Cast<object>().ToArray();
 var groups=((System.Collections.IEnumerable)values.MakeGenericMethod(listType).Invoke(null,new object[]{decoded,100})!).Cast<System.Collections.IEnumerable>().ToArray();var items=groups.SelectMany(g=>g.Cast<object>()).ToArray();
 if(!items.Select(x=>(int)resource.GetProperty("num")!.GetValue(x)!).SequenceEqual(new[]{2,3,5}) || !Encode(decoded).SequenceEqual(bytes))throw new Exception("unknown curve byte roundtrip failed values="+String.Join(",",items.Select(x=>(int)resource.GetProperty("num")!.GetValue(x)!))+" byteEqual="+Encode(decoded).SequenceEqual(bytes));
 results.Add(new{type=t.FullName,wireHex=Convert.ToHexString(bytes),tag100RepeatedMessageRoundtrip=true,costs=items.Length,originalClassWithoutAddedFields=true,byteRoundtrip=true});
}
var helperPath=Path.Combine(args[0],"HYJS.BulkPurchase.dll");
if(File.Exists(helperPath)) {
 var helper=AssemblyLoadContext.Default.LoadFromAssemblyPath(helperPath);var api=helper.GetType("HYJS.BulkPurchase.BulkPurchase")!;
 var appendBatch=api.GetMethod("AppendBatch",BindingFlags.NonPublic|BindingFlags.Static)!;
 var countType=main.GetType("ProtoBuf.Message.CSGetOpenServerReward")!;
 bool Reject(Action action){try{action();return false;}catch(TargetInvocationException e){return e.InnerException is InvalidOperationException;}}
 foreach(int n in new[]{0,101})if(!Reject(()=>appendBatch.Invoke(null,new object[]{Activator.CreateInstance(countType)!,n})))throw new Exception("invalid count accepted");
 foreach(int n in new[]{1,100}) {var request=Activator.CreateInstance(countType)!;appendBatch.Invoke(null,new object[]{request,n});var seen=((System.Collections.IEnumerable)values.MakeGenericMethod(typeof(List<int>)).Invoke(null,new object[]{request,100})!).Cast<List<int>>().SelectMany(x=>x).ToArray();if(!seen.SequenceEqual(new[]{n}))throw new Exception("bounded count wire differs");if(!Reject(()=>appendBatch.Invoke(null,new object[]{request,n})))throw new Exception("duplicate batch append accepted");}
 var curveMethod=api.GetMethod("Curve",BindingFlags.NonPublic|BindingFlags.Static)!;
 var curveType=main.GetType("ProtoBuf.Message.ActivityShopItem")!;
 object CurveOffer(int count,int price,string id){var offer=Activator.CreateInstance(curveType)!;for(int i=0;i<count;i++){var r=Activator.CreateInstance(resource)!;resource.GetProperty("id")!.SetValue(r,id);resource.GetProperty("num")!.SetValue(r,price);append.MakeGenericMethod(resource).Invoke(null,new object[]{offer,100,r});}return offer;}
 if(curveMethod.Invoke(null,new[]{Activator.CreateInstance(curveType)!})!=null)throw new Exception("absent curve differs");
 var full=(System.Collections.IList)curveMethod.Invoke(null,new[]{CurveOffer(100,1,"diamond")})!;if(full.Count!=100)throw new Exception("100 curve entries rejected");
 foreach(var invalid in new[]{CurveOffer(101,1,"diamond"),CurveOffer(1,0,"diamond"),CurveOffer(1,-1,"diamond"),CurveOffer(1,1,"")})if(!Reject(()=>curveMethod.Invoke(null,new[]{invalid})))throw new Exception("invalid curve accepted");
 results.Add(new{helperCountBounds1Through100=true,duplicateAppendRejected=true,curveBounds100=true,curve101Rejected=true,invalidCurveRejected=true});
 var selector=api.GetMethod("SelectQuantity")!;
 if(selector.GetParameters().Length!=7 || selector.ReturnType!=typeof(bool))throw new Exception("GGO selector signature differs");
 var stateType=helper.GetType("HYJS.BulkPurchase.QuantitySession")!;
 var state=System.Runtime.CompilerServices.RuntimeHelpers.GetUninitializedObject(stateType);
 stateType.GetField("Curve")!.SetValue(state,curve);
 var costs=api.GetMethod("Costs",BindingFlags.NonPublic|BindingFlags.Static)!;
 var total=(System.Collections.IList)costs.Invoke(null,new object[]{state,2})!;
 if((int)resource.GetProperty("num")!.GetValue(total[0])! !=3)throw new Exception("authoritative curve was multiplied as unit price");
 var copy=api.GetMethod("Copy",BindingFlags.NonPublic|BindingFlags.Static)!;
 var huge=(System.Collections.IList)Activator.CreateInstance(listType)!;var hugeRow=Activator.CreateInstance(resource)!;resource.GetProperty("id")!.SetValue(hugeRow,"diamond");resource.GetProperty("num")!.SetValue(hugeRow,int.MaxValue);huge.Add(hugeRow);
 bool overflow=false;try{copy.Invoke(null,new object[]{huge,2});}catch(TargetInvocationException e){overflow=e.InnerException is InvalidOperationException;}
 if(!overflow)throw new Exception("protocol count overflow was not rejected");
 results.Add(new{helperSelectorResolved=true,authoritativeTwoUnitTotal=3,unitWouldBe4=true,overflowRejected=true,unityUIExecuted=false});
}
File.WriteAllText(args[1],JsonSerializer.Serialize(results,new JsonSerializerOptions{WriteIndented=true}));Console.WriteLine("Actual shipped protobuf-net tag100 serialization passed for both messages");
