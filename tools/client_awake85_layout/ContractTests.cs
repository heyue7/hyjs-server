using System;
using System.Collections.Generic;
using HYJS.DungeonContent;

static class Test
{
    private static int checks;
    private static void Check(bool value, string message)
    {
        checks++;
        if (!value) throw new InvalidOperationException(message);
    }
    private static void Reject(Action action, string message)
    {
        bool rejected = false;
        try { action(); } catch (InvalidOperationException) { rejected = true; }
        Check(rejected, message);
    }
    private static string[] Boxes(string run)
    {
        return new string[] { run + ":b1", run + ":b2", run + ":b3", run + ":b4" };
    }
    private sealed class Chapter { internal string Born; }
    private sealed class Cell { internal string Path; internal string Hash; internal long Size; }
    private static Cell NightCell()
    {
        return new Cell { Path = Awake85NightForest.Key, Hash = Awake85NightForest.Hash, Size = Awake85NightForest.Size };
    }
    private static bool IsNightCell(Cell value)
    {
        return value != null && Awake85NightForest.Matches(value.Path, value.Hash, value.Size);
    }
    private sealed class FailingAdd : IDictionary<string, Cell>
    {
        private readonly Dictionary<string, Cell> inner = new Dictionary<string, Cell>();
        public Cell this[string key] { get { return inner[key]; } set { inner[key] = value; } }
        public ICollection<string> Keys { get { return inner.Keys; } }
        public ICollection<Cell> Values { get { return inner.Values; } }
        public int Count { get { return inner.Count; } }
        public bool IsReadOnly { get { return false; } }
        public void Add(string key, Cell value) { throw new InvalidOperationException("simulated add failure"); }
        public bool ContainsKey(string key) { return inner.ContainsKey(key); }
        public bool Remove(string key) { return inner.Remove(key); }
        public bool TryGetValue(string key, out Cell value) { return inner.TryGetValue(key, out value); }
        public void Add(KeyValuePair<string, Cell> item) { Add(item.Key, item.Value); }
        public void Clear() { inner.Clear(); }
        public bool Contains(KeyValuePair<string, Cell> item) { return ((ICollection<KeyValuePair<string, Cell>>)inner).Contains(item); }
        public void CopyTo(KeyValuePair<string, Cell>[] array, int index) { ((ICollection<KeyValuePair<string, Cell>>)inner).CopyTo(array, index); }
        public bool Remove(KeyValuePair<string, Cell> item) { return ((ICollection<KeyValuePair<string, Cell>>)inner).Remove(item); }
        public IEnumerator<KeyValuePair<string, Cell>> GetEnumerator() { return inner.GetEnumerator(); }
        System.Collections.IEnumerator System.Collections.IEnumerable.GetEnumerator() { return GetEnumerator(); }
    }

    public static void Main()
    {
        string run = "p7-awake85-0123456789abcdef0123456789abcdef";
        string other = "p7-awake85-fedcba9876543210fedcba9876543210";
        string[] boxes = Boxes(run);
        string[] reversed = new string[] { boxes[3], boxes[1], boxes[0], boxes[2] };
        foreach (string chapter in new string[] { "1", "2", "3", "4", "5" })
        {
            Check(Awake85Contract.Identify(chapter, boxes, new string[] { run + ":m1" }) == run, "alive identity");
            Check(Awake85Contract.Identify(chapter, reversed, new string[0]) == run, "defeated identity");
        }
        Check(Awake85Contract.Identify("5", new string[] { "p7-awake-old:b1" }, new string[0]) == null, "legacy entry");
        for (int i = 0; i < 4; i++) Check(Awake85Contract.BoxSlot(reversed[i], run) == new int[] { 3, 1, 0, 2 }[i], "box suffix slot");
        Reject(delegate { Awake85Contract.Identify("0", boxes, new string[0]); }, "chapter guard");
        Reject(delegate { Awake85Contract.Identify("1", new string[] { boxes[0], boxes[1], boxes[2] }, new string[0]); }, "missing box");
        Reject(delegate { Awake85Contract.Identify("1", new string[] { boxes[0], boxes[1], boxes[2], boxes[2] }, new string[0]); }, "duplicate slot");
        Reject(delegate { Awake85Contract.Identify("1", new string[] { boxes[0], boxes[1], boxes[2], other + ":b4" }, new string[0]); }, "mixed runs");
        Reject(delegate { Awake85Contract.Identify("1", boxes, new string[] { other + ":m1" }); }, "foreign monster");
        Reject(delegate { Awake85Contract.Identify("1", boxes, new string[] { run + ":m1", run + ":m1" }); }, "extra monster");
        Reject(delegate { Awake85Contract.Identify("1", new string[] { boxes[0], boxes[1], boxes[2], run + ":b5" }, new string[0]); }, "invalid suffix");
        Reject(delegate { Awake85Contract.Identify("1", new string[] { boxes[0], boxes[1], boxes[2], "p7-awake85-ABCDEF0123456789abcdef0123456789:b4" }, new string[0]); }, "invalid nonce");

        float[] x = new float[] { 0, 20, -20, 20, -20, 40 };
        float[] z = new float[] { 0, 20, -20, -20, 20, 0 };
        bool[,] paths = new bool[6, 6];
        for (int i = 0; i < 6; i++) for (int j = 0; j < 6; j++) paths[i, j] = true;
        int[] aliveSlots = Awake85Contract.Select(x, z, paths);
        int[] deadSlots = Awake85Contract.Select(x, z, paths);
        Check(aliveSlots != null && deadSlots != null, "six slots exist");
        for (int i = 0; i < 6; i++) Check(aliveSlots[i] == deadSlots[i], "alive/dead fixed slot");
        Check(aliveSlots[1] == 0, "monster slot reserved");
        for (int i = 2; i < 6; i++)
        {
            double dx = x[aliveSlots[i]] - x[aliveSlots[1]], dz = z[aliveSlots[i]] - z[aliveSlots[1]];
            Check(dx * dx + dz * dz > 81.0 && paths[aliveSlots[0], aliveSlots[i]], "box unguarded and reachable");
        }
        for (int i = 0; i < 6; i++) for (int j = 0; j < 6; j++) paths[i, j] = false;
        Check(Awake85Contract.Select(x, z, paths) == null, "unreachable layout rejected");
        Reject(delegate { Awake85Contract.Select(new float[] { 0, 1, 2, 3, 4, Single.NaN }, z, paths); }, "non-finite candidate");

        var state = new Awake85State();
        var oldRoom = new object(); var oldEntry = new object();
        var chapterObject = new Chapter { Born = "0,0,1" };
        state.Bind(oldRoom, "5", oldEntry, run);
        state.CaptureBorn(chapterObject, chapterObject.Born);
        chapterObject.Born = "25,0,30";
        Check(state.Same(oldRoom, "5", oldEntry) && state.Active(oldRoom, "5", oldEntry, 10), "same room resume");
        Check(!state.Active(oldRoom, "5", oldEntry, 3), "scene type guard");
        Check(!state.Same(oldRoom, "5", new object()), "new entry boundary");
        int restores = 0;
        state.Leave(delegate(object target, string before) { ((Chapter)target).Born = before; restores++; });
        Check(restores == 1 && chapterObject.Born == "0,0,1", "old501 born restored");
        Check(!state.Active(oldRoom, "5", oldEntry, 10), "old entry unbound");
        state.Bind(new object(), "4", new object(), other);
        state.Leave(delegate(object target, string before) { throw new InvalidOperationException("unexpected restore"); });

        var nightState = new Awake85State();
        var nightRoom = new object(); var nightEntry = new object();
        nightState.Bind(nightRoom, "1", nightEntry, run);
        Check(Awake85NightForest.Applies(nightState, nightRoom, "1", nightEntry, 10), "active v85 chapter one");
        Check(!Awake85NightForest.Applies(nightState, nightRoom, "1", nightEntry, 3), "night scene type guard");
        Check(!Awake85NightForest.Applies(nightState, nightRoom, "2", nightEntry, 10), "night chapter guard");
        Check(!Awake85NightForest.Applies(nightState, nightRoom, "1", new object(), 10), "night entry guard");
        Check(!Awake85NightForest.Applies(null, nightRoom, "1", nightEntry, 10), "legacy does not register");
        Check(Awake85NightForest.ManifestText == "3000\nmap/map_nightforestfield_001\t1fdbc4decf711cab5e8031ecb2033079\t13943479\n", "exact native manifest input");

        var parsed = new Dictionary<string, Cell>();
        var nativeCell = NightCell(); parsed.Add(Awake85NightForest.Key, nativeCell);
        var full = new Dictionary<string, Cell>(); var small = new Dictionary<string, Cell>();
        var unrelated = new Cell { Path = "other", Hash = "existing", Size = 1 };
        full.Add("other", unrelated);
        Awake85NightForest.Register<Cell>(full, small, parsed, IsNightCell);
        Check(Object.ReferenceEquals(full[Awake85NightForest.Key], nativeCell) && Object.ReferenceEquals(small[Awake85NightForest.Key], nativeCell), "missing entries inserted");
        Check(full.Count == 2 && small.Count == 1 && Object.ReferenceEquals(full["other"], unrelated), "unrelated entry preserved");
        Awake85NightForest.Register<Cell>(full, small, parsed, IsNightCell);
        Check(full.Count == 2 && small.Count == 1, "registration idempotent");

        var preserved = NightCell();
        full = new Dictionary<string, Cell>(); small = new Dictionary<string, Cell>();
        full.Add(Awake85NightForest.Key, preserved);
        Awake85NightForest.Register<Cell>(full, small, parsed, IsNightCell);
        Check(Object.ReferenceEquals(full[Awake85NightForest.Key], preserved) && Object.ReferenceEquals(small[Awake85NightForest.Key], nativeCell), "existing matching identity preserved");
        full = new Dictionary<string, Cell>(); small = new Dictionary<string, Cell>();
        preserved = NightCell(); small.Add(Awake85NightForest.Key, preserved);
        Awake85NightForest.Register<Cell>(full, small, parsed, IsNightCell);
        Check(Object.ReferenceEquals(small[Awake85NightForest.Key], preserved) && Object.ReferenceEquals(full[Awake85NightForest.Key], nativeCell), "existing small matching identity preserved");
        var shared = new Dictionary<string, Cell>();
        Awake85NightForest.Register<Cell>(shared, shared, parsed, IsNightCell);
        Awake85NightForest.Register<Cell>(shared, shared, parsed, IsNightCell);
        Check(shared.Count == 1 && Object.ReferenceEquals(shared[Awake85NightForest.Key], nativeCell), "shared dictionary added once");

        full = new Dictionary<string, Cell>(); small = new Dictionary<string, Cell>();
        var conflict = NightCell(); conflict.Size++;
        small.Add(Awake85NightForest.Key, conflict);
        Reject(delegate { Awake85NightForest.Register<Cell>(full, small, parsed, IsNightCell); }, "conflict rejected");
        Check(full.Count == 0 && Object.ReferenceEquals(small[Awake85NightForest.Key], conflict), "conflict has no partial change");
        full = new Dictionary<string, Cell>(); small = new Dictionary<string, Cell>();
        conflict = NightCell(); conflict.Hash = "bad"; full.Add(Awake85NightForest.Key, conflict);
        Reject(delegate { Awake85NightForest.Register<Cell>(full, small, parsed, IsNightCell); }, "full conflict rejected");
        Check(small.Count == 0 && Object.ReferenceEquals(full[Awake85NightForest.Key], conflict), "full conflict untouched");
        var extra = new Dictionary<string, Cell>(parsed); extra.Add("other", unrelated);
        Reject(delegate { Awake85NightForest.Register<Cell>(new Dictionary<string, Cell>(), new Dictionary<string, Cell>(), extra, IsNightCell); }, "extra parsed entry rejected");
        var badParsed = new Dictionary<string, Cell>(); badParsed.Add(Awake85NightForest.Key, new Cell { Path = "wrong", Hash = Awake85NightForest.Hash, Size = Awake85NightForest.Size });
        Reject(delegate { Awake85NightForest.Register<Cell>(new Dictionary<string, Cell>(), new Dictionary<string, Cell>(), badParsed, IsNightCell); }, "parsed path rejected");
        Reject(delegate { Awake85NightForest.Register<Cell>(null, new Dictionary<string, Cell>(), parsed, IsNightCell); }, "uninitialized dictionary rejected");
        full = new Dictionary<string, Cell>();
        var failing = new FailingAdd();
        Reject(delegate { Awake85NightForest.Register<Cell>(full, failing, parsed, IsNightCell); }, "second addition failure propagated");
        Check(full.Count == 0 && failing.Count == 0, "first addition rolled back");
        Console.WriteLine("PASS " + checks + " actual contract checks");
    }
}
