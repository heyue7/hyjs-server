using System;
using System.Collections.Generic;
using System.Text;

namespace HYJS.DesertV4
{
    // The original game's LitJson cannot read an array whose immediate parent
    // is another array. Wrap only those values before parsing; their contents,
    // numeric text, string escapes and array order stay unchanged.
    public static class NestedArrayJson
    {
        public const string Key = "__hyjs_nested_array";
        struct Frame
        {
            public char opener;
            public bool wrapped;
            public Frame(char value, bool isWrapped) { opener=value; wrapped=isWrapped; }
        }
        public static string Normalize(string raw)
        {
            if (raw==null || raw.Length>2*1024*1024) throw new ArgumentException("Invalid JSON response");
            var output=new StringBuilder(raw.Length+64);
            var stack=new List<Frame>();
            bool quoted=false, escaped=false;
            foreach (char c in raw)
            {
                if (quoted)
                {
                    output.Append(c);
                    if (escaped) escaped=false;
                    else if (c=='\\') escaped=true;
                    else if (c=='"') quoted=false;
                    continue;
                }
                if (c=='"') { quoted=true; output.Append(c); continue; }
                if (c=='[' || c=='{')
                {
                    bool wrapped=c=='[' && stack.Count>0 && stack[stack.Count-1].opener=='[';
                    if (stack.Count>=64) throw new ArgumentException("JSON nesting too deep");
                    if (wrapped) output.Append("{\"" + Key + "\":");
                    output.Append(c); stack.Add(new Frame(c,wrapped));
                    continue;
                }
                if (c==']' || c=='}')
                {
                    if (stack.Count==0) throw new ArgumentException("Unbalanced JSON response");
                    Frame frame=stack[stack.Count-1]; stack.RemoveAt(stack.Count-1);
                    if (frame.opener!=(c==']' ? '[' : '{')) throw new ArgumentException("Unbalanced JSON response");
                    output.Append(c); if (frame.wrapped) output.Append('}');
                    continue;
                }
                output.Append(c);
            }
            if (quoted || stack.Count!=0) throw new ArgumentException("Unbalanced JSON response");
            return output.ToString();
        }
    }
}
