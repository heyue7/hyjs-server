using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using NetWork.Layer;
using NetWork.Packets;
using ProtoBuf.Message;
using Room;
using UI;
using UnityEngine;

namespace HYJS.CopyBoxSync
{
    // This bridge never awards resources. The chest response supplies only the
    // identity of records to reconcile with a later authoritative list.
    public sealed class Runtime : MonoBehaviour
    {
        private sealed class Group
        {
            internal readonly object Room;
            internal readonly Dictionary<string, string> Heroes = new Dictionary<string, string>();
            internal readonly Dictionary<string, string> Equipment = new Dictionary<string, string>();
            internal readonly Dictionary<string, string> Fragments = new Dictionary<string, string>();
            internal readonly Dictionary<string, string> Items = new Dictionary<string, string>();
            internal bool Exited;
            internal Group(object room) { Room = room; }
            // Stack identities are captured for diagnosis only. They must not keep the
            // worker alive or drive updates without a server inventory revision.
            internal bool Empty { get { return Heroes.Count + Equipment.Count == 0; } }
            internal Dictionary<string, string> Entries(int kind)
            {
                if (kind == 1) return Heroes;
                if (kind == 2) return Equipment;
                if (kind == 3) return Fragments;
                return Items;
            }
        }

        private sealed class State
        {
            internal UIPlayerData Player;
            internal string Account;
            internal long PlayerId;
            internal byte[] Session;
            internal readonly List<Group> Groups = new List<Group>();
            internal Group Current;
            internal ulong Version;
            internal object Epoch = new object();
            internal object Attempt;
            internal bool Invalid;
            internal bool Running;
            internal float NextTry;
            internal int Failures;
        }

        private sealed class Fingerprint
        {
            internal object Value;
        }

        private sealed class Batch
        {
            internal int Kind;
            internal ulong Version;
            internal object Epoch;
            internal readonly List<Group> Groups = new List<Group>();
            internal readonly Dictionary<string, string> Entries = new Dictionary<string, string>();
            internal readonly Dictionary<string, Fingerprint> Before = new Dictionary<string, Fingerprint>();
        }

        private static volatile State active;
        private static Runtime host;
        [ThreadStatic] private static object ownRequest;
        [ThreadStatic] private static int ownOpcode;
        private static readonly BindingFlags Flags = BindingFlags.Static | BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;

        private static bool SameBytes(byte[] a, byte[] b)
        {
            if (a == null || b == null || a.Length != b.Length) return false;
            for (int i = 0; i < a.Length; i++) if (a[i] != b[i]) return false;
            return true;
        }

        private static bool Owned(State state)
        {
            return state != null && !state.Invalid && System.Object.ReferenceEquals(active, state) && state.Player != null &&
                System.Object.ReferenceEquals(state.Player, StaticData.playerData) && state.Player.playerId == state.PlayerId &&
                String.Equals(state.Account, StaticData.UserName, StringComparison.Ordinal) && SameBytes(state.Session, StaticData.sessionKey);
        }

        private static State Bind()
        {
            UIPlayerData player = StaticData.playerData;
            byte[] session = StaticData.sessionKey;
            string account = StaticData.UserName;
            if (player == null || player.playerId <= 0 || String.IsNullOrEmpty(account) || session == null || session.Length != 16) return null;
            State previous = active;
            if (Owned(previous)) return previous;
            State state = new State { Player = player, Account = account, PlayerId = player.playerId, Session = (byte[])session.Clone() };
            active = state;
            return state;
        }

        // Called at the beginning of the existing NetAPI.SendHttp. An unrelated
        // request invalidates any outstanding list without changing that send.
        // The identity guard also handles recursive sends while our send is on
        // the stack; only our exact request object is exempt.
        public static void ObserveSend(int opcode, object request)
        {
            try
            {
                State state = active;
                if (state == null || (ownRequest != null && opcode == ownOpcode && System.Object.ReferenceEquals(request, ownRequest))) return;
                Changed(state);
            }
            catch (Exception) { }
        }

        private static void Changed(State state)
        {
            lock (state)
            {
                if (state.Version != UInt64.MaxValue) { state.Version++; return; }
                // A distinct epoch makes wraparound unable to validate any old
                // batch, even if the counter later returns to the same value.
                try { state.Epoch = new object(); state.Version = 0; }
                catch (Exception) { state.Invalid = true; }
            }
        }

        private static void CaptureVersion(State state, Batch batch)
        { lock (state) { batch.Version = state.Version; batch.Epoch = state.Epoch; } }

        private static bool CurrentVersion(State state, Batch batch)
        { lock (state) { return !state.Invalid && state.Version == batch.Version && System.Object.ReferenceEquals(state.Epoch, batch.Epoch); } }

        private static Group OpenGroup(State state, object room)
        {
            Group group = state.Current;
            if (group == null || group.Exited || !System.Object.ReferenceEquals(group.Room, room))
            {
                group = new Group(room);
                state.Groups.Add(group);
                state.Current = group;
            }
            return group;
        }

        public static HttpHandler WrapOpen(HttpHandler original)
        {
            State state = null;
            Group group = null;
            try
            {
                state = Bind();
                object room = RoomManager.currentRoom;
                if (state != null && room is RoomCopy) group = OpenGroup(state, room);
            }
            catch (Exception) { }
            if (state == null || group == null || original == null) return original;
            return delegate(Packet packet, bool success)
            {
                try
                {
                    SCCopyBox reply = packet == null ? null : packet.kBody as SCCopyBox;
                    if (success && reply != null && packet.nOpCode == 54020 && Owned(state)) Record(state, group, reply);
                }
                catch (Exception) { Debug.LogWarning("Copy chest inventory capture unavailable"); }
                original(packet, success);
            };
        }

        public static HttpHandler WrapLeave(HttpHandler original, object room)
        {
            State state = active;
            Group group = null;
            try
            {
                if (Owned(state) && state.Current != null && !state.Current.Exited &&
                    System.Object.ReferenceEquals(state.Current.Room, room)) group = state.Current;
            }
            catch (Exception) { }
            if (state == null || group == null || original == null) return original;
            return delegate(Packet packet, bool success)
            {
                try
                {
                    if (success && packet != null && packet.nOpCode == 54014 && packet.kBody is SCCopyLeave && Owned(state))
                    {
                        group.Exited = true;
                        if (System.Object.ReferenceEquals(state.Current, group)) state.Current = null;
                        Changed(state);
                        if (group.Empty) state.Groups.Remove(group);
                        else Start(state);
                    }
                }
                catch (Exception) { Debug.LogWarning("Copy chest exit synchronization unavailable"); }
                original(packet, success);
            };
        }

        private static bool Starts(string value, string prefix) { return value.StartsWith(prefix, StringComparison.Ordinal); }
        private static void Put(Dictionary<string, string> entries, string key, string id)
        {
            string previous;
            if (entries.TryGetValue(key, out previous) && previous != id) throw new InvalidOperationException("Chest identity conflict");
            entries[key] = id;
        }

        private static void Record(State state, Group group, SCCopyBox reply)
        {
            if (reply.currentResource == null) return;
            // Validate the whole list before adding identities to the pending set.
            List<KeyValuePair<int, BaseResource>> accepted = new List<KeyValuePair<int, BaseResource>>();
            Dictionary<string, string> unique = new Dictionary<string, string>();
            foreach (BaseResource resource in reply.currentResource)
            {
                if (resource == null || String.IsNullOrEmpty(resource.id) || resource.num <= 0) throw new InvalidOperationException("Invalid chest resource");
                string id = resource.id;
                if (id == "coin" || id == "diamond") continue;
                int kind = Starts(id, "hero_") ? 1 : Starts(id, "equip_") ? 2 :
                    (Starts(id, "frag_hero_") || Starts(id, "frag_equip_") || Starts(id, "frag_pet_") || Starts(id, "frag_trea_")) ? 3 :
                    Starts(id, "item_") ? 4 : 0;
                if (kind == 0) continue;
                string key = kind <= 2 ? resource.guid : id;
                if (String.IsNullOrEmpty(key) || (kind <= 2 && resource.num != 1) || (kind > 2 && !String.IsNullOrEmpty(resource.guid)))
                    throw new InvalidOperationException("Invalid chest identity");
                string tagged = kind.ToString() + ":" + key;
                string old;
                if (unique.TryGetValue(tagged, out old) && old != id) throw new InvalidOperationException("Duplicate chest identity");
                if (group.Entries(kind).TryGetValue(key, out old) && old != id) throw new InvalidOperationException("Chest identity conflict");
                unique[tagged] = id;
                accepted.Add(new KeyValuePair<int, BaseResource>(kind, resource));
            }
            foreach (KeyValuePair<int, BaseResource> entry in accepted)
            {
                BaseResource resource = entry.Value;
                Put(group.Entries(entry.Key), entry.Key <= 2 ? resource.guid : resource.id, resource.id);
            }
            if (accepted.Count == 0) return;
            if (!state.Groups.Contains(group)) state.Groups.Add(group);
            Changed(state);
            if (group.Exited) Start(state);
        }

        private static void Start(State state)
        {
            if (!Owned(state) || state.Running) return;
            if (host == null)
            {
                GameObject obj = new GameObject("HYJS.CopyBoxSync");
                DontDestroyOnLoad(obj);
                host = obj.AddComponent<Runtime>();
            }
            state.Failures = 0;
            state.NextTry = 0;
            state.Running = true;
            try { host.StartCoroutine(host.Drain(state)); }
            catch { state.Running = false; throw; }
        }

        private static bool QueuesEmpty()
        {
            // HTTPManager clears sending after queuing a response and applies it
            // later in Tick. Wait until those callbacks and extra frames drain.
            FieldInfo field = typeof(NetAPI).GetField("http", Flags);
            object http = field == null ? null : field.GetValue(null);
            if (http == null || NetAPI.IsSendingHttp()) return false;
            foreach (string name in new string[] { "sendDatas", "handlers", "receivedDatas", "results", "externalDatas" })
            {
                FieldInfo queueField = http.GetType().GetField(name, Flags);
                ICollection queue = queueField == null ? null : queueField.GetValue(http) as ICollection;
                if (queue == null) return false;
                lock (queue) { if (queue.Count != 0) return false; }
            }
            return !NetAPI.IsSendingHttp();
        }

        private static bool NetworkUIIdle()
        {
            // CreateNetMask calls ClearNetMask first, which also destroys the
            // native timeout/error box. Never replace a pre-existing dialog.
            FieldInfo maskField = typeof(BoxManager).GetField("netMask", Flags);
            FieldInfo boxField = typeof(BoxManager).GetField("netBox", Flags);
            if (maskField == null || boxField == null) return false;
            GameObject mask = maskField.GetValue(null) as GameObject;
            GameObject box = boxField.GetValue(null) as GameObject;
            return box == null && (mask == null || !mask.activeSelf);
        }

        private static void ClearFailedOwnMask(State state, Batch batch)
        {
            // Send creates a wait mask even when serialization or transport
            // rejects the request. Clear only the mask created by our failed
            // synchronous call, after proving no other HTTP/dialog intervened.
            if (!Owned(state) || !CurrentVersion(state, batch) || !QueuesEmpty()) return;
            FieldInfo boxField = typeof(BoxManager).GetField("netBox", Flags);
            if (boxField == null || (boxField.GetValue(null) as GameObject) != null) return;
            BoxManager.ClearNetMask();
        }

        private static Fingerprint Read(UIPlayerData player, int kind, string key)
        {
            Fingerprint value = new Fingerprint();
            if (kind == 1) value.Value = player.GetUIHero(key);
            else if (kind == 2) value.Value = player.GetUIEquipment(key);
            else throw new InvalidOperationException("Unversioned stack synchronization disabled");
            return value;
        }

        private static bool Same(Fingerprint a, Fingerprint b)
        { return System.Object.ReferenceEquals(a.Value, b.Value); }

        private static Batch Next(State state)
        {
            for (int kind = 1; kind <= 2; kind++)
            {
                Batch batch = new Batch { Kind = kind };
                CaptureVersion(state, batch);
                foreach (Group group in state.Groups)
                {
                    if (!group.Exited || group.Entries(kind).Count == 0) continue;
                    batch.Groups.Add(group);
                    foreach (KeyValuePair<string, string> entry in group.Entries(kind)) Put(batch.Entries, entry.Key, entry.Value);
                }
                if (batch.Entries.Count == 0) continue;
                foreach (string key in batch.Entries.Keys) batch.Before.Add(key, Read(state.Player, kind, key));
                return batch;
            }
            return null;
        }

        private static bool CurrentBatch(State state, Batch batch)
        {
            if (!Owned(state) || !CurrentVersion(state, batch)) return false;
            foreach (KeyValuePair<string, Fingerprint> entry in batch.Before)
                if (!Same(entry.Value, Read(state.Player, batch.Kind, entry.Key))) return false;
            return true;
        }

        private static void FinishEntry(State state, Batch batch, string key)
        {
            foreach (Group group in batch.Groups)
            {
                string id;
                if (group.Entries(batch.Kind).TryGetValue(key, out id) && id == batch.Entries[key]) group.Entries(batch.Kind).Remove(key);
            }
            batch.Before[key] = Read(state.Player, batch.Kind, key);
        }

        private static void Apply(State state, Batch batch, Packet packet)
        {
            if (packet == null || !CurrentBatch(state, batch)) throw new InvalidOperationException("Stale inventory snapshot");
            if (batch.Kind == 1)
            {
                SCWarriorList reply = packet.kBody as SCWarriorList;
                if (packet.nOpCode != 20106 || reply == null || reply.warriorList == null) throw new InvalidOperationException("Warrior list missing");
                Dictionary<string, Warrior> found = new Dictionary<string, Warrior>();
                foreach (Warrior item in reply.warriorList)
                {
                    if (item == null || String.IsNullOrEmpty(item.guid) || !batch.Entries.ContainsKey(item.guid)) continue;
                    if (found.ContainsKey(item.guid) || item.id != batch.Entries[item.guid]) throw new InvalidOperationException("Warrior identity differs");
                    found.Add(item.guid, item);
                }
                foreach (string key in new List<string>(batch.Entries.Keys))
                {
                    if (!CurrentBatch(state, batch)) throw new InvalidOperationException("Inventory changed");
                    UIHero existing = batch.Before[key].Value as UIHero;
                    if (existing != null && existing.heroId != batch.Entries[key]) throw new InvalidOperationException("Local warrior identity differs");
                    Warrior item;
                    if (batch.Before[key].Value == null && found.TryGetValue(key, out item)) state.Player.InstPlayerData.AddHero(item);
                    if (!Owned(state) || !CurrentVersion(state, batch)) throw new InvalidOperationException("Inventory changed during apply");
                    FinishEntry(state, batch, key);
                }
                return;
            }
            if (batch.Kind == 2)
            {
                SCEquipmentTreasureList reply = packet.kBody as SCEquipmentTreasureList;
                if (packet.nOpCode != 20104 || reply == null || reply.equipmentList == null) throw new InvalidOperationException("Equipment list missing");
                Dictionary<string, Equipment> found = new Dictionary<string, Equipment>();
                foreach (Equipment item in reply.equipmentList)
                {
                    if (item == null || String.IsNullOrEmpty(item.guid) || !batch.Entries.ContainsKey(item.guid)) continue;
                    if (found.ContainsKey(item.guid) || item.equipId != batch.Entries[item.guid]) throw new InvalidOperationException("Equipment identity differs");
                    found.Add(item.guid, item);
                }
                foreach (string key in new List<string>(batch.Entries.Keys))
                {
                    if (!CurrentBatch(state, batch)) throw new InvalidOperationException("Inventory changed");
                    UIEquipment existing = batch.Before[key].Value as UIEquipment;
                    if (existing != null && existing.equipId != batch.Entries[key]) throw new InvalidOperationException("Local equipment identity differs");
                    Equipment item;
                    if (batch.Before[key].Value == null && found.TryGetValue(key, out item)) state.Player.InstPlayerData.AddEquipment(item);
                    if (!Owned(state) || !CurrentVersion(state, batch)) throw new InvalidOperationException("Inventory changed during apply");
                    FinishEntry(state, batch, key);
                }
                return;
            }

            throw new InvalidOperationException("Unversioned stack synchronization disabled");
        }

        private static object NewRequest(int kind, out int opcode)
        {
            if (kind == 1) { opcode = 20105; return new CSWarriorList(); }
            if (kind == 2) { opcode = 20103; return new CSEquipmentTreasureList(); }
            throw new InvalidOperationException("Unversioned stack synchronization disabled");
        }

        private static bool Send(int opcode, object request, HttpHandler callback)
        {
            object old = ownRequest;
            int oldOpcode = ownOpcode;
            try { ownRequest = request; ownOpcode = opcode; return NetAPI.SendHttp(opcode, request, callback); }
            finally { ownRequest = old; ownOpcode = oldOpcode; }
        }

        private const int MaximumFailures = 3;
        private static void Retry(State state)
        {
            state.Failures++;
            state.NextTry = Time.realtimeSinceStartup + Math.Min(30f, state.Failures * 2f);
        }

        private IEnumerator Drain(State state)
        {
            // The original leave callback and its queued external frames run first.
            yield return null;
            try
            {
                while (Owned(state))
                {
                    if (state.Failures >= MaximumFailures)
                    {
                        Debug.LogWarning("Copy chest instance synchronization stopped after three failed attempts");
                        yield break;
                    }
                    state.Groups.RemoveAll(delegate(Group group) { return group.Exited && group.Empty; });
                    if (Time.realtimeSinceStartup < state.NextTry) { yield return null; continue; }
                    Batch batch = null;
                    try { batch = Next(state); }
                    catch (Exception) { Retry(state); }
                    if (batch == null)
                    {
                        bool pending = false;
                        foreach (Group group in state.Groups) if (group.Exited && !group.Empty) pending = true;
                        if (!pending) yield break;
                    }
                    if (batch == null) { yield return null; continue; }
                    bool ready = false;
                    try { ready = QueuesEmpty() && NetworkUIIdle(); }
                    catch (Exception) { Retry(state); }
                    if (!ready) { state.NextTry = Math.Max(state.NextTry, Time.realtimeSinceStartup + .25f); yield return null; continue; }
                    // Re-read immediately before enqueueing after waiting for the channel.
                    try { batch = Next(state); }
                    catch (Exception) { Retry(state); batch = null; }
                    if (batch == null) { yield return null; continue; }
                    int opcode;
                    object request = NewRequest(batch.Kind, out opcode);
                    object attempt = new object();
                    state.Attempt = attempt;
                    bool done = false, applied = false;
                    HttpHandler callback = delegate(Packet packet, bool success)
                    {
                        if (done) return;
                        done = true;
                        if (!Owned(state) || !System.Object.ReferenceEquals(state.Attempt, attempt) || !success) return;
                        try { Apply(state, batch, packet); applied = true; }
                        catch (Exception) { }
                    };
                    bool sent = false;
                    try { sent = Send(opcode, request, callback); }
                    catch (Exception) { }
                    if (!sent)
                    {
                        try { ClearFailedOwnMask(state, batch); }
                        catch (Exception) { }
                    }
                    if (sent)
                    {
                        float deadline = Time.realtimeSinceStartup + 15f;
                        while (Owned(state) && !done && Time.realtimeSinceStartup < deadline) yield return null;
                    }
                    if (!Owned(state)) yield break;
                    if (!sent || !done || !applied)
                    {
                        state.Attempt = null;
                        Retry(state);
                    }
                    else { state.Failures = 0; state.NextTry = 0; }
                }
            }
            finally { state.Running = false; }
        }
    }
}
