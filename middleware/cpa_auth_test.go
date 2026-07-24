package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHasExternalCPAManagementKeyIgnoresPanelPlaceholder(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name   string
		header string
		value  string
		want   bool
	}{
		{name: "placeholder x-management-key", header: "X-Management-Key", value: cpaPanelManagementKeyPlaceholder, want: false},
		{name: "placeholder bearer", header: "Authorization", value: "Bearer " + cpaPanelManagementKeyPlaceholder, want: false},
		{name: "real x-management-key", header: "X-Management-Key", value: "real-secret", want: true},
		{name: "real bearer", header: "Authorization", value: "Bearer real-secret", want: true},
		{name: "empty", header: "", value: "", want: false},
		{name: "empty bearer", header: "Authorization", value: "Bearer ", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(http.MethodGet, "/v0/management/config", nil)
			if tc.header != "" {
				req.Header.Set(tc.header, tc.value)
			}
			c.Request = req

			if got := hasExternalCPAManagementKey(c); got != tc.want {
				t.Fatalf("hasExternalCPAManagementKey() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsCPAPanelManagementKeyPlaceholder(t *testing.T) {
	if !isCPAPanelManagementKeyPlaceholder("gateway-managed") {
		t.Fatal("expected gateway-managed to be treated as placeholder")
	}
	if !isCPAPanelManagementKeyPlaceholder(" Gateway-Managed ") {
		t.Fatal("expected case/space-insensitive placeholder match")
	}
	if isCPAPanelManagementKeyPlaceholder("real-secret") {
		t.Fatal("real secret must not be treated as placeholder")
	}
}
