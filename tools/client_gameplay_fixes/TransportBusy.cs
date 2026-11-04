using System;
using System.Net;
using System.Reflection;

namespace HYJS.GameplayFixes
{
    public static class TransportBusy
    {
        static readonly BindingFlags Fields = BindingFlags.Instance | BindingFlags.NonPublic | BindingFlags.Public;
        static readonly Type ManagerType = typeof(NetAPI).Assembly.GetType("NetWork.Layer.HTTPManager", true);
        static readonly Type UtilType = typeof(NetAPI).Assembly.GetType("NetWork.Layer.HttpUtil", true);
        static readonly FieldInfo UtilField = RequireField(ManagerType, "httpUtil", UtilType);
        static readonly FieldInfo ClientField = RequireClientField();

        static FieldInfo RequireField(Type owner, string name, Type fieldType)
        {
            FieldInfo field = owner.GetField(name, Fields);
            if (field == null || field.FieldType != fieldType) throw new MissingFieldException(owner.FullName, name);
            return field;
        }

        static FieldInfo RequireClientField()
        {
            FieldInfo field = UtilType.GetField("webClient", Fields);
            if (field == null || !typeof(WebClient).IsAssignableFrom(field.FieldType))
                throw new MissingFieldException(UtilType.FullName, "webClient");
            return field;
        }

        public static bool IsSending(object manager)
        {
            if (manager == null || !ManagerType.IsInstanceOfType(manager))
                throw new InvalidOperationException("HTTP manager is unavailable");
            object util = UtilField.GetValue(manager);
            if (util == null || !UtilType.IsInstanceOfType(util))
                throw new InvalidOperationException("HTTP transport is unavailable");
            WebClient client = ClientField.GetValue(util) as WebClient;
            if (client == null) throw new InvalidOperationException("HTTP client is unavailable");
            return client.IsBusy;
        }
    }
}
