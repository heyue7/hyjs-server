namespace HYJS.Latency62 {
public static class Policy {
 public const float CompactAfter = 0.25f;
 public const float FullAfter = 1.5f;
 public const float FadeDuration = 0.10f;
 public static float Fade(float age, float start) { float v=(age-start)/FadeDuration; return v<0?0:(v>1?1:v); }
 public static float Compact(float age) { return Fade(age,CompactAfter)*(1-Fade(age,FullAfter)); }
 public static float Full(float age) { return Fade(age,FullAfter); }
}
}
