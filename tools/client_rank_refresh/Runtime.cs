using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using NetWork.Layer;
using NetWork.Packets;
using NetWork.Packets.Factory;
using ProtoBuf.Message;
using UnityEngine;

namespace HYJS.Ranks
{
    public static class Refresh
    {
        // Called after the original worship action has succeeded. Any earlier
        // rank snapshot is now stale; keep the view and schedule a fresh read.
        public static void WorshipCompleted(MonoBehaviour owner, Packet packet, bool success)
        {
            if (!success || owner == null || packet == null || !(packet.kBody is SCGood)) return;
            try { RankRefreshRunner.AfterWorship(owner); }
            catch (Exception) { }
        }

        // Injected only at the selected read-only leaderboard call sites.
        // Unsupported opcodes, request types and non-component handlers keep
        // the original entry point without installing any observer.
        public static bool Send(int opcode, object request, HttpHandler handler)
        {
            MonoBehaviour owner = handler == null ? null : handler.Target as MonoBehaviour;
            if (!RankSelection.Allowed(opcode, request) || owner == null)
                return NetAPI.SendHttp(opcode, request, handler);

            RankSelection selection = null;
            object generation = null;
            try
            {
                generation = SelectionGenerations.Begin(owner);
                RankRefreshRunner.CancelFor(owner);
                selection = RankSelection.Capture(owner, generation, opcode, request, handler);
            }
            catch (Exception) { selection = null; }

            if (selection == null)
                return NetAPI.SendHttp(opcode, request, handler);

            RankSelection captured = selection;
            HttpHandler observe = delegate(Packet packet, bool success)
            {
                byte[] displayed = null;
                try
                {
                    if (success && packet != null && packet.nOpCode == captured.Opcode + 1 && packet.kBody != null &&
                        captured.Owned() && captured.VisiblePage() && captured.RequestUnchanged())
                        displayed = PBConvert.toBytes(packet.nOpCode, packet.kBody);
                }
                catch (Exception) { displayed = null; }

                // The native initial callback always receives its original
                // arguments, including failures and unexpected packets.
                handler(packet, success);

                try
                {
                    if (displayed != null && captured.Owned() && captured.VisiblePage() && captured.RequestUnchanged())
                    {
                        captured.LastDisplayed = displayed;
                        RankRefreshRunner.Install(captured);
                    }
                }
                catch (Exception) { }
            };

            bool sent = NetAPI.SendHttp(opcode, request, observe);
            if (!sent) SelectionGenerations.Invalidate(owner, generation);
            return sent;
        }
    }

    public sealed class RankRefreshRunner : MonoBehaviour
    {
        private const float Interval = 15f;
        private const float Timeout = 5f;
        private RankSelection selection;
        private WWW pending;
        private CSUnionSearch pendingFirstPage;
        private float next;
        private float deadline;

        internal static void CancelFor(MonoBehaviour owner)
        {
            RankRefreshRunner[] runners = owner.gameObject.GetComponents<RankRefreshRunner>();
            for (int i = 0; i < runners.Length; ++i)
                if (runners[i] != null && runners[i].selection != null && System.Object.ReferenceEquals(runners[i].selection.Owner, owner))
                    runners[i].Cancel();
        }

        internal static void Install(RankSelection value)
        {
            RankRefreshRunner runner = null;
            RankRefreshRunner[] runners = value.Owner.gameObject.GetComponents<RankRefreshRunner>();
            for (int i = 0; i < runners.Length; ++i)
            {
                if (runners[i] != null && runners[i].selection != null && System.Object.ReferenceEquals(runners[i].selection.Owner, value.Owner))
                { runner = runners[i]; break; }
                if (runners[i] != null && runners[i].selection == null && runner == null) runner = runners[i];
            }
            if (runner == null) runner = value.Owner.gameObject.AddComponent<RankRefreshRunner>();
            runner.Cancel();
            runner.selection = value;
            runner.next = Time.realtimeSinceStartup + Interval;
        }

        internal static void AfterWorship(MonoBehaviour owner)
        {
            RankRefreshRunner[] runners = owner.gameObject.GetComponents<RankRefreshRunner>();
            for (int i = 0; i < runners.Length; ++i)
            {
                RankRefreshRunner runner = runners[i];
                if (runner == null || runner.selection == null || !System.Object.ReferenceEquals(runner.selection.Owner, owner)) continue;
                if (!runner.selection.Owned()) { runner.Cancel(); continue; }
                runner.DisposePending();
                runner.selection.Generation = SelectionGenerations.Begin(owner);
                runner.next = Time.realtimeSinceStartup + Interval;
            }
        }

        private void Update()
        {
            try { Tick(); }
            catch (Exception)
            {
                // A transport or decode exception is a failed attempt. Keep
                // the last displayed table and try again at the next period.
                DisposePending();
            }
        }

        private void Tick()
        {
            RankSelection current = selection;
            if (current == null) return;
            if (!current.Owned()) { Cancel(); return; }
            if (!current.VisiblePage())
            {
                DisposePending();
                return;
            }

            float now = Time.realtimeSinceStartup;
            if (pending != null)
            {
                if (now >= deadline) { DisposePending(); return; }
                if (!pending.isDone) return;
                WWW result = pending;
                CSUnionSearch firstPage = pendingFirstPage;
                pending = null;
                pendingFirstPage = null;
                try
                {
                    if (!String.IsNullOrEmpty(result.error)) return;
                    byte[] bytes = result.bytes;
                    if (bytes == null || bytes.Length == 0 || !current.Owned() || !current.VisiblePage() || !current.RequestUnchanged()) return;
                    int responseOpcode;
                    object body;
                    List<Packet> extras;
                    if (!PacketFactory.WebBytes2PB(bytes, out responseOpcode, out body, out extras) ||
                        responseOpcode != current.Opcode + 1 || body == null) return;
                    if (current.Owner.GetType().FullName == "UI.UnionRankPage" && current.Opcode == 57071)
                    {
                        SCUnionSearch unions = body as SCUnionSearch;
                        CSUnionSearch requested = current.Request as CSUnionSearch;
                        if (unions == null || unions.unions == null || requested == null) return;
                        if (firstPage == null && requested.page > 1 && unions.unions.Count == 0)
                        {
                            BeginFirstPage(current, requested, now);
                            return;
                        }
                    }
                    byte[] displayed = PBConvert.toBytes(responseOpcode, body);
                    if (displayed == null || (firstPage == null && RankSelection.EqualBytes(current.LastDisplayed, displayed))) return;
                    if (!current.Owned() || !current.VisiblePage() || !current.RequestUnchanged()) return;
                    Packet packet = new Packet(responseOpcode, body);
                    if (firstPage == null) Apply(current, packet, displayed);
                    else ApplyFirstPage(current, packet, displayed);
                }
                finally { result.Dispose(); }
                return;
            }

            if (now < next) return;
            next = now + Interval;
            if (!current.RequestUnchanged()) { Cancel(); return; }
            byte[] frame;
            if (!PacketFactory.WebPB2Bytes(current.Opcode, current.Request, out frame) || frame == null || frame.Length == 0) return;
            if (!current.Owned() || !current.VisiblePage()) return;
            pending = new WWW(current.Url, frame);
            deadline = now + Timeout;
        }

        // One additional request is allowed only for an emptied non-first
        // union page. The displayed page stays untouched until page 1 succeeds.
        private void BeginFirstPage(RankSelection current, CSUnionSearch requested, float now)
        {
            if (!current.Owned() || !current.VisiblePage() || !current.RequestUnchanged()) return;
            CSUnionSearch first = new CSUnionSearch();
            first.name = requested.name;
            first.page = 1;
            byte[] frame;
            if (!PacketFactory.WebPB2Bytes(current.Opcode, first, out frame) || frame == null || frame.Length == 0) return;
            if (!current.Owned() || !current.VisiblePage()) return;
            pending = new WWW(current.Url, frame);
            pendingFirstPage = first;
            deadline = now + Timeout;
        }

        private void ApplyFirstPage(RankSelection current, Packet packet, byte[] displayed)
        {
            CSUnionSearch request = current.Request as CSUnionSearch;
            if (request == null || current.SelectionField == null || request.page <= 1) return;
            int previousPage = request.page;
            object previousSelection = current.SelectionValue;
            byte[] previousBytes = current.RequestBytes;
            try
            {
                request.page = 1;
                byte[] updatedBytes = PBConvert.toBytes(current.Opcode, request);
                if (updatedBytes == null) throw new InvalidOperationException("Union page request could not be serialized");
                current.SelectionField.SetValue(current.Owner, 1);
                current.SelectionValue = 1;
                current.RequestBytes = updatedBytes;
                Apply(current, packet, displayed);
            }
            catch (Exception)
            {
                if (System.Object.ReferenceEquals(selection, current) && SelectionGenerations.Current(current.Owner, current.Generation))
                {
                    request.page = previousPage;
                    current.SelectionField.SetValue(current.Owner, previousSelection);
                    current.SelectionValue = previousSelection;
                    current.RequestBytes = previousBytes;
                }
                throw;
            }
        }

        private void Apply(RankSelection current, Packet packet, byte[] displayed)
        {
            ScrollPosition[] positions = ScrollPosition.Capture(current.Owner);
            try
            {
                if (!CleanRankRows(current)) return;
                if (!CleanFamous(current)) return;
                current.Handler(packet, true);
                if (System.Object.ReferenceEquals(selection, current) && current.Owned()) current.LastDisplayed = displayed;
            }
            finally { ScrollPosition.Restore(positions); }
        }

        private static bool CleanRankRows(RankSelection current)
        {
            Type type = current.Owner.GetType();
            if (type.FullName != "UI.RankPage") return true;
            const BindingFlags flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
            FieldInfo field = type.GetField("itemList", flags);
            IList rows = field == null ? null : field.GetValue(current.Owner) as IList;
            if (rows == null) return false;
            for (int i = 0; i < rows.Count; ++i)
                if (rows[i] != null && !(rows[i] is GameObject)) return false;
            for (int i = 0; i < rows.Count; ++i)
            {
                GameObject row = rows[i] as GameObject;
                if (row == null) continue;
                row.transform.parent = null;
                row.SetActive(false);
                UnityEngine.Object.Destroy(row);
            }
            rows.Clear();
            return true;
        }

        private static bool CleanFamous(RankSelection current)
        {
            if (current.Opcode != 23023) return true;
            Type type = current.Owner.GetType();
            if (type.FullName != "UI.PowerPage" && type.FullName != "UI.LevelPage") return true;
            const BindingFlags flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
            MethodInfo clean = type.GetMethod("clean", flags, null, Type.EmptyTypes, null);
            FieldInfo field = type.GetField("playerFlagList", flags);
            IList flagsList = field == null ? null : field.GetValue(current.Owner) as IList;
            if (clean == null || flagsList == null) return false;
            clean.Invoke(current.Owner, null);
            flagsList.Clear();
            return true;
        }

        private void DisposePending()
        {
            WWW value = pending;
            pending = null;
            pendingFirstPage = null;
            if (value != null) value.Dispose();
        }

        private void Cancel()
        {
            DisposePending();
            RankSelection value = selection;
            selection = null;
            if (value != null) SelectionGenerations.Invalidate(value.Owner, value.Generation);
        }

        private void OnDisable() { Cancel(); }
        private void OnDestroy() { Cancel(); }

        private sealed class ScrollPosition
        {
            private UIScrollView scroll;
            private UIPanel panel;
            private Vector3 localPosition;
            private Vector2 clipOffset;

            internal static ScrollPosition[] Capture(MonoBehaviour owner)
            {
                UIScrollView[] scrolls = owner.GetComponentsInChildren<UIScrollView>(true);
                List<ScrollPosition> positions = new List<ScrollPosition>();
                for (int i = 0; i < scrolls.Length; ++i)
                {
                    UIScrollView scroll = scrolls[i];
                    if (scroll == null) continue;
                    ScrollPosition position = new ScrollPosition();
                    position.scroll = scroll;
                    position.panel = scroll.panel;
                    position.localPosition = scroll.transform.localPosition;
                    if (position.panel != null) position.clipOffset = position.panel.clipOffset;
                    positions.Add(position);
                }
                return positions.ToArray();
            }

            internal static void Restore(ScrollPosition[] positions)
            {
                for (int i = 0; i < positions.Length; ++i)
                {
                    ScrollPosition position = positions[i];
                    if (position.scroll != null) position.scroll.transform.localPosition = position.localPosition;
                    if (position.panel != null) position.panel.clipOffset = position.clipOffset;
                }
            }
        }
    }
}
