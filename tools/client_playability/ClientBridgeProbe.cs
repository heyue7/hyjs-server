// Explicit developer UI hot-load only. No game requests, settings or account data.
using System;
using System.Collections;
using System.IO;
using System.Reflection;
using System.Text;
using UnityEngine;

public static class NativeBatchRunner
{
    static bool activityProbed;
    public static bool TryStart(object candidate)
    {
        try
        {
            using (StreamWriter output = new StreamWriter(Path.Combine(Application.persistentDataPath, "client-bridge-diagnostics.txt"), false, Encoding.UTF8))
            {
                output.AutoFlush = true;
                output.WriteLine("ClientBridgeProbe20261018R3 v3 UTC=" + DateTime.UtcNow.ToString("o"));
                output.WriteLine("Explicit UI diagnostic; original PlaybackKit request intercepted");
                foreach (Assembly assembly in AppDomain.CurrentDomain.GetAssemblies()) Describe(output, assembly);
                Inspect(output, "SoulFlowerActivity", "HYJS.ActivitySafety.ActivityGuard", true);
                Inspect(output, "SoulFlowerDisplay", "HYJS.Display.DisplaySettings", false);
                Snapshot(output, "UI.UIActivity");
                Snapshot(output, "UI.UIActivityPanel");
                Snapshot(output, "UI.QualityBar");
                InspectOriginalIL(output);
                output.WriteLine("[initial capture complete; delayed activity watcher starts]");
            }
            GameObject host = new GameObject("ClientBridgeProbeWatcher");
            UnityEngine.Object.DontDestroyOnLoad(host);
            host.AddComponent<ClientBridgeProbeWatch>();
        }
        catch (Exception) { }
        // Always suppress the original diagnostic PlaybackKit network branch,
        // including when writing the report or reflecting a bridge fails.
        return true;
    }

    public static void LogRoomPosition(object net, object response, int phase) { }
    public static void ApplyActivityInformation(object panel, object packet, bool success) { }
    public static void ApplyExpeditionPreview(object panel) { }
    public static void ObservePVEUIPress(int camp, string actorUid, int skillIndex) { }
    public static void ObservePVEUIBegin(object skill) { }
    public static void AfterPVEUIUpdate(object manager) { }

    static void Failure(StreamWriter output, Exception failure)
    {
        output.WriteLine(failure.ToString());
        ReflectionTypeLoadException loader = failure as ReflectionTypeLoadException;
        if (loader != null && loader.LoaderExceptions != null)
            foreach (Exception cause in loader.LoaderExceptions)
                if (cause != null) output.WriteLine("LoaderException=" + cause.ToString());
    }

    static void Describe(StreamWriter output, Assembly assembly)
    {
        try { output.WriteLine("Assembly=" + assembly.FullName); }
        catch (Exception failure) { Failure(output, failure); }
        try { output.WriteLine("MVID=" + assembly.ManifestModule.ModuleVersionId); }
        catch (Exception failure) { Failure(output, failure); }
        try { output.WriteLine("Location=" + assembly.Location); }
        catch (Exception failure) { Failure(output, failure); }
    }

    static void Inspect(StreamWriter output, string name, string typeName, bool probe)
    {
        output.WriteLine("[load " + name + "]");
        try
        {
            Assembly assembly = Assembly.Load(name);
            output.WriteLine("Assembly.Load OK"); Describe(output, assembly);
            try
            {
                Type[] types = assembly.GetTypes();
                output.WriteLine("GetTypes OK count=" + types.Length);
                foreach (Type item in types) output.WriteLine("Type=" + item.FullName);
            }
            catch (Exception failure) { Failure(output, failure); }
            Type type = assembly.GetType(typeName, true);
            output.WriteLine("GetType OK=" + type.FullName);
            MethodInfo[] methods = type.GetMethods(BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Static | BindingFlags.Instance | BindingFlags.DeclaredOnly);
            output.WriteLine("GetMethods OK count=" + methods.Length);
            foreach (MethodInfo method in methods) output.WriteLine("Method=" + method.ToString());
            if (probe)
            {
                output.WriteLine("HasCurrent(null) JIT probe begin");
                MethodInfo method = type.GetMethod("HasCurrent", BindingFlags.Public | BindingFlags.Static);
                if (method == null) throw new MissingMethodException(typeName, "HasCurrent");
                output.WriteLine("HasCurrent(null) OK result=" + method.Invoke(null, new object[] { null }));
            }
        }
        catch (Exception failure) { Failure(output, failure); }
    }

    static void Snapshot(StreamWriter output, string typeName)
    {
        output.WriteLine("[active UI snapshot " + typeName + "]");
        try
        {
            Type type = Assembly.Load("Assembly-CSharp").GetType(typeName, true);
            UnityEngine.Object[] instances = UnityEngine.Object.FindObjectsOfType(type);
            output.WriteLine("ActiveInstances=" + instances.Length);
            foreach (UnityEngine.Object instance in instances)
            {
                if (typeName == "UI.UIActivity" && !activityProbed)
                {
                    activityProbed = true;
                    DirectActivityGuards(output, instance);
                }
                Component component = instance as Component;
                output.WriteLine("GameObject=" + (component == null ? "none" : component.gameObject.name));
                if (component != null) output.WriteLine("activeSelf=" + component.gameObject.activeSelf + " activeInHierarchy=" + component.gameObject.activeInHierarchy);
                foreach (FieldInfo field in type.GetFields(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.DeclaredOnly))
                {
                    try
                    {
                        object value = field.GetValue(instance);
                        string description = value == null ? "null" : value.GetType().FullName;
                        // Only structural information. Never print arbitrary
                        // strings/IDs/objects or examine account/session fields.
                        Component child = value as Component;
                        ICollection collection = value as ICollection;
                        if (child != null) description += " GameObject=" + child.gameObject.name + " active=" + child.gameObject.activeInHierarchy;
                        else if (collection != null) description += " Count=" + collection.Count;
                        else if (value != null && field.FieldType.IsEnum) description += " Enum=" + value;
                        else if (field.Name == "showTimes" && value != null) description += " Value=" + value;
                        output.WriteLine("Field=" + field.Name + " Value=" + description);
                        if (field.Name == "activityIds" && value is IDictionary)
                            foreach (DictionaryEntry pair in (IDictionary)value) output.WriteLine("ActivityId=" + pair.Key + ":" + pair.Value);
                        if (field.Name == "alphas" && value is IList)
                            foreach (object item in (IList)value) DescribeUIObject(output, item, "Alpha");
                        if (field.Name == "panel" || field.Name == "moduleTrans" || field.Name == "curActivity")
                            DescribeUIObject(output, value, field.Name);
                    }
                    catch (Exception failure) { Failure(output, failure); }
                }
            }
        }
        catch (Exception failure) { Failure(output, failure); }
    }

    static void DescribeUIObject(StreamWriter output, object value, string label)
    {
        Component component = value as Component;
        GameObject go = value as GameObject;
        if (component != null) go = component.gameObject;
        if (go == null) { output.WriteLine(label + "=null/non-component"); return; }
        output.WriteLine(label + "=" + go.name + " active=" + go.activeInHierarchy + " childCount=" + go.transform.childCount);
        foreach (Transform child in go.transform)
        {
            output.WriteLine(label + ".child=" + child.gameObject.name + " active=" + child.gameObject.activeInHierarchy);
            foreach (Component item in child.GetComponents<Component>())
            {
                if (item == null) continue;
                FieldInfo alpha = item.GetType().GetField("mAlpha", BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Instance);
                if (alpha != null) output.WriteLine("ChildComponent=" + item.GetType().FullName + " mAlpha=" + alpha.GetValue(item));
            }
        }
    }

    static void DirectActivityGuards(StreamWriter output, object page)
    {
        output.WriteLine("[isolated guard JIT probes; synthetic empty response and independent nonempty tabs; no requests]");
        try
        {
            Type bridge = Assembly.Load("SoulFlowerActivity").GetType("HYJS.ActivitySafety.ActivityGuard", true);
            foreach (MethodInfo method in bridge.GetMethods(BindingFlags.Public | BindingFlags.Static))
                foreach (ParameterInfo parameter in method.GetParameters())
                    output.WriteLine("Parameter=" + method.Name + ":" + parameter.Name + ":" + parameter.ParameterType.AssemblyQualifiedName);
            MethodInfo prepare = bridge.GetMethod("PrepareResponse");
            Type packetType = prepare.GetParameters()[1].ParameterType;
            Type responseType = Assembly.Load("Assembly-CSharp-firstpass").GetType("ProtoBuf.Message.SCActivityList", false);
            if (responseType == null) responseType = Assembly.Load("Assembly-CSharp").GetType("ProtoBuf.Message.SCActivityList", true);
            object response = Activator.CreateInstance(responseType);
            PropertyInfo listProperty = responseType.GetProperty("activityList");
            FieldInfo listField = responseType.GetField("activityList");
            object independentEntries = listProperty != null ? listProperty.GetValue(response, null) : (listField == null ? null : listField.GetValue(response));
            if (!(independentEntries is IList) || ((IList)independentEntries).Count != 0)
                throw new InvalidOperationException("Skip response probe: synthetic response did not expose an independent empty activityList");
            object packet = Activator.CreateInstance(packetType, new object[] { 0, response });
            output.WriteLine("PrepareResponse synthetic-empty begin");
            try { output.WriteLine("PrepareResponse synthetic-empty result=" + prepare.Invoke(null, new object[] { page, packet, true })); }
            catch (Exception failure) { Failure(output, failure); }
            MethodInfo tabsMethod = bridge.GetMethod("PrepareTabs");
            Type listType = tabsMethod.GetParameters()[1].ParameterType;
            Type enumType = listType.GetGenericArguments()[0];
            IList tabs = (IList)Activator.CreateInstance(listType);
            tabs.Add(Enum.Parse(enumType, "EveryDaySign"));
            output.WriteLine("PrepareTabs independent EveryDaySign list begin");
            try { output.WriteLine("PrepareTabs independent-list result=" + tabsMethod.Invoke(null, new object[] { page, tabs })); }
            catch (Exception failure) { Failure(output, failure); }
            output.WriteLine("Original UIActivity.MsgCallback synthetic-empty direct invoke begin");
            try
            {
                MethodInfo original = page.GetType().GetMethod("MsgCallback", BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Instance);
                original.Invoke(page, new object[] { packet, true });
                output.WriteLine("Original MsgCallback returned OK");
            }
            catch (Exception failure) { Failure(output, failure); }
        }
        catch (Exception failure) { Failure(output, failure); }
    }

    static void InspectOriginalIL(StreamWriter output)
    {
        output.WriteLine("[actual loaded main IL]");
        foreach (string target in new string[] { "UI.QualityBar:Start", "UI.UIActivity:MsgCallback", "UI.UIActivity:Init", "UI.UIActivityPanel:Show" })
        {
            try
            {
                string[] names = target.Split(':');
                Type type = Assembly.Load("Assembly-CSharp").GetType(names[0], true);
                foreach (MethodInfo method in type.GetMethods(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.DeclaredOnly))
                {
                    if (method.Name != names[1]) continue;
                    MethodBody body = method.GetMethodBody();
                    output.WriteLine(target + " token=" + method.MetadataToken + " IL=" + (body == null ? "none" : BitConverter.ToString(body.GetILAsByteArray())));
                }
            }
            catch (Exception failure) { Failure(output, failure); }
        }
    }

    static void DisplayState(StreamWriter output, Component quality, Type options, string phase)
    {
        output.WriteLine("DisplayState=" + phase + " optionsComponents=" + quality.GetComponents(options).Length);
        Transform parent = quality.transform.parent;
        Transform row = parent == null ? null : parent.Find("SoulFlowerFrameRate");
        output.WriteLine("SoulFlowerFrameRate=" + (row == null ? "missing" : "present active=" + row.gameObject.activeInHierarchy));
    }

    static void DirectDisplayAttach(StreamWriter output)
    {
        output.WriteLine("[authorized one-time Display.Attach UI diagnostic; no FPS setters]");
        try
        {
            Assembly bridge = Assembly.Load("SoulFlowerDisplay");
            Type settings = bridge.GetType("HYJS.Display.DisplaySettings", true);
            Type options = bridge.GetType("HYJS.Display.DisplayFrameOptions", true);
            Type qualityType = Assembly.Load("Assembly-CSharp").GetType("UI.QualityBar", true);
            foreach (UnityEngine.Object value in UnityEngine.Object.FindObjectsOfType(qualityType))
            {
                Component quality = value as Component;
                if (quality == null) continue;
                DisplayState(output, quality, options, "before");
                try
                {
                    settings.GetMethod("Attach", BindingFlags.Public | BindingFlags.Static).Invoke(null, new object[] { quality });
                    output.WriteLine("Attach returned OK");
                }
                catch (Exception failure) { Failure(output, failure); }
                DisplayState(output, quality, options, "after");
            }
        }
        catch (Exception failure) { Failure(output, failure); }
    }

    internal static IEnumerator Watch(GameObject host)
    {
        float end = Time.realtimeSinceStartup + 20f;
        float next = 0f;
        while (Time.realtimeSinceStartup < end)
        {
            if (Time.realtimeSinceStartup >= next)
            {
                next = Time.realtimeSinceStartup + 1f;
                try
                {
                    using (StreamWriter output = new StreamWriter(Path.Combine(Application.persistentDataPath, "client-bridge-diagnostics.txt"), true, Encoding.UTF8))
                    {
                        output.AutoFlush = true;
                        output.WriteLine("[delayed UTC=" + DateTime.UtcNow.ToString("o") + "]");
                        Snapshot(output, "UI.UIActivity");
                        Snapshot(output, "UI.UIActivityPanel");
                    }
                }
                catch (Exception) { }
            }
            yield return null;
        }
        try { File.AppendAllText(Path.Combine(Application.persistentDataPath, "client-bridge-diagnostics.txt"), "[complete]\n"); }
        catch (Exception) { }
        UnityEngine.Object.Destroy(host);
    }
}

public sealed class ClientBridgeProbeWatch : MonoBehaviour
{
    void Start() { StartCoroutine(NativeBatchRunner.Watch(gameObject)); }
}
