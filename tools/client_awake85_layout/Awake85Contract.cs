using System;
using System.Collections.Generic;

namespace HYJS.DungeonContent
{
    // Pure checks shared by the Unity helper and the offline contract harness.
    internal static class Awake85Contract
    {
        private const string Prefix = "p7-awake85-";
        private const int NonceLength = 32;

        internal static string Identify(string chapter, string[] boxes, string[] monsters)
        {
            bool marked = false;
            if (boxes != null) foreach (string value in boxes)
                if (value != null && value.StartsWith(Prefix, StringComparison.Ordinal)) marked = true;
            if (monsters != null) foreach (string value in monsters)
                if (value != null && value.StartsWith(Prefix, StringComparison.Ordinal)) marked = true;
            if (!marked) return null;
            if ((chapter != "1" && chapter != "2" && chapter != "3" && chapter != "4" && chapter != "5") ||
                boxes == null || boxes.Length != 4 || monsters == null || monsters.Length > 1)
                throw new InvalidOperationException("Awake85 entry shape invalid");
            string run = null;
            bool[] seen = new bool[4];
            foreach (string value in boxes)
            {
                int slot;
                string candidate = RunFromBox(value, out slot);
                if (run == null) run = candidate;
                if (candidate != run || seen[slot]) throw new InvalidOperationException("Awake85 box run or slot differs");
                seen[slot] = true;
            }
            if (monsters.Length == 1 && monsters[0] != run + ":m1")
                throw new InvalidOperationException("Awake85 monster run differs");
            return run;
        }

        internal static int BoxSlot(string guid, string run)
        {
            int slot;
            if (RunFromBox(guid, out slot) != run) throw new InvalidOperationException("Awake85 box run differs");
            return slot;
        }

        private static string RunFromBox(string guid, out int slot)
        {
            int runLength = Prefix.Length + NonceLength;
            if (guid == null || guid.Length != runLength + 3 || !guid.StartsWith(Prefix, StringComparison.Ordinal) ||
                guid[runLength] != ':' || guid[runLength + 1] != 'b' || guid[runLength + 2] < '1' || guid[runLength + 2] > '4')
                throw new InvalidOperationException("Awake85 box GUID invalid");
            for (int i = Prefix.Length; i < runLength; i++)
                if (!((guid[i] >= '0' && guid[i] <= '9') || (guid[i] >= 'a' && guid[i] <= 'f')))
                    throw new InvalidOperationException("Awake85 run nonce invalid");
            slot = guid[runLength + 2] - '1';
            return guid.Substring(0, runLength);
        }

        // Six output slots are independent of whether the one monster remains.
        // Every object is reachable from spawn and outside the conservative
        // 9-unit encounter radius of the monster slot.
        internal static int[] Select(float[] x, float[] z, bool[,] paths)
        {
            if (x == null || z == null || x.Length != z.Length || x.Length < 6 || paths == null ||
                paths.GetLength(0) != x.Length || paths.GetLength(1) != x.Length)
                throw new InvalidOperationException("Awake85 candidate shape invalid");
            for (int i = 0; i < x.Length; i++)
                if (Single.IsNaN(x[i]) || Single.IsInfinity(x[i]) || Single.IsNaN(z[i]) || Single.IsInfinity(z[i]))
                    throw new InvalidOperationException("Awake85 candidate coordinate invalid");
            for (int monster = 0; monster < x.Length; monster++)
            {
                for (int spawn = 0; spawn < x.Length; spawn++)
                {
                    if (spawn == monster || !Outside(x, z, spawn, monster) || !paths[spawn, monster]) continue;
                    int[] slots = new int[6];
                    slots[0] = spawn;
                    slots[1] = monster;
                    int next = 2;
                    for (int box = 0; box < x.Length && next < slots.Length; box++)
                        if (box != monster && box != spawn && Outside(x, z, box, monster) && paths[spawn, box])
                            slots[next++] = box;
                    if (next == slots.Length) return slots;
                }
            }
            return null;
        }

        private static bool Outside(float[] x, float[] z, int first, int second)
        {
            double dx = (double)x[first] - x[second];
            double dz = (double)z[first] - z[second];
            return dx * dx + dz * dz > 81.0;
        }
    }

    // Only one known packaged map is registered for an active v85 chapter-one entry.
    internal static class Awake85NightForest
    {
        internal const string Key = "map/map_nightforestfield_001";
        internal const string Hash = "1fdbc4decf711cab5e8031ecb2033079";
        internal const long Size = 13943479;
        internal const string ManifestText = "3000\n" + Key + "\t" + Hash + "\t13943479\n";

        internal static bool Applies(Awake85State state, object room, string chapter, object entry, int sceneType)
        {
            return chapter == "1" && state != null && state.Active(room, chapter, entry, sceneType);
        }

        internal static bool Matches(string path, string hash, long size)
        {
            return path == Key && hash == Hash && size == Size;
        }

        // Validate both current dictionaries before either addition. An existing
        // entry is never replaced; a shared dictionary is changed only once.
        internal static void Register<T>(IDictionary<string, T> full, IDictionary<string, T> small,
            IDictionary<string, T> parsed, Func<T, bool> expected) where T : class
        {
            if (full == null || small == null || parsed == null || expected == null || parsed.Count != 1)
                throw new InvalidOperationException("Awake85 night manifest unavailable");
            T added, fullValue, smallValue;
            if (!parsed.TryGetValue(Key, out added) || added == null || !expected(added))
                throw new InvalidOperationException("Awake85 night manifest invalid");
            bool inFull = full.TryGetValue(Key, out fullValue);
            bool inSmall = small.TryGetValue(Key, out smallValue);
            if ((inFull && (fullValue == null || !expected(fullValue))) ||
                (inSmall && (smallValue == null || !expected(smallValue))))
                throw new InvalidOperationException("Awake85 night manifest conflict");
            bool shared = Object.ReferenceEquals(full, small);
            if (!inFull) full.Add(Key, added);
            try
            {
                if (!inSmall && !shared) small.Add(Key, added);
            }
            catch
            {
                T current;
                if (!inFull && full.TryGetValue(Key, out current) && Object.ReferenceEquals(current, added))
                    full.Remove(Key);
                throw;
            }
        }
    }

    // Only v85 entries bind this state. A transition restores the captured
    // shared chapter object before a later entry can read its birth position.
    internal sealed class Awake85State
    {
        private object room;
        private object entry;
        private string chapter;
        private string run;
        private object bornOwner;
        private string previousBorn;

        internal bool Same(object currentRoom, string currentChapter, object currentEntry)
        {
            return Object.ReferenceEquals(room, currentRoom) && Object.ReferenceEquals(entry, currentEntry) &&
                chapter == currentChapter && run != null;
        }

        internal bool Active(object currentRoom, string currentChapter, object currentEntry, int sceneType)
        {
            return sceneType == 10 && Same(currentRoom, currentChapter, currentEntry);
        }

        internal string Run { get { return run; } }

        internal void Bind(object currentRoom, string currentChapter, object currentEntry, string currentRun)
        {
            if (currentRoom == null || currentEntry == null || currentRun == null || run != null)
                throw new InvalidOperationException("Awake85 state bind invalid");
            room = currentRoom; chapter = currentChapter; entry = currentEntry; run = currentRun;
        }

        internal void CaptureBorn(object owner, string value)
        {
            if (owner == null || value == null || run == null || bornOwner != null)
                throw new InvalidOperationException("Awake85 birth capture invalid");
            bornOwner = owner; previousBorn = value;
        }

        internal void Leave(Action<object, string> restore)
        {
            if (bornOwner != null)
            {
                if (restore == null) throw new InvalidOperationException("Awake85 birth restore unavailable");
                restore(bornOwner, previousBorn);
            }
            room = null; entry = null; chapter = null; run = null; bornOwner = null; previousBorn = null;
        }
    }
}
