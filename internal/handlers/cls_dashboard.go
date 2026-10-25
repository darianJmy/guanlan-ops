package handlers

import (
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"guanlan/pkg/textutil"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	tencentCLSEndpoint = "https://cls.tencentcloudapi.com"
	tencentSTSEndpoint = "https://sts.tencentcloudapi.com"
	tencentCLSRoleTTL  = 300
)

type logDashboardResource struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group,omitempty"`
}

type tencentTemporaryCredentials struct {
	SecretID  string
	SecretKey string
	Token     string
}

type tencentCloudAPIError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

func fetchTencentCLSTopics(auth slsAuth, region string) ([]logDashboardResource, error) {
	const pageSize = 100
	resources := make([]logDashboardResource, 0)
	for offset := 0; ; offset += pageSize {
		payload, _ := json.Marshal(map[string]interface{}{"Offset": offset, "Limit": pageSize, "BizType": 0})
		body, err := callTencentCloudAPI(tencentCLSEndpoint, "cls", "2020-10-16", "DescribeTopics", region, auth.AccessKeyID, auth.AccessKeySecret, "", payload)
		if err != nil {
			return nil, err
		}
		var parsed struct {
			Response struct {
				Topics []struct {
					TopicID    string `json:"TopicId"`
					TopicName  string `json:"TopicName"`
					LogsetID   string `json:"LogsetId"`
					LogsetName string `json:"LogsetName"`
				} `json:"Topics"`
				TotalCount int                   `json:"TotalCount"`
				Error      *tencentCloudAPIError `json:"Error"`
			} `json:"Response"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("解析 CLS Topic 响应失败: %w", err)
		}
		if parsed.Response.Error != nil {
			return nil, fmt.Errorf("%s: %s", parsed.Response.Error.Code, parsed.Response.Error.Message)
		}
		for _, topic := range parsed.Response.Topics {
			if strings.TrimSpace(topic.TopicID) == "" {
				continue
			}
			group := strings.TrimSpace(topic.LogsetName)
			if group == "" {
				group = strings.TrimSpace(topic.LogsetID)
			}
			resources = append(resources, logDashboardResource{ID: topic.TopicID, Name: topic.TopicName, Group: group})
		}
		if len(parsed.Response.Topics) < pageSize || len(resources) >= parsed.Response.TotalCount {
			break
		}
	}
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Group == resources[j].Group {
			return resources[i].Name < resources[j].Name
		}
		return resources[i].Group < resources[j].Group
	})
	return resources, nil
}

func assumeTencentCLSRole(auth slsAuth, region, roleArn string) (tencentTemporaryCredentials, error) {
	payload, _ := json.Marshal(map[string]interface{}{
		"RoleArn":         strings.TrimSpace(roleArn),
		"RoleSessionName": "aiops-cls",
		"DurationSeconds": tencentCLSRoleTTL,
	})
	body, err := callTencentCloudAPI(tencentSTSEndpoint, "sts", "2018-08-13", "AssumeRole", region, auth.AccessKeyID, auth.AccessKeySecret, "", payload)
	if err != nil {
		return tencentTemporaryCredentials{}, err
	}
	var parsed struct {
		Response struct {
			Credentials struct {
				Token        string `json:"Token"`
				TmpSecretID  string `json:"TmpSecretId"`
				TmpSecretKey string `json:"TmpSecretKey"`
			} `json:"Credentials"`
			Error *tencentCloudAPIError `json:"Error"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return tencentTemporaryCredentials{}, fmt.Errorf("解析 STS 响应失败: %w", err)
	}
	if parsed.Response.Error != nil {
		return tencentTemporaryCredentials{}, fmt.Errorf("%s: %s", parsed.Response.Error.Code, parsed.Response.Error.Message)
	}
	credentials := tencentTemporaryCredentials{
		SecretID: parsed.Response.Credentials.TmpSecretID, SecretKey: parsed.Response.Credentials.TmpSecretKey, Token: parsed.Response.Credentials.Token,
	}
	if credentials.SecretID == "" || credentials.SecretKey == "" || credentials.Token == "" {
		return tencentTemporaryCredentials{}, fmt.Errorf("腾讯云 STS 未返回完整临时凭证")
	}
	return credentials, nil
}

func callTencentCloudAPI(endpoint, service, version, action, region, secretID, secretKey, token string, payload []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	signTencentCloudRequestAt(req, secretID, secretKey, service, version, action, region, token, string(payload), time.Now())
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求腾讯云 %s 失败: %w", action, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取腾讯云 %s 响应失败: %w", action, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("腾讯云 %s 返回 HTTP %d: %s", action, resp.StatusCode, textutil.Ellipsis(string(body), 500))
	}
	return body, nil
}

func signTencentCloudRequestAt(req *http.Request, secretID, secretKey, service, version, action, region, token, payload string, now time.Time) {
	host := req.URL.Host
	timestamp := now.Unix()
	date := now.UTC().Format("2006-01-02")
	canonicalHeaders := fmt.Sprintf("content-type:application/json\nhost:%s\n", host)
	signedHeaders := "content-type;host"
	canonicalRequest := fmt.Sprintf("POST\n/\n\n%s\n%s\n%s", canonicalHeaders, signedHeaders, sha256Hex(payload))
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	stringToSign := fmt.Sprintf("TC3-HMAC-SHA256\n%d\n%s\n%s", timestamp, credentialScope, sha256Hex(canonicalRequest))
	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := fmt.Sprintf("%x", hmacSHA256(secretSigning, stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", secretID, credentialScope, signedHeaders, signature))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	if strings.TrimSpace(region) != "" {
		req.Header.Set("X-TC-Region", strings.TrimSpace(region))
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("X-TC-Token", strings.TrimSpace(token))
	}
}

func buildTencentCLSRoleLoginURL(credentials tencentTemporaryCredentials, region, topicID string, now time.Time, nonce int64) string {
	targetQuery := url.Values{}
	targetQuery.Set("region", strings.TrimSpace(region))
	targetQuery.Set("topic_id", strings.TrimSpace(topicID))
	for _, key := range []string{"hideWidget", "hideTopNav", "hideLeftNav", "hideConfigMenu", "hideDashboardMenu", "hideAlertMenu", "hideCollectionMenu", "hideIndexMenu", "hideMoreMenu"} {
		targetQuery.Set(key, "true")
	}
	targetURL := "https://console.cloud.tencent.cn/cls/search?" + targetQuery.Encode()

	signatureParams := url.Values{}
	signatureParams.Set("action", "roleLogin")
	signatureParams.Set("nonce", strconv.FormatInt(nonce, 10))
	signatureParams.Set("secretId", credentials.SecretID)
	signatureParams.Set("timestamp", strconv.FormatInt(now.Unix(), 10))
	source := "GETcloud.tencent.cn/login/roleAccessCallback?" + signatureParams.Encode()
	mac := hmac.New(sha1.New, []byte(credentials.SecretKey))
	_, _ = mac.Write([]byte(source))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	query := url.Values{}
	query.Set("algorithm", "sha1")
	query.Set("action", "roleLogin")
	query.Set("nonce", strconv.FormatInt(nonce, 10))
	query.Set("secretId", credentials.SecretID)
	query.Set("timestamp", strconv.FormatInt(now.Unix(), 10))
	query.Set("token", credentials.Token)
	query.Set("signature", signature)
	query.Set("s_url", targetURL)
	return "https://cloud.tencent.cn/login/roleAccessCallback?" + query.Encode()
}

func secureLoginNonce() int64 {
	const minimum int64 = 10000
	const span int64 = 100000000 - minimum
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(span))
	if err != nil {
		return time.Now().UnixNano()%span + minimum
	}
	return value.Int64() + minimum
}
