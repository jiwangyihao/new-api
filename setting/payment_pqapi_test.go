package setting

import "testing"

func TestNormalizePQAPIBaseURL(t *testing.T) {
	got, err := NormalizePQAPIBaseURL("https://shop.pqapi.shop/")
	if err != nil || got != "https://shop.pqapi.shop" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, value := range []string{"http://shop.pqapi.shop", "https://user:pass@shop.pqapi.shop", "https://shop.pqapi.shop?secret=x"} {
		if _, err := NormalizePQAPIBaseURL(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}
