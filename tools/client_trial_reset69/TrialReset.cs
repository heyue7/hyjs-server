using System;
using System.Reflection;
using ProtoBuf.Message;
using UI;
using UnityEngine;

namespace HYJS.TrialReset69
{
    public sealed class ResetLayout : MonoBehaviour
    {
        internal bool captured;
        internal Vector3 fightPosition, resetPosition;
    }

    public static class Controls
    {
        static GameObject Button(object page, string name)
        {
            FieldInfo field = page.GetType().GetField(name, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            return field == null ? null : field.GetValue(page) as GameObject;
        }

        public static void AfterRefresh(object owner, object response)
        {
            RushPage page = owner as RushPage;
            SCGetRushInfo info = response as SCGetRushInfo;
            if (page == null || info == null) return;
            GameObject fight = Button(page, "fightBtn"), reset = Button(page, "resetBtn");
            if (fight == null || reset == null || fight.transform.parent != reset.transform.parent) return;
            ResetLayout layout = page.gameObject.GetComponent<ResetLayout>();
            if (layout == null) layout = page.gameObject.AddComponent<ResetLayout>();
            if (!layout.captured)
            {
                layout.fightPosition = fight.transform.localPosition;
                layout.resetPosition = reset.transform.localPosition;
                layout.captured = true;
            }
            fight.transform.localPosition = layout.fightPosition;
            reset.transform.localPosition = layout.resetPosition;
            // A failed run keeps its original single reset action. During a live
            // challenge, the original page also owns visibility and input.
            if (info.isMustReset > 0 || !fight.activeSelf || info.resetLeftTime + info.diamondRestLeftTime <= 0) return;
            float width = NGUIMath.CalculateRelativeWidgetBounds(fight.transform).size.x;
            float offset = Mathf.Max(84f, width * 0.5f + 12f);
            Vector3 center = layout.fightPosition;
            fight.transform.localPosition = new Vector3(center.x - offset, center.y, center.z);
            reset.transform.localPosition = new Vector3(center.x + offset, center.y, center.z);
            reset.SetActive(true);
            // Both buttons retain the original handlers. The server validates
            // progress, pending ownership, available resets and payment.
        }
    }
}
