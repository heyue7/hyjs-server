using System;
using System.Collections.Generic;
using System.Globalization;
using System.Reflection;
using LitJson;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Keep the original centred name intact. Independent TextMeshes inherit the
    // native five-layer font and follow each original layer's transform.
    public sealed class ChipNames : MonoBehaviour
    {
        sealed class NameState
        {
            public UIName3D label;
            public readonly TextMesh[] native=new TextMesh[5];
            public readonly TextMesh[] chip=new TextMesh[5];
            public readonly GameObject[] objects=new GameObject[5];
        }
        static readonly FieldInfo LabelField=typeof(RCUIPlayer).GetField("label",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
        static readonly FieldInfo[] TextFields=new FieldInfo[] {
            TextField("text0"), TextField("text1"), TextField("text2"), TextField("text3"), TextField("text4") };
        static ChipNames instance;
        readonly Dictionary<long,NameState> names=new Dictionary<long,NameState>();
        readonly List<long> removals=new List<long>();
        Dictionary<long,long> counts=new Dictionary<long,long>();
        RoomBase room;
        FieldWire.Session owner;
        int sceneVersion, connectionEpoch;
        long line;

        static FieldInfo TextField(string name)
        { return typeof(UIName3D).GetField(name,BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic); }
        static bool Same(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.sessionHex==b.sessionHex && a.origin==b.origin; }

        public static void ApplySnapshot(JsonData data)
        {
            try
            {
                FieldWire.Session current;
                RoomBase active=RoomManager.currentRoom;
                if (data==null || StaticData.fieldId!="field_003" || active==null || active.charactorManager==null ||
                    !FieldWire.Current(out current) || (string)data["scene"]!="field_003")
                { ReleaseScene(); return; }
                long nextLine=Int64.Parse(data["line"].ToString(),CultureInfo.InvariantCulture);
                if (nextLine<1) throw new InvalidOperationException("Invalid field line");
                Dictionary<long,long> next=new Dictionary<long,long>();
                foreach (JsonData entry in data["playerChipCounts"])
                {
                    long id=Int64.Parse(entry["playerId"].ToString(),CultureInfo.InvariantCulture);
                    long count=Int64.Parse(entry["count"].ToString(),CultureInfo.InvariantCulture);
                    if (id<1 || count<0 || next.ContainsKey(id) || next.Count>=1000)
                        throw new InvalidOperationException("Invalid chip count projection");
                    next.Add(id,count);
                }
                if (instance==null)
                {
                    GameObject root=new GameObject("HYJSDesertChipNames");
                    UnityEngine.Object.DontDestroyOnLoad(root);
                    instance=root.AddComponent<ChipNames>();
                }
                int version=FieldRuntime.SceneVersion, epoch=PositionSync.ConnectionEpoch;
                if (instance.room!=active || !Same(instance.owner,current) || instance.line!=nextLine ||
                    instance.sceneVersion!=version || instance.connectionEpoch!=epoch) instance.Clear();
                instance.room=active; instance.owner=current; instance.line=nextLine;
                instance.sceneVersion=version; instance.connectionEpoch=epoch; instance.counts=next;
            }
            catch (Exception) { ReleaseScene(); }
        }
        public static void ReleaseScene()
        {
            if (instance!=null) instance.Clear();
        }
        static void Hide(NameState state)
        {
            if (state==null) return;
            for (int i=0;i<state.objects.Length;i++)
                if (state.objects[i]!=null) state.objects[i].SetActive(false);
        }
        static void Release(NameState state)
        {
            if (state==null) return;
            for (int i=0;i<state.objects.Length;i++)
            {
                GameObject go=state.objects[i];
                if (go!=null) { go.SetActive(false); UnityEngine.Object.Destroy(go); }
                state.objects[i]=null; state.chip[i]=null;
            }
        }
        void Remove(long id)
        {
            NameState state;
            if (names.TryGetValue(id,out state)) { Release(state); names.Remove(id); }
        }
        void Clear()
        {
            foreach (NameState state in names.Values) Release(state);
            names.Clear(); counts.Clear(); room=null; owner=null; line=0;
        }
        void OnDestroy() { Clear(); if (instance==this) instance=null; }

        static bool Matches(NameState state,UIName3D label)
        {
            if (state==null || state.label!=label) return false;
            for (int i=0;i<TextFields.Length;i++)
                if (TextFields[i]==null || TextFields[i].GetValue(label) as TextMesh!=state.native[i] || state.native[i]==null)
                    return false;
            return true;
        }
        static NameState MakeState(UIName3D label)
        {
            NameState state=new NameState(); state.label=label;
            for (int i=0;i<TextFields.Length;i++)
            {
                if (TextFields[i]==null) return null;
                state.native[i]=TextFields[i].GetValue(label) as TextMesh;
                if (state.native[i]==null) return null;
            }
            return state;
        }

        // TextMesh owns its mesh. Recover its local rectangle from the world
        // renderer bounds and the two transformed text axes, as the existing
        // native head-icon implementation does. A degenerate view stays hidden.
        static bool TextBounds(TextMesh text,Renderer renderer,out Bounds bounds)
        {
            bounds=new Bounds(); Bounds world=renderer.bounds;
            Vector3 x=text.transform.TransformVector(Vector3.right), y=text.transform.TransformVector(Vector3.up);
            x=new Vector3(Mathf.Abs(x.x),Mathf.Abs(x.y),Mathf.Abs(x.z));
            y=new Vector3(Mathf.Abs(y.x),Mathf.Abs(y.y),Mathf.Abs(y.z));
            float xx=Vector3.Dot(x,x), xy=Vector3.Dot(x,y), yy=Vector3.Dot(y,y), det=xx*yy-xy*xy;
            if (det<=xx*yy*0.000001f) return false;
            float xs=Vector3.Dot(x,world.size), ys=Vector3.Dot(y,world.size);
            float width=(xs*yy-ys*xy)/det, height=(ys*xx-xs*xy)/det;
            if (width<=0.00001f || height<=0.00001f) return false;
            bounds=new Bounds(text.transform.InverseTransformPoint(world.center),new Vector3(width,height,0));
            return true;
        }
        static TextAnchor LeftAnchor(TextAnchor anchor)
        {
            switch (anchor)
            {
                case TextAnchor.UpperLeft: case TextAnchor.UpperCenter: case TextAnchor.UpperRight: return TextAnchor.UpperLeft;
                case TextAnchor.MiddleLeft: case TextAnchor.MiddleCenter: case TextAnchor.MiddleRight: return TextAnchor.MiddleLeft;
                default: return TextAnchor.LowerLeft;
            }
        }
        static void CreateLayers(NameState state)
        {
            for (int i=0;i<state.native.Length;i++)
            {
                GameObject go=new GameObject("HYJSDesertChipName"+i.ToString(CultureInfo.InvariantCulture));
                state.objects[i]=go; go.SetActive(false);
                go.transform.parent=state.native[i].transform;
                go.transform.localRotation=Quaternion.identity; go.transform.localScale=Vector3.one;
                state.chip[i]=go.AddComponent<TextMesh>();
                if (go.GetComponent<Renderer>()==null) go.AddComponent<MeshRenderer>();
            }
        }
        static bool LayersAlive(NameState state)
        {
            for (int i=0;i<state.native.Length;i++)
                if (state.objects[i]==null || state.chip[i]==null) return false;
            return true;
        }
        static void SyncLayer(NameState state,int i,string value,float x)
        {
            TextMesh source=state.native[i], target=state.chip[i];
            GameObject go=state.objects[i];
            Renderer original=source.GetComponent<Renderer>(), copy=go.GetComponent<Renderer>();
            if (original==null || copy==null) { go.SetActive(false); return; }
            go.layer=source.gameObject.layer;
            go.transform.localPosition=new Vector3(x,0,0);
            if (target.font!=source.font) target.font=source.font;
            if (target.fontSize!=source.fontSize) target.fontSize=source.fontSize;
            if (target.fontStyle!=source.fontStyle) target.fontStyle=source.fontStyle;
            if (target.characterSize!=source.characterSize) target.characterSize=source.characterSize;
            if (target.lineSpacing!=source.lineSpacing) target.lineSpacing=source.lineSpacing;
            if (target.tabSize!=source.tabSize) target.tabSize=source.tabSize;
            if (target.offsetZ!=source.offsetZ) target.offsetZ=source.offsetZ;
            if (target.richText!=source.richText) target.richText=source.richText;
            if (target.color!=source.color) target.color=source.color;
            TextAnchor anchor=LeftAnchor(source.anchor);
            if (target.anchor!=anchor) target.anchor=anchor;
            if (target.alignment!=TextAlignment.Left) target.alignment=TextAlignment.Left;
            if (target.text!=value) target.text=value;
            if (copy.sharedMaterial!=original.sharedMaterial) copy.sharedMaterial=original.sharedMaterial;
            if (copy.sortingLayerID!=original.sortingLayerID) copy.sortingLayerID=original.sortingLayerID;
            if (copy.sortingOrder!=original.sortingOrder) copy.sortingOrder=original.sortingOrder;
            copy.enabled=original.enabled;
            go.SetActive(true);
        }
        static void UpdateName(NameState state,long count)
        {
            TextMesh text=state.native[0]; Renderer renderer=text.GetComponent<Renderer>(); Bounds bounds;
            if (!state.label.gameObject.activeInHierarchy || !text.gameObject.activeInHierarchy || String.IsNullOrEmpty(text.text) ||
                renderer==null || !renderer.enabled || !TextBounds(text,renderer,out bounds))
            { Hide(state); return; }
            if (!LayersAlive(state)) { Release(state); CreateLayers(state); }
            string value="芯片 "+count.ToString(CultureInfo.InvariantCulture);
            float x=bounds.max.x+bounds.size.y*0.2f;
            for (int i=0;i<state.native.Length;i++) SyncLayer(state,i,value,x);
        }
        void LateUpdate()
        {
            try
            {
                FieldWire.Session current;
                if (room==null) return;
                if (StaticData.fieldId!="field_003" || room!=RoomManager.currentRoom || room.charactorManager==null ||
                    !FieldWire.Current(out current) || !Same(owner,current) || sceneVersion!=FieldRuntime.SceneVersion ||
                    connectionEpoch!=PositionSync.ConnectionEpoch || !NetAPI.SocketIsConnected())
                { Clear(); return; }
                removals.Clear();
                foreach (KeyValuePair<long,NameState> old in names)
                    if (!counts.ContainsKey(old.Key)) removals.Add(old.Key);
                foreach (long id in removals) Remove(id);
                foreach (KeyValuePair<long,long> entry in counts)
                {
                    try
                    {
                        RoomCharactor actor=entry.Key==owner.playerId ? room.charactorManager.GetSelfCharactor() :
                            room.charactorManager.GetCharactor(entry.Key.ToString(CultureInfo.InvariantCulture));
                        RCUIPlayer player=actor==null ? null : actor.ui as RCUIPlayer;
                        UIName3D label=player==null || LabelField==null ? null : LabelField.GetValue(player) as UIName3D;
                        NameState state;
                        names.TryGetValue(entry.Key,out state);
                        if (label==null) { Remove(entry.Key); continue; }
                        if (!Matches(state,label))
                        {
                            Remove(entry.Key); state=MakeState(label);
                            if (state==null) continue;
                            names[entry.Key]=state;
                        }
                        UpdateName(state,entry.Value);
                    }
                    catch (Exception) { Remove(entry.Key); }
                }
            }
            catch (Exception) { Clear(); }
        }
    }
}
