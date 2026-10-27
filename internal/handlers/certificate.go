package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/certutil"
	"guanlan/pkg/resp"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type AliyunCertificateUpdateReq struct {
	AccessKeyID     string   `json:"access_key_id" binding:"required"`
	AccessKeySecret string   `json:"access_key_secret" binding:"required"`
	SecurityToken   string   `json:"security_token"`
	Product         string   `json:"product" binding:"required"`
	Domains         []string `json:"domains" binding:"required"`
	CertName        string   `json:"cert_name" binding:"required"`
	Certificate     string   `json:"certificate" binding:"required"`
	PrivateKey      string   `json:"private_key" binding:"required"`
}

type aliyunCertProduct struct {
	Action  string
	Version string
	Host    string
}

var aliyunCertProducts = map[string]aliyunCertProduct{
	"cdn": {
		Action:  "SetCdnDomainSSLCertificate",
		Version: "2018-05-10",
		Host:    "cdn.aliyuncs.com",
	},
	"dcdn": {
		Action:  "SetDcdnDomainSSLCertificate",
		Version: "2018-01-15",
		Host:    "dcdn.aliyuncs.com",
	},
}

type certificateUpdateResult struct {
	Domain    string `json:"domain"`
	Status    string `json:"status"`
	RequestID string `json:"request_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

func AliyunUpdateCertificate(c *gin.Context) {
	var req AliyunCertificateUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误: "+err.Error())
		return
	}

	productKey := strings.ToLower(strings.TrimSpace(req.Product))
	product, ok := aliyunCertProducts[productKey]
	if !ok {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "暂只支持 CDN 和 DCDN")
		return
	}

	domains := certutil.NormalizeDomains(req.Domains)
	if len(domains) == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请至少填写一个域名")
		return
	}

	results := make([]certificateUpdateResult, 0, len(domains))
	successCount := 0
	for _, domain := range domains {
		requestID, err := callAliyunSetCertificate(product, req, domain)
		status := "success"
		errMsg := ""
		if err != nil {
			status = "failed"
			errMsg = err.Error()
		} else {
			successCount++
		}

		record := models.CertificateRotationRecord{
			Provider:  "aliyun",
			Product:   productKey,
			Domain:    domain,
			CertName:  req.CertName,
			Status:    status,
			RequestID: requestID,
			Error:     errMsg,
		}
		database.DB.Create(&record)

		results = append(results, certificateUpdateResult{
			Domain:    domain,
			Status:    status,
			RequestID: requestID,
			Error:     errMsg,
		})
	}

	resp.Success(c, http.StatusOK, fmt.Sprintf("完成 %d/%d", successCount, len(results)), gin.H{
		"success": successCount,
		"total":   len(results),
		"results": results,
	})
}

func ListCertificateRotationRecords(c *gin.Context) {
	var records []models.CertificateRotationRecord
	database.DB.Order("id DESC").Limit(100).Find(&records)
	resp.Success(c, http.StatusOK, "成功", records)
}

func callAliyunSetCertificate(product aliyunCertProduct, req AliyunCertificateUpdateReq, domain string) (string, error) {
	params := map[string]string{
		"Action":           product.Action,
		"Version":          product.Version,
		"Format":           "JSON",
		"AccessKeyId":      strings.TrimSpace(req.AccessKeyID),
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   nonce(),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"DomainName":       domain,
		"CertName":         strings.TrimSpace(req.CertName),
		"CertType":         "upload",
		"SSLProtocol":      "on",
		"SSLPub":           strings.TrimSpace(req.Certificate),
		"SSLPri":           strings.TrimSpace(req.PrivateKey),
		"ForceSet":         "1",
	}
	if strings.TrimSpace(req.SecurityToken) != "" {
		params["SecurityToken"] = strings.TrimSpace(req.SecurityToken)
	}
	params["Signature"] = aliyunSign("POST", params, req.AccessKeySecret)

	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}

	httpReq, err := http.NewRequest(http.MethodPost, "https://"+product.Host+"/", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("请求阿里云失败: %s", err.Error())
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var parsed struct {
		RequestID string `json:"RequestId"`
		Code      string `json:"Code"`
		Message   string `json:"Message"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("解析阿里云响应失败: %s", string(body))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || parsed.Code != "" {
		msg := parsed.Message
		if msg == "" {
			msg = string(body)
		}
		if parsed.Code != "" {
			msg = parsed.Code + ": " + msg
		}
		return parsed.RequestID, fmt.Errorf("%s", msg)
	}
	return parsed.RequestID, nil
}

func aliyunSign(method string, params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, percentEncode(key)+"="+percentEncode(params[key]))
	}
	canonicalizedQuery := strings.Join(parts, "&")
	stringToSign := method + "&" + percentEncode("/") + "&" + percentEncode(canonicalizedQuery)

	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func percentEncode(value string) string {
	encoded := url.QueryEscape(value)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func nonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
