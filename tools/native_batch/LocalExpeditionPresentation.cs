// The local backend has no restored leader-fragment reward. Keep selection and
// engine state untouched and display only the explicit server reward preview.
using System;
using System.Collections;
using System.Reflection;
using UnityEngine;

public static class LocalExpeditionPresentation
{
    private static object Field(object value, string name)
    {
        if (value == null) return null;
        for (Type type = value.GetType(); type != null; type = type.BaseType)
        {
            FieldInfo field = type.GetField(name, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.DeclaredOnly);
            if (field != null) return field.GetValue(value);
        }
        return null;
    }

    public static void Apply(object panel)
    {
        if (panel == null || panel.GetType().FullName != "UI.SelectPlayerPanel") return;
        string preview = Field(panel, "possibleReward") as string;
        if ((Field(panel, "bigReward") as string) != "" || String.IsNullOrEmpty(preview) || preview.IndexOf(',') >= 0) return;
        IList selected = Field(panel, "flag") as IList;
        IList icons = Field(panel, "allList") as IList;
        if (selected == null || selected.Count == 0 || icons == null) return;
        int entries = 0;
        foreach (string id in preview.Split(';')) if (id.Length > 0) entries++;
        if (icons.Count != entries + 1) return;
        GameObject unsupportedHint = icons[0] as GameObject;
        if (unsupportedHint == null || unsupportedHint.name != "001") return;
        icons.RemoveAt(0);
        UnityEngine.Object.Destroy(unsupportedHint);
        GameObject gridObject = Field(panel, "giftGrid") as GameObject;
        if (gridObject == null) return;
        UIGrid grid = gridObject.GetComponent<UIGrid>();
        if (grid != null) { grid.Reposition(); grid.repositionNow = true; }
    }
}
