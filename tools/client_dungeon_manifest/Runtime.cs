using System;
using System.IO;
using System.Security.Cryptography;
using UnityEngine;
namespace HYJS.DungeonManifest {
 public static class ManifestPath {
  public const string KnownManifestSHA256 = "28e264a89fb6b1a5fc27fa5a6fbdfb55cf510e4ba6e6431d1a023631d620f28d";
  private const int MaximumBytes = 2097152;
  private static readonly object Gate = new object();
  private static string cachedPath;
  private static long cachedLength, cachedWrite;
  private static bool cachedMatch;
  public static string Resolve(string originalResult, string name) {
   if (name != "android") return originalResult;
   try {
    string expected = Application.persistentDataPath + "/android";
    if (originalResult != expected) return originalResult;
    bool match;
    lock (Gate) {
     FileInfo info = new FileInfo(expected);
     if (!info.Exists || info.Length <= 0 || info.Length > MaximumBytes) return originalResult;
     long length = info.Length, write = info.LastWriteTimeUtc.Ticks;
     if (cachedPath == expected && cachedLength == length && cachedWrite == write) match = cachedMatch;
     else {
      byte[] bytes = new byte[(int)length];
      using (FileStream stream = new FileStream(expected, FileMode.Open, FileAccess.Read, FileShare.ReadWrite)) {
       int offset = 0;
       while (offset < bytes.Length) { int read = stream.Read(bytes, offset, bytes.Length-offset); if (read <= 0) return originalResult; offset += read; }
       if (stream.ReadByte() != -1) return originalResult;
      }
      using (SHA256 digest = SHA256.Create()) { match = BitConverter.ToString(digest.ComputeHash(bytes)).Replace("-", "").ToLowerInvariant() == KnownManifestSHA256; }
      info.Refresh();
      if (!info.Exists || info.Length != length || info.LastWriteTimeUtc.Ticks != write) return originalResult;
      cachedPath = expected; cachedLength = length; cachedWrite = write; cachedMatch = match;
     }
    }
    return match ? Application.dataPath + "!assets/android/" + name : originalResult;
   } catch { return originalResult; }
  }
 }
}
