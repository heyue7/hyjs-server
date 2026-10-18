// Display the server's actual named local objective in the reused original panel.
using System;
using System.Reflection;
using ProtoBuf.Message;

public static class LocalActivityInformation
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

    public static void Apply(object panel, object packet, bool success)
    {
        if (!success) return;
        ActivityItem item = packet as ActivityItem;
        if (item != null)
        {
            if ((Field(panel, "activityId") as string) != "totalLogin_p1_directory" || String.IsNullOrEmpty(item.message)) return;
            UILabel title = Field(panel, "labTitle") as UILabel;
            if (title != null)
            {
                title.text = item.message;
                // The original localized title includes an inline dark color;
                // the server caption is plain text, so restore readable contrast.
                title.color = new UnityEngine.Color(0.13f, 0.13f, 0.13f, 1f);
            }
            return;
        }
        SCActivityInfo info = Field(packet, "kBody") as SCActivityInfo;
        if (info == null || info.activityId == null || !(info.activityId.StartsWith("kaiGongActivity_p4_", StringComparison.Ordinal) || info.activityId == "totalLogin_p1_directory")
            || String.IsNullOrEmpty(info.information)) return;
        UILabel label = Field(panel, "topTips") as UILabel;
        if (label != null) label.text = info.information;
    }
}
