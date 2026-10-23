using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using UnityEngine;
using UI;

namespace HYJS.OneKey59
{
    public sealed class DeferredSelection : MonoBehaviour
    {
        internal void Schedule(BagSellPage page, GameObject button) { StartCoroutine(Apply(page, button)); }
        IEnumerator Apply(BagSellPage page, GameObject button)
        {
            object owner = StaticData.playerData;
            byte[] key = StaticData.sessionKey == null ? null : (byte[])StaticData.sessionKey.Clone();
            int kind = Selection.Kind(page);
            yield return null;
            if (page == null || button == null || !page.gameObject.activeInHierarchy || owner != StaticData.playerData ||
                kind != Selection.Kind(page) || !Selection.SameSession(key)) yield break;
            UIToggle toggle = button.GetComponent<UIToggle>();
            if (toggle != null) Selection.Apply(page, button.name, toggle.value);
        }
        void OnDisable() { StopAllCoroutines(); }
    }

    public static class Selection
    {
        const BindingFlags Flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        static void Limit() { BoxManager.CreatePopupTextBox("每次最多选择100件，剩余物品可在本次出售后继续选择", 0f); }
        static object Field(object value, string name)
        {
            if (value == null) return null;
            for (Type t = value.GetType(); t != null; t = t.BaseType)
            {
                FieldInfo field = t.GetField(name, Flags | BindingFlags.DeclaredOnly);
                if (field != null) return field.GetValue(value);
            }
            throw new InvalidOperationException("Missing original field " + name);
        }
        static object Call(object value, string name, params object[] args)
        {
            MethodInfo method = value.GetType().GetMethod(name, Flags);
            if (method == null) throw new InvalidOperationException("Missing original method " + name);
            return method.Invoke(value, args);
        }
        public static int Kind(BagSellPage page) { return Convert.ToInt32(Field(page, "bagType")); }
        internal static bool SameSession(byte[] saved)
        {
            byte[] current = StaticData.sessionKey;
            if (saved == null || current == null) return saved == current;
            if (saved.Length != current.Length) return false;
            for (int i = 0; i < saved.Length; i++) if (saved[i] != current[i]) return false;
            return true;
        }
        static Dictionary<string, string> Selected(BagSellPage page) { return (Dictionary<string, string>)Field(page, "selectedGuids"); }
        public static void BeforeSuccessfulRefresh(BagSellPage page)
        {
            // Called only after original SellCallback has accepted its success branch.
            page.StopAllCoroutines();
            FieldInfo count = typeof(BagSellPage).GetField("curPetCount", Flags);
            if (count == null) throw new InvalidOperationException("Missing original pet page counter");
            count.SetValue(page, 0);
        }
        public static void QueueQuick(BagSellPage page, GameObject button)
        {
            if (page == null || button == null) return;
            DeferredSelection control = page.GetComponent<DeferredSelection>();
            if (control == null) control = page.gameObject.AddComponent<DeferredSelection>();
            control.Schedule(page, button);
        }
        public static bool GateString(BagSellPage page, string guid)
        {
            var selected = Selected(page);
            if (guid != null && selected.ContainsKey(guid)) return false;
            if (selected.Count < 100) return false;
            Limit(); return true;
        }
        public static bool GateEquip(BagSellPage page, EquipIcon icon) { return GateString(page, icon == null ? null : icon.guid); }
        sealed class Candidate { public string Id; public int Quality; public bool Trained; }
        static void Add(List<Candidate> result, IEnumerable values, string data, bool pet)
        {
            foreach (object value in values)
            {
                object table = Field(value, data);
                result.Add(new Candidate { Id = (string)Field(value, "guid"), Quality = (int)Field(table, "quality"), Trained = pet && (bool)Call(value, "IsTrained") });
            }
        }
        static void AddFragments(List<Candidate> result, IEnumerable values, int kind)
        {
            foreach (UIFragItem value in values)
            {
                string target = value.fragData.target;
                int quality = kind == 4 ? Dict.Blo.DictHeroBlo.GetTableHero(target).quality :
                    kind == 5 ? Dict.Blo.DictEquipBlo.GetTableEquip(target).quality : Dict.Blo.DictPetBlo.GetTablePet(target).quality;
                result.Add(new Candidate { Id = value.fragData.id, Quality = quality });
            }
        }
        static List<Candidate> Candidates(BagSellPage page)
        {
            var result = new List<Candidate>(); var player = StaticData.playerData;
            var inst = player.InstPlayerData; int kind = Kind(page);
            if (kind == 2) Add(result, inst.GetHeroListForSellInOrder(), "heroData", false);
            else if (kind == 3) { Add(result, inst.GetEquipListForSell(), "equipData", false); Add(result, inst.GetTreasureListForSell(), "treasureData", false); }
            else if (kind == 8) Add(result, inst.GetPetListForSellInOrder(), "tableData", true);
            else if (kind == 4) AddFragments(result, player.GetFragHerosForSellInOrder(), kind);
            else if (kind == 5) AddFragments(result, player.GetEquipFragListForSellInOrder(), kind);
            else if (kind == 9) AddFragments(result, player.GetFragPetsForSellInOrder(), kind);
            return result;
        }
        public static void Apply(BagSellPage page, string buttonName, bool select)
        {
            int index; int[] qualities = new int[] { 8, 12, 13, 18, 20 };
            if (!Int32.TryParse(buttonName, out index) || index < 1 || index > qualities.Length) return;
            var selected = Selected(page); var candidates = Candidates(page); var ids = new List<string>();
            foreach (Candidate candidate in candidates)
                if (candidate.Quality == qualities[index - 1] && !candidate.Trained && !String.IsNullOrEmpty(candidate.Id)) ids.Add(candidate.Id);
            bool capped = ApplyIds(selected, ids, select);
            Draw(page); Call(page, "UpdateGetThingInfo"); if (capped) Limit();
        }
        internal static bool ApplyIds(Dictionary<string,string> selected, IList<string> ids, bool select)
        {
            bool capped = false;
            foreach (string id in ids)
            {
                if (!select) selected.Remove(id);
                else if (!selected.ContainsKey(id))
                {
                    if (selected.Count >= 100) capped = true;
                    else selected[id] = id;
                }
            }
            return capped;
        }
        public static void Draw(BagSellPage page)
        {
            if (page == null) return;
            var selected = Selected(page);
            var icons = (Dictionary<string, GameObject>)Field(page, "sellPanelIcons");
            foreach (var pair in icons) if (pair.Value != null) Call(page, "SetIconChosed", pair.Value, selected.ContainsKey(pair.Key));
        }
    }
}
