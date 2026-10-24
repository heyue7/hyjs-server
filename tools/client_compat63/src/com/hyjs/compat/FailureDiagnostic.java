package com.hyjs.compat;

/** Fixed metadata only: never serialize Throwable messages, paths or object values. */
public final class FailureDiagnostic {
    private FailureDiagnostic() { }
    public static String format(String phase, Throwable failure) {
        StringBuilder result = new StringBuilder("startup ").append(phase).append(" failed");
        Throwable current = failure;
        for (int depth = 0; current != null && depth < 4; depth++) {
            result.append("\nexception[").append(depth).append("]=").append(current.getClass().getName());
            StackTraceElement[] frames = current.getStackTrace();
            for (int i = 0; i < frames.length && i < 16; i++) {
                StackTraceElement frame = frames[i];
                result.append("\n at ").append(frame.getClassName()).append('.').append(frame.getMethodName())
                    .append(" line=").append(frame.getLineNumber());
            }
            Throwable next = current.getCause();
            if (next == current) break;
            current = next;
        }
        return result.length() > 8192 ? result.substring(0, 8192) : result.toString();
    }
}
