// A small managed model of the native objects touched by the recharge bridge.
// The tested bridge code is extracted verbatim from InstanceSelector.cs.
using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;

namespace UnityEngine
{
    public class Object
    {
        public static void Destroy(Object value)
        {
            GameObject go = value as GameObject;
            if (go != null) { go.SetActive(false); go.transform.parent = null; }
        }
    }
    public struct Vector3
    {
        public float x, y, z;
        public Vector3(float a, float b, float c) { x = a; y = b; z = c; }
        public static Vector3 one { get { return new Vector3(1, 1, 1); } }
    }
    public struct Color
    {
        public float r, g, b, a;
        public Color(float a1, float a2, float a3, float a4) { r = a1; g = a2; b = a3; a = a4; }
    }
    public class Component : Object
    {
        public GameObject gameObject;
        public Transform transform { get { return gameObject.transform; } }
        public T GetComponent<T>() where T : Component { return gameObject.GetComponent<T>(); }
        public T GetComponentInParent<T>() where T : Component
        {
            for (Transform t = transform; t != null; t = t.parent)
            {
                T result = t.gameObject.GetComponent<T>();
                if (result != null) return result;
            }
            return null;
        }
    }
    public class MonoBehaviour : Component { }
    public class GameObject : Object
    {
        readonly List<Component> components = new List<Component>();
        public readonly Transform transform;
        public string name;
        public int layer;
        public bool activeSelf = true;
        public GameObject(string n)
        {
            name = n;
            transform = new Transform();
            transform.gameObject = this;
            components.Add(transform);
        }
        public void SetActive(bool active) { activeSelf = active; }
        public T AddComponent<T>() where T : Component, new()
        {
            T value = new T(); value.gameObject = this; components.Add(value); return value;
        }
        public T GetComponent<T>() where T : Component
        {
            foreach (Component component in components)
                if (component is T) return (T)component;
            return null;
        }
    }
    public class Transform : Component, IEnumerable<Transform>
    {
        readonly List<Transform> children = new List<Transform>();
        Transform owner;
        public Vector3 localPosition, localScale = Vector3.one;
        public Transform parent
        {
            get { return owner; }
            set { if (owner != null) owner.children.Remove(this); owner = value; if (owner != null) owner.children.Add(this); }
        }
        public Transform Find(string name)
        {
            foreach (Transform child in children) if (child.gameObject.name == name) return child;
            return null;
        }
        public IEnumerator<Transform> GetEnumerator() { return children.GetEnumerator(); }
        IEnumerator IEnumerable.GetEnumerator() { return GetEnumerator(); }
    }
}

public class UIWidget : UnityEngine.MonoBehaviour
{
    public enum Pivot { Center, TopLeft }
    Pivot currentPivot = Pivot.Center;
    public Pivot pivot
    {
        get { return currentPivot; }
        set
        {
            if (currentPivot == Pivot.Center && value == Pivot.TopLeft)
            {
                UnityEngine.Vector3 position = transform.localPosition;
                transform.localPosition = new UnityEngine.Vector3(position.x - width / 2f, position.y + height / 2f, position.z);
            }
            currentPivot = value;
        }
    }
    public int width, height, depth;
    public UnityEngine.Color color;
}
public static class NGUIText { public enum Alignment { Left } }
public class UILabel : UIWidget
{
    public enum Overflow { ShrinkContent }
    public object bitmapFont, trueTypeFont;
    public int fontStyle, fontSize, maxLineCount;
    public NGUIText.Alignment alignment;
    public bool supportEncoding = true;
    public Overflow overflowMethod;
    public string text;
}
public class UISprite : UIWidget { }
public class UIScrollView : UnityEngine.MonoBehaviour
{
    public bool enabled;
    public int invalidated, resets;
    public void InvalidateBounds() { invalidated++; }
    public void ResetPosition() { resets++; }
}
public class SimpleGrid : UnityEngine.MonoBehaviour
{
    public enum Arrangement { Horizontal, Vertical }
    public Arrangement arrangement = Arrangement.Vertical;
    public int maxPerLine = 1;
    public float cellHeight = 148f;
    public int repositions;
    public readonly List<UnityEngine.GameObject> childs = new List<UnityEngine.GameObject>();
    public void AddChild(UnityEngine.GameObject child)
    {
        childs.Add(child); child.transform.parent = transform;
        child.transform.localPosition = new UnityEngine.Vector3(0, -(childs.Count - 1) * cellHeight, 0);
    }
    public void Reposition()
    {
        repositions++;
        for (int i = 0; i < childs.Count; i++)
            childs[i].transform.localPosition = new UnityEngine.Vector3(0, -i * cellHeight, 0);
    }
}
public static class TableTextBlo
{
    public static string GetText(string key) { return "累计充值{0}元"; }
}
namespace ProtoBuf.Message
{
    public class ActivityItem { public string value, param, message; public int left, total, status, index; }
}
namespace UI
{
    public enum ActivityType { Other = 1, RechargeTotal = 16 }
    public class AcRechargeSingleItem : UnityEngine.MonoBehaviour
    {
        string activityId;
        public ActivityType type;
        public UILabel labTitle, labLeftTimes, leftLabel;
        public void SetActivityID(string id) { activityId = id; }
    }
}

public static class RechargeDirectoryHarness
{
    const string Directory = "rechargeTotal_p4_directory_8e1df9c1";
    const string Marker = "recharge-directory-v1";
    const string End = "2026-11-04 00:00:00";
    const string Claim = "2026-11-05 23:59:59";
    static int checks;
    static void Check(bool ok, string message) { checks++; if (!ok) throw new Exception(message); }
    static UnityEngine.GameObject Child(UnityEngine.Transform parent, string name, float x, float y)
    {
        UnityEngine.GameObject go = new UnityEngine.GameObject(name);
        go.transform.parent = parent; go.transform.localPosition = new UnityEngine.Vector3(x, y, 0); return go;
    }
    static UI.AcRechargeSingleItem Row(SimpleGrid grid, string id, UI.ActivityType type)
    {
        UnityEngine.GameObject go = new UnityEngine.GameObject("ActivityItem"); go.layer = 7;
        UI.AcRechargeSingleItem row = go.AddComponent<UI.AcRechargeSingleItem>(); row.type = type; row.SetActivityID(id);
        UnityEngine.GameObject title = Child(go.transform, "titleBg", 0, 53);
        row.labTitle = Child(title.transform, "Label", -187, 2).AddComponent<UILabel>();
        row.labTitle.trueTypeFont = "Words"; row.labTitle.fontStyle = 4; row.labTitle.depth = 19;
        row.leftLabel = Child(title.transform, "lefttimes", 154, 2).AddComponent<UILabel>();
        row.leftLabel.color = new UnityEngine.Color(.318f, .318f, .318f, 1); row.leftLabel.depth = 20;
        row.labLeftTimes = Child(title.transform, "lefttimesNun", 199, 2).AddComponent<UILabel>();
        UISprite back = Child(go.transform, "bg", 0, -2).AddComponent<UISprite>(); back.height = 144;
        Child(go.transform, "Scroll View", -64, -15);
        Child(go.transform, "btnGet", 191, -20);
        Child(go.transform, "getted", 191, -20);
        Child(go.transform, "notReached", 191, -20);
        grid.AddChild(go); return row;
    }
    static ProtoBuf.Message.ActivityItem Item(int status, string title)
    {
        return new ProtoBuf.Message.ActivityItem { value = "6000", left = 3000, total = 6000, status = status, index = 80 + status,
            param = Marker, message = title + " · 活动截止 " + End + " · 领奖截止 " + Claim };
    }
    static void Late(SimpleGrid grid)
    {
        object state = grid.GetComponent<HYJS.ActivitySafety.RechargeDirectoryGrid>();
        Check(state != null, "directory grid component missing");
        state.GetType().GetMethod("LateUpdate", BindingFlags.NonPublic | BindingFlags.Instance).Invoke(state, null);
    }
    public static void Main()
    {
        UIWidget pivotProbe = new UnityEngine.GameObject("pivot probe").AddComponent<UIWidget>();
        pivotProbe.width = 472; pivotProbe.height = 80;
        pivotProbe.transform.localPosition = new UnityEngine.Vector3(100, 200, 3);
        Check(pivotProbe.pivot == UIWidget.Pivot.Center, "native pivot default changed");
        pivotProbe.pivot = UIWidget.Pivot.TopLeft;
        Check(pivotProbe.transform.localPosition.x == -136 && pivotProbe.transform.localPosition.y == 240 && pivotProbe.transform.localPosition.z == 3, "pivot did not move the widget");

        UnityEngine.GameObject view = new UnityEngine.GameObject("Scroll View");
        UIScrollView scroll = view.AddComponent<UIScrollView>();
        SimpleGrid grid = Child(view.transform, "simpleGrid", 0, 0).AddComponent<SimpleGrid>();
        UI.AcRechargeSingleItem first = Row(grid, Directory, UI.ActivityType.RechargeTotal);
        ProtoBuf.Message.ActivityItem a = Item(0, "测试充值");
        first.transform.Find("btnGet").gameObject.SetActive(false);
        first.transform.Find("getted").gameObject.SetActive(false);
        HYJS.ActivitySafety.RechargeDisplay.Apply(first, a);
        Check(first.labTitle.text == "累计充值60元" && first.labLeftTimes.text == "30/60", "original amount display changed");
        Check(a.status == 0 && a.index == 80, "bridge changed the reward state or index");
        Check(!first.transform.Find("btnGet").gameObject.activeSelf && !first.transform.Find("getted").gameObject.activeSelf && first.transform.Find("notReached").gameObject.activeSelf, "unreached status changed");
        Check(grid.cellHeight == 244, "expanded native row spacing is wrong");
        Check(first.transform.Find("titleBg").localPosition.y == 53, "native title moved");
        Check(first.transform.Find("bg").GetComponent<UISprite>().height == 240 && first.transform.Find("bg").localPosition.y == -50, "background expansion is wrong");
        Check(first.transform.Find("Scroll View").localPosition.y == -111 && first.transform.Find("btnGet").localPosition.y == -116, "reward or button position is wrong");
        UILabel caption = first.transform.Find("HYJSRechargeDirectoryCaption").GetComponent<UILabel>();
        Check(caption.text == "测试充值\n活动截止 " + End + "\n领奖截止 " + Claim, "activity title or deadline changed");
        Check(caption.trueTypeFont == first.labTitle.trueTypeFont && caption.fontStyle == 4 && caption.fontSize == 16, "original font was not reused");
        Check(caption.width == 472 && caption.height == 80 && caption.maxLineCount == 6 && !caption.supportEncoding && caption.overflowMethod == UILabel.Overflow.ShrinkContent, "caption is not bounded");
        Check(caption.pivot == UIWidget.Pivot.TopLeft && caption.transform.localPosition.x == -236 && caption.transform.localPosition.y == 27 && caption.transform.localPosition.z == 0 && caption.gameObject.layer == 7, "caption placement changed");
        HYJS.ActivitySafety.RechargeDisplay.Apply(first, a);
        Check(first.transform.Find("btnGet").localPosition.y == -116 && first.transform.Find("bg").GetComponent<UISprite>().height == 240, "same row was expanded twice");

        UI.AcRechargeSingleItem second = Row(grid, Directory, UI.ActivityType.RechargeTotal);
        ProtoBuf.Message.ActivityItem b = Item(1, new string('测', 80));
        second.transform.Find("getted").gameObject.SetActive(false);
        second.transform.Find("notReached").gameObject.SetActive(false);
        HYJS.ActivitySafety.RechargeDisplay.Apply(second, b);
        Check(b.status == 1 && b.index == 81 && second.transform.localPosition.y == -244, "second row identity or placement changed");
        Check(second.transform.Find("btnGet").gameObject.activeSelf && !second.transform.Find("getted").gameObject.activeSelf && !second.transform.Find("notReached").gameObject.activeSelf, "claimable status changed");
        Check(second.transform.Find("HYJSRechargeDirectoryCaption").GetComponent<UILabel>().text == new string('测', 80) + "\n活动截止 " + End + "\n领奖截止 " + Claim, "long title or deadline was lost");
        // Model the original UpdateActivityItems disabling scroll after adding two rows.
        scroll.enabled = false; second.transform.localPosition = new UnityEngine.Vector3(0, -148, 0);
        Late(grid);
        Check(scroll.enabled && scroll.invalidated == 1 && scroll.resets == 1, "two rows cannot scroll after native callback");
        Check(second.transform.localPosition.y == -244, "final grid pass did not reposition the last row");
        Check(27 - 80 > 40 - 96 && -74 - 96 > 73 - 244, "caption, reward area or adjacent row overlap");
        Late(grid); Check(scroll.resets == 1, "idle frames repeatedly reset the scroll");

        UI.AcRechargeSingleItem third = Row(grid, Directory, UI.ActivityType.RechargeTotal);
        ProtoBuf.Message.ActivityItem c = Item(2, "已领取"); c.value = "0.5";
        third.transform.Find("btnGet").gameObject.SetActive(false);
        third.transform.Find("notReached").gameObject.SetActive(false);
        HYJS.ActivitySafety.RechargeDisplay.Apply(third, c);
        Check(c.status == 2 && third.transform.localPosition.y == -488 && third.transform.Find("HYJSRechargeDirectoryCaption") != null, "claimed row or noninteger amount was lost");
        Check(!third.transform.Find("btnGet").gameObject.activeSelf && third.transform.Find("getted").gameObject.activeSelf && !third.transform.Find("notReached").gameObject.activeSelf, "claimed status changed");
        Check(third.labTitle.text == null, "noninteger original title handling changed");
        Late(grid); Check(third.transform.localPosition.y == -488 && scroll.resets == 2, "last of three rows was not revalidated");

        string unusual = "标题 · 活动截止 假文字 · 活动截止 " + End + " · 领奖截止 " + Claim;
        Check(HYJS.ActivitySafety.RechargeDirectoryDisplay.Caption(unusual) == "标题 · 活动截止 假文字\n活动截止 " + End + "\n领奖截止 " + Claim, "a title containing the marker was changed");
        Check(HYJS.ActivitySafety.RechargeDirectoryDisplay.Caption("单独标题") == "单独标题", "unexpected message was rewritten");
        Check(HYJS.ActivitySafety.RechargeDirectoryDisplay.Caption("甲\r\n乙\t丙 · 活动截止 " + End + " · 领奖截止 " + Claim) == "甲  乙 丙\n活动截止 " + End + "\n领奖截止 " + Claim, "title whitespace can create unbounded lines");
        HYJS.ActivitySafety.RechargeDisplay.Apply(null, a);

        // A reusable row can be rebound; its owned geometry must be restored.
        grid.childs.Clear();
        grid.childs.Add(first.gameObject);
        first.SetActivityID("rechargeTotal_p4_other");
        HYJS.ActivitySafety.RechargeDisplay.Apply(first, Item(1, "重用"));
        Check(first.transform.Find("HYJSRechargeDirectoryCaption") == null && first.transform.Find("btnGet").localPosition.y == -20 && first.transform.Find("bg").GetComponent<UISprite>().height == 144, "reused row was not restored");
        Check(grid.cellHeight == 148, "reused row did not restore native spacing");

        // Native UpdateActivityItems clears the previous rows before creating a new list.
        grid.childs.Clear();
        UI.AcRechargeSingleItem other = Row(grid, "rechargeTotal_p4_other", UI.ActivityType.RechargeTotal);
        ProtoBuf.Message.ActivityItem unrelated = Item(2, "其他活动");
        HYJS.ActivitySafety.RechargeDisplay.Apply(other, unrelated);
        Check(grid.cellHeight == 148 && other.transform.Find("HYJSRechargeDirectoryCaption") == null, "ordinary activity did not restore the original layout");
        Check(other.transform.Find("bg").GetComponent<UISprite>().height == 144 && other.transform.Find("btnGet").localPosition.y == -20, "ordinary activity row was changed");
        Check(other.labTitle.text == "累计充值60元" && other.labLeftTimes.text == "30/60", "ordinary recharge conversion changed");
        scroll.enabled = false; Late(grid); Check(!scroll.enabled && scroll.resets == 2, "ordinary activity scrolling was overridden");

        grid.childs.Clear();
        UI.AcRechargeSingleItem wrongType = Row(grid, Directory, UI.ActivityType.Other);
        HYJS.ActivitySafety.RechargeDisplay.Apply(wrongType, Item(1, "非充值"));
        Check(wrongType.transform.Find("HYJSRechargeDirectoryCaption") == null && grid.cellHeight == 148 && wrongType.labTitle.text == null, "non-recharge page changed");
        UI.AcRechargeSingleItem noMarker = Row(grid, Directory, UI.ActivityType.RechargeTotal);
        ProtoBuf.Message.ActivityItem unmarked = Item(0, "未标记"); unmarked.param = "other";
        HYJS.ActivitySafety.RechargeDisplay.Apply(noMarker, unmarked);
        Check(noMarker.transform.Find("HYJSRechargeDirectoryCaption") == null && grid.cellHeight == 148, "unmarked row changed");
        Console.WriteLine("PASS recharge directory managed model: " + checks + " assertions");
    }
}
