package resp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestResponseContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		write  func(*gin.Context)
		status int
		body   string
	}{
		{"success without data", func(c *gin.Context) { Success(c, http.StatusOK, "成功") }, 200, `{"code":0,"message":"成功"}`},
		{"explicit null", func(c *gin.Context) { Success(c, http.StatusCreated, "", nil) }, 201, `{"code":0,"message":"","data":null}`},
		{"empty list", func(c *gin.Context) { Success(c, http.StatusOK, "成功", []string{}) }, 200, `{"code":0,"message":"成功","data":[]}`},
		{"zero data", func(c *gin.Context) { Success(c, http.StatusOK, "成功", 0) }, 200, `{"code":0,"message":"成功","data":0}`},
		{"legacy business error", func(c *gin.Context) { Error(c, http.StatusOK, CodeFailure, "连接失败") }, 200, `{"code":1,"message":"连接失败"}`},
		{"unauthorized", func(c *gin.Context) { Error(c, http.StatusUnauthorized, CodeUnauthorized, "未登录") }, 401, `{"code":401,"message":"未登录"}`},
		{"error data", func(c *gin.Context) {
			Error(c, http.StatusBadRequest, CodeBadRequest, "参数错误", gin.H{"field": "name"})
		}, 400, `{"code":400,"message":"参数错误","data":{"field":"name"}}`},
		{"no message", func(c *gin.Context) { SuccessData(c, http.StatusOK, nil) }, 200, `{"code":0,"data":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			tc.write(c)
			if w.Code != tc.status {
				t.Fatalf("HTTP status = %d, want %d", w.Code, tc.status)
			}
			if c.IsAborted() {
				t.Fatal("response must not change middleware control flow")
			}
			if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("unexpected content type: %s", w.Header().Get("Content-Type"))
			}
			var got, want any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.body), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("body = %s, want %s", w.Body.String(), tc.body)
			}
		})
	}
}
