using System;
using System.Collections.Generic;
using System.Globalization;
using System.Reflection;
using LitJson;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Reuses the five native name TextMeshes through UIName3D.text. The
    // account name and the original player data are never changed.
    public sealed class ChipNames : MonoBehaviour
    {
        sealed class NameState
        {
            public UIName3D label;
            public string original, rendered;
            public long count;
            public bool hasCount;
        }
        static readonly FieldInfo LabelField=typeof(RCUIPlayer).GetField("label",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
        static ChipNames instance;
        readonly Dictionary<long,NameState> names=new Dictionary<long,NameState>();
        readonly List<long> removals=new List<long>();
        Dictionary<long,long> counts=new Dictionary<long,long>();
        RoomBase room;
        FieldWire.Session owner;
        int sceneVersion, connectionEpoch;
        long line;

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
        void Restore(NameState state)
        {
            if (state==null || state.label==null) return;
            try { if (state.label.text==state.rendered) state.label.text=state.original; }
            catch (Exception) { }
        }
        void Clear()
        {
            foreach (NameState state in names.Values) Restore(state);
            names.Clear(); counts.Clear(); room=null; owner=null; line=0;
        }
        void OnDestroy() { Clear(); if (instance==this) instance=null; }

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
                foreach (long id in removals) { Restore(names[id]); names.Remove(id); }
                foreach (KeyValuePair<long,long> entry in counts)
                {
                    RoomCharactor actor=entry.Key==owner.playerId ? room.charactorManager.GetSelfCharactor() :
                        room.charactorManager.GetCharactor(entry.Key.ToString(CultureInfo.InvariantCulture));
                    RCUIPlayer player=actor==null ? null : actor.ui as RCUIPlayer;
                    UIName3D label=player==null || LabelField==null ? null : LabelField.GetValue(player) as UIName3D;
                    NameState state;
                    names.TryGetValue(entry.Key,out state);
                    if (label==null)
                    {
                        if (state!=null) { Restore(state); names.Remove(entry.Key); }
                        continue;
                    }
                    if (state==null || state.label!=label)
                    {
                        Restore(state);
                        state=new NameState(); state.label=label; names[entry.Key]=state;
                    }
                    string native=label.text;
                    if (String.IsNullOrEmpty(native)) continue;
                    if (native!=state.rendered) { state.original=native; state.hasCount=false; }
                    if (!state.hasCount || state.count!=entry.Value)
                    {
                        state.count=entry.Value; state.hasCount=true;
                        state.rendered=state.original+"  芯片 "+entry.Value.ToString(CultureInfo.InvariantCulture);
                    }
                    if (native!=state.rendered) label.text=state.rendered;
                }
            }
            catch (Exception) { Clear(); }
        }
    }
}
