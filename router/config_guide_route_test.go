package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func TestConfigGuideRouteReturnsJSONBeforeWebFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetConfigGuideRouter(engine)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config-guides/opencode-openai/manifest.json", nil)
	req.Host = "gateway.example.com"
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected route status 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("expected JSON content-type, got %q", contentType)
	}
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("route should return JSON error, got %q: %v", recorder.Body.String(), err)
	}
	if response.Success || response.Message != "api_key is required" {
		t.Fatalf("unexpected route response: %#v", response)
	}
}

func TestOMPConfigGuidePluginAndImageGeneratorRoutesAreNotRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetConfigGuideRouter(engine)

	for _, path := range []string{"/config-guides/omp-openai/plugin.txt", "/config-guides/omp-openai/image-generator.md"} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path+"?api_key=sk-livetoken", nil)
		engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected %s to be unregistered with 404, got %d: %s", path, recorder.Code, recorder.Body.String())
		}
	}
}
