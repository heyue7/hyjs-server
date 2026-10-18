using System;
using System.Collections.Generic;
using System.Reflection;
using ProtoBuf.Message;
using UI;
using UnityEngine;

// Opt-in local APK UI adapter. All mutations go through original 21049.
// Never changes a warrior, server save, battle result, Engine or resource gate.
namespace HYJS.LocalFormation
{
    public static class LocalFormationUI
    {
        internal const BindingFlags Fields = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;

        internal static bool OtherPlayer(ArrayPage page)
        {
            FieldInfo field = typeof(ArrayPage).GetField("isOtherPlayer", Fields);
            return field == null || (bool)field.GetValue(page);
        }

        internal static bool Bare(UIHeroFormation camp)
        {
            if (camp == null || camp.pet != null) return false;
            foreach (UIEquipment equipment in camp.equipments) if (equipment != null) return false;
            foreach (UITreasure treasure in camp.treasures) if (treasure != null) return false;
            return true;
        }

        internal static int OccupiedCount(UIInstPlayerData inst)
        {
            int count = 0;
            foreach (KeyValuePair<int, UIHeroFormation> entry in inst.BattleCamps)
                if (entry.Key >= 1 && entry.Key <= 5 && entry.Value != null && entry.Value.hero != null) ++count;
            return count;
        }

        internal static bool SameBytes(byte[] first, byte[] second)
        {
            if (first == null || second == null || first.Length != 16 || second.Length != 16) return false;
            for (int n = 0; n < first.Length; ++n) if (first[n] != second[n]) return false;
            return true;
        }

        internal static void Notice(string text) { BoxManager.CreatePopupTextBox(text, 0f); }

        // Invoked by the already running wallet adapter through reflection.
        // No original ArrayPage method references this assembly.
        public static bool Ensure(ArrayPage array)
        {
            try
            {
                if (Application.platform != RuntimePlatform.Android || array == null || OtherPlayer(array)) return false;
                UIPlayerData player = StaticData.playerData;
                if (player == null || player.playerId <= 1) return false;
                FormationControls controls = array.GetComponent<FormationControls>();
                if (controls == null) controls = array.gameObject.AddComponent<FormationControls>();
                controls.Initialize(array);
                Trace("attached", "original ArrayPage initialized; real button ready");
                return true;
            }
            catch (Exception error)
            {
                try
                {
                    FormationControls controls = array == null ? null : array.GetComponent<FormationControls>();
                    if (controls != null) controls.DiscardBroken();
                }
                catch (Exception) { }
                Trace("attach-failed", error.GetType().FullName);
                return false;
            }
        }

        static readonly string[] diagnostics = new string[16];
        static int diagnosticCount;
        internal static void Trace(string stage, string detail)
        {
            string line = stage + " " + detail;
            for (int n = 0; n < diagnosticCount; ++n) if (diagnostics[n] == line) return;
            if (diagnosticCount >= diagnostics.Length) return;
            diagnostics[diagnosticCount++] = line;
            try
            {
                using (AndroidJavaClass log = new AndroidJavaClass("android.util.Log"))
                    log.CallStatic<int>("w", "HYJSFormation", line);
            }
            catch (Exception) { Debug.LogWarning("LocalFormation " + line); }
        }

        internal sealed class Pending
        {
            readonly ArrayPage array;
            readonly FormationControls controls;
            readonly UIPlayerData player;
            readonly UIInstPlayerData inst;
            readonly UIHero hero;
            readonly UIHeroFormation camp;
            readonly byte[] session;
            readonly string guid, heroID;
            readonly int position;
            readonly long playerID;
            readonly Dictionary<Collider, bool> colliders = new Dictionary<Collider, bool>();
            internal bool busy, committed;

            internal Pending(ArrayPage a, FormationControls d, UIPlayerData p, UIInstPlayerData i, UIHero h, UIHeroFormation c, byte[] s)
            {
                array = a; controls = d; player = p; inst = i; hero = h; camp = c; session = s;
                guid = h.guid; heroID = h.heroId; position = c.campPos; playerID = p.playerId;
            }

            bool SameSession()
            {
                return System.Object.ReferenceEquals(player, StaticData.playerData) && player.playerId == playerID &&
                    System.Object.ReferenceEquals(inst, player.InstPlayerData) && SameBytes(session, StaticData.sessionKey);
            }

            bool SameSlot()
            {
                UIHeroFormation current;
                return SameSession() && playerID > 1 && OccupiedCount(inst) > 1 &&
                    inst.BattleCamps.TryGetValue(position, out current) && System.Object.ReferenceEquals(current, camp) &&
                    System.Object.ReferenceEquals(current.hero, hero) && System.Object.ReferenceEquals(hero.owner, camp) &&
                    hero.guid == guid && hero.heroId == heroID && System.Object.ReferenceEquals(inst.GetUIHero(guid), hero);
            }

            void Block(bool value)
            {
                busy = value;
                if (value && array != null)
                {
                    foreach (Collider collider in array.GetComponentsInChildren<Collider>(true))
                    {
                        if (!colliders.ContainsKey(collider)) colliders[collider] = collider.enabled;
                        collider.enabled = false;
                    }
                }
                if (!value)
                {
                    foreach (KeyValuePair<Collider, bool> entry in colliders) if (entry.Key != null) entry.Key.enabled = entry.Value;
                    colliders.Clear();
                }
            }

            public void Click()
            {
                if (busy || !SameSession()) return;
                if (committed) { Fetch(); return; }
                if (!SameSlot()) { Notice("角色阵容已变化，请重新打开阵容"); return; }
                if (array == null || PageManager.Instance.currentPage != array) return;
                Block(true);
                try
                {
                    NetAPI.SendHttp(21049, new CSFormationOff { id = heroID, guid = guid, formatPos = position },
                        delegate(NetWork.Packets.Packet packet, bool success)
                        {
                            Block(false);
                            if (!SameSession()) return;
                            if (!success || packet == null || !(packet.kBody is SCFormationOff))
                            {
                                Notice("下阵尚未确认，阵容未作本地修改，可重试"); return;
                            }
                            committed = true;
                            controls.SetCaption("同步阵容");
                            Fetch();
                        });
                }
                catch (Exception) { Block(false); if (SameSession()) Notice("下阵尚未确认，阵容未作本地修改，可重试"); }
            }

            void Fetch()
            {
                if (busy || !SameSession()) return;
                Block(true);
                try
                {
                    NetAPI.SendHttp(20111, new CSFormationList(), delegate(NetWork.Packets.Packet packet, bool success)
                    {
                        Block(false);
                        if (!SameSession()) return;
                        SCFormationList reply = packet == null ? null : packet.kBody as SCFormationList;
                        if (!success || reply == null)
                        {
                            Notice("下阵已提交，请点击同步阵容重试"); return;
                        }
                        try { Apply(reply); }
                        catch (Exception) { Notice("下阵已提交，阵容同步未完成，请重新登录"); }
                    });
                }
                catch (Exception) { Block(false); if (SameSession()) Notice("下阵已提交，请点击同步阵容重试"); }
            }

            void Apply(SCFormationList reply)
            {
                // Preflight the complete original response before touching any
                // cached hero/owner. No client-side candidate winning state.
                var positions = new Dictionary<int, bool>();
                var guids = new Dictionary<string, bool>();
                foreach (ProtoBuf.Message.FormationItem item in reply.formationList)
                {
                    if (item == null || item.formatPos < 1 || item.formatPos > 5 || positions.ContainsKey(item.formatPos)) throw new InvalidOperationException();
                    positions.Add(item.formatPos, true);
                    FormationThing actor = null;
                    foreach (FormationThing thing in item.things)
                    {
                        if (thing == null || String.IsNullOrEmpty(thing.guid) || String.IsNullOrEmpty(thing.id)) throw new InvalidOperationException();
                        if (thing.pos == 0)
                        {
                            UIHero owned = inst.GetUIHero(thing.guid);
                            if (actor != null || owned == null || owned.heroId != thing.id || guids.ContainsKey(thing.guid) || thing.guid == guid) throw new InvalidOperationException();
                            actor = thing; guids.Add(thing.guid, true);
                        }
                        else if (thing.pos >= 1 && thing.pos <= 4) { if (inst.GetUIEquipment(thing.guid) == null) throw new InvalidOperationException(); }
                        else if (thing.pos >= 5 && thing.pos <= 6) { if (inst.GetUITreasure(thing.guid) == null) throw new InvalidOperationException(); }
                        else if (thing.pos == 7) { if (inst.GetUIPet(thing.guid) == null) throw new InvalidOperationException(); }
                        else throw new InvalidOperationException();
                    }
                    // Empty protocol rows are incompatible with original
                    // RefreshLuckData; root must omit them in 20111 output.
                    if (actor == null) throw new InvalidOperationException();
                }
                if (guids.Count < 1 || array == null || PageManager.Instance.currentPage != array || !SameSlot()) throw new InvalidOperationException();
                MethodInfo unload = typeof(FormationResourceCache).GetMethod("UnloadHero", Fields, null, new Type[] { typeof(string) }, null);
                if (unload == null) throw new InvalidOperationException();

                // Closing while the old cache is still valid avoids original
                // page callbacks dereferencing an empty current slot.
                array.Close();
                var empty = new List<int>();
                foreach (KeyValuePair<int, UIHeroFormation> entry in inst.BattleCamps)
                    if (!positions.ContainsKey(entry.Key) && entry.Value.hero != null) empty.Add(entry.Key);
                foreach (int key in empty)
                {
                    UIHeroFormation previous = inst.BattleCamps[key];
                    UIHero removed = previous.hero;
                    removed.owner = null;
                    // Server 21049 clears this complete slot while retaining all
                    // owned inventory. Match that authoritative detach locally.
                    foreach (UIEquipment equipment in previous.equipments)
                        if (equipment != null) { equipment.owner = null; equipment.lastHeroId = ""; }
                    foreach (UITreasure treasure in previous.treasures)
                        if (treasure != null) treasure.owner = null;
                    if (previous.pet != null) previous.pet.owner = null;
                    unload.Invoke(ManagerController.Instance.formationResourceCache, new object[] { removed.heroId });
                    inst.BattleCamps[key] = new UIHeroFormation(key, previous.battlePos, inst);
                }
                // Existing original initialization handles every authoritative
                // nonempty row, inventory references, pets, luck and properties.
                inst.InitBattleCamp(reply.formationList, false);
                if (!String.IsNullOrEmpty(reply.tacticalSkillId)) inst.UpdateFormationData(reply.tacticalSkillId);
                inst.ResetAllFormationAttribute();
                if (StaticData.funcNavKit != null) StaticData.funcNavKit.OnFormationChange();
                int selected = 0;
                foreach (KeyValuePair<int, UIHeroFormation> entry in inst.BattleCamps)
                    if (entry.Value.hero != null && (selected == 0 || entry.Key < selected)) selected = entry.Key;
                controls.Complete();
                if (selected > 0) PageManager.Instance.OpenPage("ArrayPage", "pos=" + selected);
                Notice("角色已下阵，仍保留在背包中");
            }
        }
    }
    public sealed class FormationControls : MonoBehaviour
    {
        ArrayPage array;
        GameObject button;
        UILabel caption;
        LocalFormationUI.Pending pending;

        public void Initialize(ArrayPage page)
        {
            array = page;
            if (button != null) { button.SetActive(true); return; }
            FieldInfo field = typeof(ArrayPage).GetField("tabProperty", LocalFormationUI.Fields);
            Component tab = field == null ? null : field.GetValue(page) as Component;
            if (tab == null) throw new InvalidOperationException("Original property tab missing");
            UISprite[] sprites = tab.GetComponentsInChildren<UISprite>(true);
            UILabel[] labels = tab.GetComponentsInChildren<UILabel>(true);
            if (sprites.Length == 0 || labels.Length == 0 || sprites[0].atlas == null) throw new InvalidOperationException("Original NGUI theme missing");
            int depth = 0;
            foreach (UIWidget widget in page.GetComponentsInChildren<UIWidget>(true)) depth = Math.Max(depth, widget.depth);
            button = new GameObject("LocalFormationUnload");
            button.layer = tab.gameObject.layer;
            button.transform.parent = page.transform;
            button.transform.localScale = Vector3.one;
            // Original ArrayPage coordinates: above the model, clear of
            // the right detail panel and the bottom replace/upgrade controls.
            button.transform.localPosition = new Vector3(40f, 230f, 0f);
            UISprite background = button.AddComponent<UISprite>();
            background.atlas = sprites[0].atlas;
            background.spriteName = sprites[0].spriteName;
            background.type = UIBasicSprite.Type.Sliced;
            background.pivot = UIWidget.Pivot.Center;
            background.width = 110; background.height = 46; background.depth = depth + 10;
            GameObject text = new GameObject("Caption");
            text.layer = button.layer; text.transform.parent = button.transform;
            text.transform.localScale = Vector3.one; text.transform.localPosition = Vector3.zero;
            caption = text.AddComponent<UILabel>();
            caption.bitmapFont = labels[0].bitmapFont;
            caption.trueTypeFont = labels[0].trueTypeFont;
            caption.fontStyle = labels[0].fontStyle;
            caption.fontSize = 22; caption.width = 106; caption.height = 42;
            caption.pivot = UIWidget.Pivot.Center; caption.alignment = NGUIText.Alignment.Center;
            caption.color = labels[0].color; caption.depth = depth + 11; caption.text = "下阵";
            BoxCollider collider = button.AddComponent<BoxCollider>();
            collider.center = Vector3.zero; collider.size = new Vector3(110f, 46f, 1f);
            UIEventListener.Get(button).onClick = delegate(GameObject ignored) { Click(); };
            LocalFormationUI.Trace("button", "visible ArrayPage control created");
        }

        void LateUpdate()
        {
            if (button == null || array == null) return;
            UIPlayerData player = StaticData.playerData;
            bool visible = player != null && player.playerId > 1 && !LocalFormationUI.OtherPlayer(array) && PageManager.Instance.currentPage == array;
            if (button.activeSelf != visible) button.SetActive(visible);
        }

        void Click()
        {
            if (array == null || PageManager.Instance.currentPage != array || LocalFormationUI.OtherPlayer(array)) return;
            if (pending != null && (pending.busy || pending.committed)) { pending.Click(); return; }
            UIPlayerData player = StaticData.playerData;
            byte[] session = StaticData.sessionKey;
            if (player == null || player.playerId <= 1 || session == null || session.Length != 16) return;
            FieldInfo field = typeof(ArrayPage).GetField("heroGuid", LocalFormationUI.Fields);
            string guid = field == null ? null : field.GetValue(array) as string;
            UIInstPlayerData inst = player.InstPlayerData;
            UIHero hero = String.IsNullOrEmpty(guid) ? null : inst.GetUIHero(guid);
            if (hero == null || hero.owner == null) { LocalFormationUI.Notice("请先选择已上阵角色"); return; }
            if (LocalFormationUI.OccupiedCount(inst) <= 1) { LocalFormationUI.Notice("阵容中至少需要保留一名角色"); return; }
            UIHeroFormation camp = hero.owner, current;
            if (camp.campPos < 1 || camp.campPos > 5 || !inst.BattleCamps.TryGetValue(camp.campPos, out current) ||
                !System.Object.ReferenceEquals(camp, current) || !System.Object.ReferenceEquals(camp.hero, hero)) return;
            pending = new LocalFormationUI.Pending(array, this, player, inst, hero, camp, (byte[])session.Clone());
            pending.Click();
        }

        internal void DiscardBroken()
        {
            if (button != null) Destroy(button);
            button = null; caption = null;
        }

        internal void SetCaption(string text) { if (caption != null) caption.text = text; }
        internal void Complete() { pending = null; SetCaption("下阵"); }
    }


}
