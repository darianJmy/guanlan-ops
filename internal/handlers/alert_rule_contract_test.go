package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAlertRuleRequiresGroupSelection(t *testing.T) {
	operationTestDB(t)
	router := gin.New()
	router.POST("/rules", CreateAlertRule)
	for _, tc := range []struct {
		name   string
		groups bool
		direct bool
		want   int
	}{
		{"no groups", false, false, http.StatusBadRequest},
		{"direct only", false, true, http.StatusBadRequest},
		{"mixed fields", true, true, http.StatusBadRequest},
		{"groups", true, false, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := cpuRuleArgs()
			if !tc.groups {
				delete(args, "datasource_group_ids")
			}
			if tc.direct {
				args["datasource_ids"] = []uint{1}
			}
			body, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/rules", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestAlertRuleConfigRejectsDirectSelection(t *testing.T) {
	_, err := parseAlertRuleConfigDocument("apiVersion: aiops/v1\nkind: AlertRuleList\nrules:\n  - name: CPU\n    datasource_ids: [1]\n")
	if err == nil {
		t.Fatal("direct datasource selection accepted")
	}
}
