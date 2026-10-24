package com.hyjs.compat;
public final class DiagnosticProbe {
 public static void main(String[] ignored) {
  Throwable cause=new java.io.IOException("SECRET_CREDENTIAL /private/account/file");
  cause.setStackTrace(new StackTraceElement[]{new StackTraceElement("fixture.Safe","call","/private/source",19)});
  String log=FailureDiagnostic.format("prepare",new ExceptionInInitializerError(cause));
  if(log.contains("SECRET") || log.contains("/private") || !log.contains("java.io.IOException") || !log.contains("ExceptionInInitializerError") || !log.contains("fixture.Safe.call line=19"))throw new AssertionError(log);
  System.out.println("PASS safe cause types and stack class/method/line; messages/paths/credentials absent");
 }
}
