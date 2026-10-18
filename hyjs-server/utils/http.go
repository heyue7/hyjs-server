package utils

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"hyjs-server/constUtil"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	resty "resty.dev/v3"
)

type HttpClient struct {
	client  *resty.Client
	headers map[string][]string
	fields  map[string][]string
	lb      *LB
	lbKey   string
}

func NewHttp() *HttpClient {
	client := resty.New()
	client.SetTimeout(30 * time.Second)

	return &HttpClient{
		client:  client,
		headers: make(map[string][]string),
		fields:  make(map[string][]string),
	}
}

func (hc *HttpClient) WithLB(lb *LB) *HttpClient {
	hc.lb = lb
	if hc.lb != nil {
		hc.client.SetLoadBalancer(hc.lb.LoadBalancer)
	}
	return hc
}

func (hc *HttpClient) WithLBKey(key string) *HttpClient {
	hc.lbKey = key
	return hc
}

func (hc *HttpClient) WithLBKeyUserId(c *gin.Context) *HttpClient {
	userID := c.GetInt64(constUtil.CtxUid)
	if userID != 0 {
		hc.lbKey = fmt.Sprintf("%d", userID)
	}
	return hc
}

func (hc *HttpClient) SetTimeout(timeout time.Duration) *HttpClient {
	hc.client.SetTimeout(timeout)
	return hc
}

func (hc *HttpClient) SetHeader(headers map[string][]string) *HttpClient {
	hc.headers = headers
	return hc
}

func (hc *HttpClient) AddHeader(key, value string) *HttpClient {
	hc.headers[key] = append(hc.headers[key], value)
	return hc
}

func (hc *HttpClient) SetField(fields map[string][]string) *HttpClient {
	hc.fields = fields
	return hc
}

func (hc *HttpClient) AddField(key, value string) *HttpClient {
	hc.fields[key] = append(hc.fields[key], value)
	return hc
}

func (hc *HttpClient) doRequest(method, rawURL string, body interface{}) (int, []byte, error) {
	return hc.doRequestWithContext(context.Background(), method, rawURL, body)
}

// doRequestWithContext 沿调用方生命周期发送请求，取消会终止连接等待。
// 共用原头部和负载规则，保持既有内部服务适配行为。
func (hc *HttpClient) doRequestWithContext(ctx context.Context, method, rawURL string, body interface{}) (int, []byte, error) {
	reqClient := hc.clientForRequest()
	req := reqClient.R().SetContext(ctx)
	hc.applyHeaders(req)

	if body != nil {
		req.SetBody(body)
		if !hc.hasHeader("Content-Type") {
			req.SetHeader("Content-Type", "application/json")
		}
	}

	resp, err := req.Execute(method, rawURL)
	if err != nil {
		if resp != nil {
			return resp.StatusCode(), resp.Bytes(), err
		}
		return 0, []byte{}, err
	}

	return resp.StatusCode(), resp.Bytes(), nil
}

func (hc *HttpClient) Get(rawURL string, query map[string]string) (int, []byte, error) {
	if len(query) == 0 {
		return hc.doRequest(http.MethodGet, rawURL, nil)
	}

	reqClient := hc.clientForRequest()
	req := reqClient.R()
	hc.applyHeaders(req)
	req.SetQueryParams(query)

	resp, err := req.Get(rawURL)
	if err != nil {
		if resp != nil {
			return resp.StatusCode(), resp.Bytes(), err
		}
		return 0, []byte{}, err
	}

	return resp.StatusCode(), resp.Bytes(), nil
}

func (hc *HttpClient) Post(rawURL string, body interface{}) (int, []byte, error) {
	return hc.doRequest(http.MethodPost, rawURL, body)
}

// PostWithContext 使内部跨服重试随请求取消或服务退出立即结束。
func (hc *HttpClient) PostWithContext(ctx context.Context, rawURL string, body interface{}) (int, []byte, error) {
	return hc.doRequestWithContext(ctx, http.MethodPost, rawURL, body)
}

func (hc *HttpClient) Put(rawURL string, body interface{}) (int, []byte, error) {
	return hc.doRequest(http.MethodPut, rawURL, body)
}

func (hc *HttpClient) Delete(rawURL string, body interface{}) (int, []byte, error) {
	return hc.doRequest(http.MethodDelete, rawURL, body)
}

func (hc *HttpClient) PostForm(rawURL string, data url.Values) (int, []byte, error) {
	reqClient := hc.clientForRequest()
	req := reqClient.R()
	hc.applyHeaders(req)

	if data != nil {
		req.SetFormDataFromValues(data)
	}

	if !hc.hasHeader("Content-Type") {
		req.SetHeader("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := req.Post(rawURL)
	if err != nil {
		if resp != nil {
			return resp.StatusCode(), resp.Bytes(), err
		}
		return 0, []byte{}, err
	}

	return resp.StatusCode(), resp.Bytes(), nil
}

func (hc *HttpClient) UploadFile(rawURL string, fieldName, filePath string) (int, []byte, error) {
	reqClient := hc.clientForRequest()
	req := reqClient.R()
	hc.applyHeaders(req)

	req.SetFile(fieldName, filePath)

	resp, err := req.Post(rawURL)
	if err != nil {
		if resp != nil {
			return resp.StatusCode(), resp.Bytes(), err
		}
		return 0, []byte{}, err
	}

	return resp.StatusCode(), resp.Bytes(), nil
}

func (hc *HttpClient) DownloadFile(rawURL string) (string, error) {
	reqClient := hc.clientForRequest()
	resp, err := reqClient.R().Get(rawURL)
	if err != nil {
		return "", err
	}
	return resp.String(), nil
}

func (hc *HttpClient) applyHeaders(req *resty.Request) {
	if len(hc.headers) == 0 {
		return
	}
	req.SetHeaderMultiValues(hc.headers)
}

func (hc *HttpClient) hasHeader(key string) bool {
	for k := range hc.headers {
		if strings.EqualFold(k, key) {
			return true
		}
	}
	return false
}

func (hc *HttpClient) clientForRequest() *resty.Client {
	key := hc.consumeLBKey()
	if hc.lb == nil || key == "" {
		return hc.client
	}

	reqClient := hc.client.Clone(context.TODO())
	lb := hc.lb.BuildLoadBalancerForKey(key)
	if lb != nil {
		reqClient.SetLoadBalancer(lb)
	}
	return reqClient
}

func (hc *HttpClient) consumeLBKey() string {
	if hc.lbKey == "" {
		return ""
	}
	key := hc.lbKey
	hc.lbKey = ""
	return key
}

func (hc *HttpClient) PostStreamWithContext(ctx context.Context, rawURL string, body interface{}, dataChan chan<- map[string]interface{}) error {
	reqClient := hc.clientForRequest()
	req := reqClient.R().SetContext(ctx).SetResponseDoNotParse(true)
	hc.applyHeaders(req)

	if body != nil {
		req.SetBody(body)
		if !hc.hasHeader("Content-Type") {
			req.SetHeader("Content-Type", "application/json")
		}
	}

	resp, err := req.Post(rawURL)
	if err != nil {
		return fmt.Errorf("send request failed: %w", err)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode() != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode(), string(respBytes))
	}

	contentType := resp.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected content-type: %s, response: %s", contentType, string(respBytes))
	}

	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- hc.readSSEStream(ctx, resp.Body, dataChan)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (hc *HttpClient) readSSEStream(ctx context.Context, body io.Reader, dataChan chan<- map[string]interface{}) error {
	reader := bufio.NewReaderSize(body, 64*1024)
	var currentData strings.Builder
	var inDataBlock bool

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if inDataBlock && currentData.Len() > 0 {
					hc.processSSEData(currentData.String(), dataChan)
				}
				return nil
			}
			return fmt.Errorf("read line failed: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if inDataBlock && currentData.Len() > 0 {
				hc.processSSEData(currentData.String(), dataChan)
				currentData.Reset()
				inDataBlock = false
			}
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")
			if currentData.Len() > 0 {
				currentData.WriteString("\n")
			}
			currentData.WriteString(dataStr)
			inDataBlock = true
		}
	}
}

func (hc *HttpClient) processSSEData(dataStr string, dataChan chan<- map[string]interface{}) error {
	if dataStr == "" {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
		return fmt.Errorf("unmarshal JSON failed: %w", err)
	}

	select {
	case dataChan <- data:
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("send data to channel timeout")
	}
}
