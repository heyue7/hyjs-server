// Local501 resource readiness only. The original global full/download flags,
// getter, downloader, resource loader and battle engine remain untouched.
using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.IO.Compression;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using System.Threading;

namespace HYJS.LocalAssets
{
    public sealed class ResourceValidationException : IOException
    { public ResourceValidationException(string message) : base(message) { } }

    public sealed class ResourceRow
    {
        public string Entry, SHA256, MD5;
        public long Size;
    }

    // Used unchanged by the Android bridge and the offline executable check.
    public static class VerifiedPackage
    {
        sealed class Entry { public string Name; public long Size, Packed, Offset; public int Method, Flags; }
        sealed class Limited : Stream
        {
            readonly Stream source; long left;
            public Limited(Stream stream, long length) { source = stream; left = length; }
            public override int Read(byte[] buffer, int offset, int count)
            { int n = source.Read(buffer, offset, (int)Math.Min(count, left)); left -= n; return n; }
            protected override void Dispose(bool disposing) { if (disposing) source.Dispose(); base.Dispose(disposing); }
            public override bool CanRead { get { return true; } } public override bool CanSeek { get { return false; } }
            public override bool CanWrite { get { return false; } } public override long Length { get { throw new NotSupportedException(); } }
            public override long Position { get { throw new NotSupportedException(); } set { throw new NotSupportedException(); } }
            public override void Flush() { } public override long Seek(long x, SeekOrigin y) { throw new NotSupportedException(); }
            public override void SetLength(long x) { throw new NotSupportedException(); }
            public override void Write(byte[] b, int o, int c) { throw new NotSupportedException(); }
        }
        static byte[] Read(BinaryReader reader, int count)
        { byte[] raw = reader.ReadBytes(count); if (raw.Length != count) throw new ResourceValidationException("ZIP truncated"); return raw; }
        static uint U32(byte[] b, int p) { return BitConverter.ToUInt32(b, p); }
        static ushort U16(byte[] b, int p) { return BitConverter.ToUInt16(b, p); }
        static Dictionary<string, Entry> Index(string apk)
        {
            using (FileStream file = File.OpenRead(apk))
            using (BinaryReader reader = new BinaryReader(file))
            {
                int length = (int)Math.Min(file.Length, 65557); file.Position = file.Length - length;
                byte[] tail = Read(reader, length); int end = -1;
                for (int i = length - 22; i >= 0; --i)
                    if (U32(tail, i) == 0x06054b50 && i + 22 + U16(tail, i + 20) == length) { end = i; break; }
                if (end < 0 || U16(tail, end + 4) != 0 || U16(tail, end + 6) != 0 ||
                    U16(tail, end + 8) != U16(tail, end + 10) || U16(tail, end + 10) == 65535)
                    throw new ResourceValidationException("Unsupported ZIP/ZIP64 directory");
                long start = U32(tail, end + 16), size = U32(tail, end + 12);
                if (start + size > file.Length - length + end || size > 16000000) throw new ResourceValidationException("ZIP directory bounds");
                file.Position = start; byte[] directory = Read(reader, (int)size); int at = 0;
                Dictionary<string, Entry> result = new Dictionary<string, Entry>(StringComparer.Ordinal);
                for (int i = 0; i < U16(tail, end + 10); ++i)
                {
                    if (at + 46 > directory.Length || U32(directory, at) != 0x02014b50) throw new ResourceValidationException("ZIP directory row");
                    int names = U16(directory, at + 28), extra = U16(directory, at + 30), comment = U16(directory, at + 32);
                    if (at + 46 + names + extra + comment > directory.Length) throw new ResourceValidationException("ZIP filename bounds");
                    string name = new UTF8Encoding(false, true).GetString(directory, at + 46, names);
                    if (result.ContainsKey(name)) throw new ResourceValidationException("Duplicate ZIP name");
                    Entry row = new Entry(); row.Name = name; row.Flags = U16(directory, at + 8); row.Method = U16(directory, at + 10);
                    row.Packed = U32(directory, at + 20); row.Size = U32(directory, at + 24); row.Offset = U32(directory, at + 42);
                    if (row.Offset + 30 + row.Packed > start) throw new ResourceValidationException("ZIP local bounds");
                    result.Add(name, row); at += 46 + names + extra + comment;
                }
                if (at != directory.Length) throw new ResourceValidationException("ZIP trailing directory data");
                return result;
            }
        }
        static Stream Open(string apk, Entry entry)
        {
            FileStream file = File.OpenRead(apk);
            try
            {
                file.Position = entry.Offset; BinaryReader reader = new BinaryReader(file);
                byte[] header = Read(reader, 30);
                if (U32(header, 0) != 0x04034b50 || (entry.Flags & 1) != 0 || U16(header, 8) != entry.Method ||
                    (entry.Method != 0 && entry.Method != 8)) throw new ResourceValidationException("Unsupported resource ZIP entry");
                string name = new UTF8Encoding(false, true).GetString(Read(reader, U16(header, 26)));
                if (name != entry.Name) throw new ResourceValidationException("ZIP local name differs");
                file.Position += U16(header, 28);
                Stream bounded = new Limited(file, entry.Packed);
                return entry.Method == 0 ? bounded : (Stream)new DeflateStream(bounded, CompressionMode.Decompress);
            }
            catch { file.Dispose(); throw; }
        }
        public static string Hex(byte[] value) { return BitConverter.ToString(value).Replace("-", "").ToLowerInvariant(); }
        static bool IsHex(string value, int length)
        {
            if (value.Length != length) return false;
            foreach (char c in value) if (!((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'))) return false;
            return true;
        }
        public static List<ResourceRow> Parse(string text)
        {
            string[] lines = text.Split(new char[] { '\n' }, StringSplitOptions.RemoveEmptyEntries);
            if (lines.Length < 2 || lines[0] != "local-awake501-original-resource-closure-v1") throw new ResourceValidationException("Resource proof schema");
            List<ResourceRow> rows = new List<ResourceRow>(); HashSet<string> seen = new HashSet<string>(StringComparer.Ordinal);
            for (int i = 1; i < lines.Length; ++i)
            {
                string[] parts = lines[i].Split('\t'); long size;
                if (parts.Length != 4 || !parts[0].StartsWith("assets/", StringComparison.Ordinal) ||
                    parts[0].Contains("..") || parts[0].Contains("\\") || !seen.Add(parts[0]) ||
                    !Int64.TryParse(parts[1], NumberStyles.None, CultureInfo.InvariantCulture, out size) || size <= 0 ||
                    !IsHex(parts[2], 64) || !IsHex(parts[3], 32)) throw new ResourceValidationException("Resource proof row");
                ResourceRow row = new ResourceRow(); row.Entry = parts[0]; row.Size = size; row.SHA256 = parts[2]; row.MD5 = parts[3]; rows.Add(row);
            }
            return rows;
        }
        static void Hash(Stream input, ResourceRow row)
        {
            using (input) using (SHA256 hash = SHA256.Create())
            {
                long size = 0; byte[] buffer = new byte[65536]; int count;
                while ((count = input.Read(buffer, 0, buffer.Length)) > 0)
                { size += count; if (size > row.Size) throw new ResourceValidationException("Resource exceeds declared size"); hash.TransformBlock(buffer, 0, count, buffer, 0); }
                hash.TransformFinalBlock(new byte[0], 0, 0);
                if (size != row.Size || Hex(hash.Hash) != row.SHA256) throw new ResourceValidationException("Resource bytes differ: " + row.Entry);
            }
        }
        public static int Verify(string apk, string cache, List<ResourceRow> rows)
        {
            Dictionary<string, Entry> entries = Index(apk); int overrides = 0;
            foreach (ResourceRow row in rows)
            {
                Entry entry;
                if (!entries.TryGetValue(row.Entry, out entry) || entry.Size != row.Size) throw new ResourceValidationException("Missing resource: " + row.Entry);
                Hash(Open(apk, entry), row);
                if (row.Entry.StartsWith("assets/android/", StringComparison.Ordinal))
                {
                    string path = Path.Combine(cache, row.Entry.Substring(15));
                    if (File.Exists(path)) { Hash(File.OpenRead(path), row); ++overrides; }
                }
            }
            return overrides;
        }
        // The original loader prefers persistent files over packaged bundles.
        // A new/changed override invalidates readiness until its bytes are verified.
        public static string CacheVersion(string cache, List<ResourceRow> rows)
        {
            StringBuilder text = new StringBuilder();
            foreach (ResourceRow row in rows)
            {
                if (!row.Entry.StartsWith("assets/android/", StringComparison.Ordinal)) continue;
                FileInfo file = new FileInfo(Path.Combine(cache, row.Entry.Substring(15)));
                text.Append(row.Entry).Append('\t');
                if (file.Exists) text.Append(file.Length).Append('\t').Append(file.LastWriteTimeUtc.Ticks);
                text.Append('\n');
            }
            return text.ToString();
        }
    }

#if !HYJS_ASSET_VERIFY_ONLY
    public static class LocalAssetReady
    {
        const BindingFlags Flags = BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Static | BindingFlags.Instance;
        static int state; static string failure; static List<ResourceRow> rows;
        static string verifiedCacheVersion, cachePath;
        static int reportedState;
        static readonly Dictionary<string, string> diagnostics = new Dictionary<string, string>();
        static int diagnosticCount;
        // Only public gate/resource metadata; never account, password or session data.
        static void Trace(string stage, string detail)
        {
            try {
            if (detail == null) detail = "null";
            detail = detail.Replace('\r', ' ').Replace('\n', ' ');
            if (detail.Length > 320) detail = detail.Substring(0, 320);
            lock (diagnostics)
            {
                string previous;
                if (diagnosticCount >= 64 || (diagnostics.TryGetValue(stage, out previous) && previous == detail)) return;
                diagnostics[stage] = detail; ++diagnosticCount;
            }
            string message = "Local501 " + stage + " " + detail;
            // Android's own log survives release builds that suppress Unity's
            // informational logger. Diagnostics never participate in readiness.
            try {
                using (UnityEngine.AndroidJavaClass log = new UnityEngine.AndroidJavaClass("android.util.Log"))
                    log.CallStatic<int>("i", "HYJS501", message);
            } catch { }
            UnityEngine.Debug.LogWarning(message);
            } catch { /* Diagnostics must not change the original full=true path. */ }
        }
        static bool Closed(string stage, string detail) { Trace(stage, detail); return false; }
        static object Member(object value, string name)
        {
            if (value == null) return null;
            for (Type type = value.GetType(); type != null; type = type.BaseType)
            {
                FieldInfo field = type.GetField(name, Flags | BindingFlags.DeclaredOnly);
                if (field != null) return field.GetValue(value);
                PropertyInfo property = type.GetProperty(name, Flags | BindingFlags.DeclaredOnly);
                if (property != null) return property.GetValue(value, null);
            }
            return null;
        }
        static object Singleton(string name)
        {
            Type type = Type.GetType(name + ", Assembly-CSharp", true);
            return type.GetProperty("Instance", Flags | BindingFlags.FlattenHierarchy).GetValue(null, null);
        }
        public static void StartVerification()
        {
            if (Interlocked.CompareExchange(ref state, 1, 0) != 0) return;
            try
            {
                string apk = UnityEngine.Application.dataPath, cache = UnityEngine.Application.persistentDataPath;
                cachePath = cache;
                if (UnityEngine.Application.platform != UnityEngine.RuntimePlatform.Android || !File.Exists(apk)) throw new ResourceValidationException("Expected installed Android APK");
                string text;
                using (Stream source = typeof(LocalAssetReady).Assembly.GetManifestResourceStream("local501.inventory"))
                using (MemoryStream copy = new MemoryStream())
                {
                    if (source == null) throw new ResourceValidationException("Missing embedded resource proof");
                    byte[] buffer = new byte[65536]; int count;
                    while ((count = source.Read(buffer, 0, buffer.Length)) > 0) copy.Write(buffer, 0, count);
                    byte[] raw = copy.ToArray();
                    using (SHA256 hash = SHA256.Create()) if (VerifiedPackage.Hex(hash.ComputeHash(raw)) != LocalResourceProof.InventorySHA256) throw new ResourceValidationException("Embedded proof differs");
                    text = Encoding.UTF8.GetString(raw);
                }
                rows = VerifiedPackage.Parse(text);
                ThreadPool.QueueUserWorkItem(delegate(object ignored)
                {
                    try {
                        string before = VerifiedPackage.CacheVersion(cache, rows);
                        VerifiedPackage.Verify(apk, cache, rows);
                        string after = VerifiedPackage.CacheVersion(cache, rows);
                        if (before != after) throw new ResourceValidationException("Resource cache changed during verification");
                        verifiedCacheVersion = after;
                        Thread.VolatileWrite(ref state, 2);
                    }
                    catch (Exception error) { failure = error.Message; Thread.VolatileWrite(ref state, -1); }
                });
            }
            catch (Exception error) { failure = error.Message; state = -1; }
        }
        static bool Ready()
        {
            StartVerification();
            int current = Thread.VolatileRead(ref state);
            if (current != reportedState)
            {
                reportedState = current;
                Trace("resource", "verification state=" + current + (failure == null ? "" : " reason=" + failure));
            }
            if (current != 2) return Closed("verify", "state=" + current + (failure == null ? "" : " reason=" + failure));
            try
            {
                if (VerifiedPackage.CacheVersion(cachePath, rows) != verifiedCacheVersion)
                {
                    failure = "Resource cache changed; verification required";
                    Interlocked.CompareExchange(ref state, 0, 2);
                    return Closed("cache", "changed; recheck required");
                }
                Type render = Type.GetType("RenderManager, Assembly-CSharp", true);
                int quality = Convert.ToInt32(render.GetProperty("Quality", Flags).GetValue(null, null));
                if (quality != 2) return Closed("quality", "actual=" + quality + " expected=2");
                if (!PartyCovered()) return false;
                object manager = Singleton("AssetLoad.AssetBundleManager");
                if (!Convert.ToBoolean(Member(manager, "IsInitialized"))) return Closed("loader", "not initialized");
                foreach (string name in new string[] { "smallAssetFileTool", "assetFileTool" })
                {
                    object file = Member(manager, name); if (file == null) return Closed("manifest", name + " missing");
                    IDictionary cells = Member(file, "assetCells") as IDictionary; if (cells == null) return Closed("manifest", name + " assetCells unavailable");
                    foreach (ResourceRow row in rows)
                    {
                        if (!row.Entry.StartsWith("assets/android/", StringComparison.Ordinal) || row.Entry.EndsWith("/smallassetmanifest", StringComparison.Ordinal)) continue;
                        string path = row.Entry.Substring(15); object cell = cells[path];
                        if (cell == null) return Closed("cell", name + " missing " + path);
                        if (Convert.ToInt64(Member(cell, "size")) != row.Size || Convert.ToString(Member(cell, "hashCode")) != row.MD5)
                            return Closed("cell", name + " size/hash differs " + path);
                    }
                }
                Trace("ready", "verified scoped501"); return true;
            }
            catch (Exception error) { failure = error.Message; return Closed("ready-exception", error.GetType().FullName + ": " + error.Message); }
        }
        static bool PartyCovered()
        {
            // The frozen dynamic resource closure covers the actual ordinary
            // 001/003 party and pet_002 only. No stats or battle input change.
            object inst = Member(StaticData.playerData, "instPlayerData");
            if (inst == null) return Closed("party", "instPlayerData unavailable");
            IDictionary camps = Member(inst, "BattleCamps") as IDictionary;
            if (camps == null) return Closed("party", "BattleCamps unavailable");
            int heroes = 0;
            foreach (DictionaryEntry entry in camps)
            {
                object hero = Member(entry.Value, "hero"), pet = Member(entry.Value, "pet");
                string id = Convert.ToString(Member(hero, "heroId"));
                if (!String.IsNullOrEmpty(id))
                {
                    if (id != "hero_001" && id != "hero_003") return Closed("party", "uncovered hero=" + id);
                    ++heroes;
                }
                string petID = Convert.ToString(Member(pet, "id"));
                if (!String.IsNullOrEmpty(petID) && petID != "pet_002") return Closed("party", "uncovered pet=" + petID);
            }
            return heroes > 0 || Closed("party", "no heroes");
        }
        public static bool CatalogueReady(bool originalFull, object caller)
        {
            Trace("catalogue-entry", "originalFull=" + originalFull + " caller=" + (caller == null ? "null" : caller.GetType().FullName));
            if (originalFull) return true;
            try {
                if (caller == null || caller.GetType().FullName != "Room.RCUINormalNPC") return Closed("catalogue-predicate", "caller mismatch");
                string name = Convert.ToString(Member(caller, "pageName")), option = Convert.ToString(Member(caller, "pageOption"));
                object level = Member(caller, "pageLevel");
                Trace("catalogue-fields", "pageName=" + name + " pageOption=" + option + " pageLevel=" + Convert.ToString(level));
                if (name != "ChallengeActivityCopyPage" || option != "type=0" || Convert.ToInt32(level) != 70) return Closed("catalogue-predicate", "original NPC310 fields differ");
                return Ready();
            }
            catch (Exception error) { return Closed("catalogue-exception", error.GetType().FullName + ": " + error.Message); }
        }
        public static bool EntryReady(bool originalFull, object caller)
        {
            Trace("entry", "originalFull=" + originalFull + " caller=" + (caller == null ? "null" : caller.GetType().FullName));
            if (originalFull) return true;
            try {
            if (caller == null || caller.GetType().FullName != "AwakeDiffItem") return Closed("entry-predicate", "caller mismatch");
            object task = Member(caller, "taskInfo"), page = Member(caller, "page");
            if (task == null || page == null || Convert.ToString(Member(task, "copyId")) != "501") return Closed("entry-predicate", "task/page unavailable or copy differs");
            object chapter = page.GetType().GetMethod("GetCurrChapterID", Flags).Invoke(page, null);
            if (Convert.ToString(chapter) != "5") return Closed("entry-predicate", "chapter=" + Convert.ToString(chapter));
            return Ready();
            } catch (Exception error) { return Closed("entry-exception", error.GetType().FullName + ": " + error.Message); }
        }
    }
#endif
}
