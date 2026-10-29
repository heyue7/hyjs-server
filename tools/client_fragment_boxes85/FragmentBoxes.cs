using System;
using System.Globalization;
using Dict.DataModel;

namespace HYJS.FragmentBoxes85
{
    // The native selection list expects exactly "resourceId,quantity" per row.
    // Inspect the same runtime table that populates that list.
    public static class FragmentBoxes
    {
        const string CountKey = "fragmentCount85";

        public static bool IsEligible(string id)
        {
            if (String.IsNullOrEmpty(id)) return false;
            TableItem item = Dict.Blo.DictItemBlo.GetTableItem(id);
            if (item == null || item.category != 5) return false;
            var choices = Dict.Blo.DictItemBlo.GetTableItemUseList(id);
            if (choices == null || choices.Count == 0) return false;
            foreach (TableItemUse choice in choices)
            {
                if (choice == null || String.IsNullOrEmpty(choice.result)) return false;
                string[] parts = choice.result.Split(',');
                int amount;
                if (parts.Length != 2 || !parts[0].StartsWith("frag_", StringComparison.Ordinal) ||
                    parts[0].Length <= 5 ||
                    !Int32.TryParse(parts[1], NumberStyles.None, CultureInfo.InvariantCulture, out amount) ||
                    amount <= 0) return false;
            }
            return true;
        }

        public static long Limit(long owned)
        {
            return owned > 100L ? 100L : owned;
        }

        // The count belongs to the options of this particular page opening.
        public static string AppendCount(string originalOptions, long selected, string id)
        {
            if (originalOptions != "id=" + id || !IsEligible(id) || selected < 1 || selected > 100)
                return originalOptions;
            return originalOptions + "&" + CountKey + "=" + selected.ToString(CultureInfo.InvariantCulture);
        }

        public static int RequestedCount(UI.Page page, string id)
        {
            string optionId, raw;
            int selected;
            if (page == null || page.options == null ||
                !page.options.TryGetValue("id", out optionId) || optionId != id ||
                !page.options.TryGetValue(CountKey, out raw) || !IsEligible(id) ||
                !Int32.TryParse(raw, NumberStyles.None, CultureInfo.InvariantCulture, out selected) ||
                selected < 1 || selected > 100) return 1;
            return selected;
        }
    }
}
