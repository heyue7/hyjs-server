using System.Reflection;
using System.Runtime.Loader;
using System.Text.Json;
AssemblyLoadContext.Default.Resolving += (c,n) => {var p=Path.Combine(args[0],n.Name+".dll");return File.Exists(p)?c.LoadFromAssemblyPath(p):null;};
var first=AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(args[0],"Assembly-CSharp-firstpass.dll"));
var main=AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(args[0],"Assembly-CSharp.dll"));
var helper=AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(args[0],"SoulFlowerReWelfare.dll"));
var type=main.GetType("ProtoBuf.Message.ActivityFortuneCatData")!;
if(type.GetProperties().Any(p=>p.GetCustomAttributesData().Any(a=>a.AttributeType.Name=="ProtoMemberAttribute"&&(int)a.ConstructorArguments[0].Value! ==100)))throw new Exception("Original schema tag100 collision");
var deserialize=first.GetType("ProtoBuf.Serializer")!.GetMethods().Single(m=>m.Name=="Deserialize"&&m.IsGenericMethodDefinition&&m.GetParameters().Length==1&&m.GetParameters()[0].ParameterType==typeof(Stream)).MakeGenericMethod(type);
object Decode(byte[] raw){using var stream=new MemoryStream(raw);return deserialize.Invoke(null,new object[]{stream})!;}
var quote=helper.GetType("HYJS.ReWelfare.ReWelfareBridge")!.GetMethod("ReturnQuote",BindingFlags.Static|BindingFlags.NonPublic)!;
long Read(byte[] bytes)=>(long)quote.Invoke(null,new object?[]{null,Decode(bytes)})!;
if(Read(new byte[]{0x08,0x05,0xa0,0x06,0xf8,0x06})!=888)throw new Exception("Actual protobuf-net scalar quote differs");
if(Read(new byte[]{0x08,0x05})!=0)throw new Exception("Missing quote accepted");
if(Read(new byte[]{0x08,0x05,0xa0,0x06,0xf8,0x06,0xa0,0x06,0x01})!=0)throw new Exception("Duplicate quote accepted");
File.WriteAllText(args[1],JsonSerializer.Serialize(new { originalSchemaWithoutTag100=true,actualShippedProtobufNetDecodedQuote=888,actualHelperReturnedQuote=true,missingQuoteRejected=true,duplicateQuoteRejected=true,UnityUIExecuted=false },new JsonSerializerOptions{WriteIndented=true}));
Console.WriteLine("PASS actual shipped protobuf-net + actual commerce helper quote extension");
