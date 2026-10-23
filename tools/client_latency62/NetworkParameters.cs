using System.Net;
namespace HYJS.Latency62 {
public static class NetworkParameters {
 public static HttpWebRequest Configure(HttpWebRequest request) {
  request.KeepAlive=true;
  request.ServicePoint.Expect100Continue=false;
  request.ServicePoint.UseNagleAlgorithm=false;
  return request;
 }
}
}
