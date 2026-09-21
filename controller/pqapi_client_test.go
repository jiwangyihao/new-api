package controller

import "testing"

func TestPQAPIHMAC(t *testing.T) {
	got := pqapiHMAC("secret", "1700000000\nnonce\n{\"amount\":3990}")
	want := "9ea50b93cbbdcc31e1bc859e072100278e34115f745c1c68d983e318c71a4957"
	if got != want {
		t.Fatalf("unexpected signature: got %s want %s", got, want)
	}
}

func TestPQAPICheckoutURLAllowed(t *testing.T) {
	if !pqapiCheckoutURLAllowed("https://shop.pqapi.shop", "https://shop.pqapi.shop/pay/token") {
		t.Fatal("expected same-origin HTTPS checkout to be accepted")
	}
	for _, candidate := range []string{"javascript:alert(1)", "http://shop.pqapi.shop/pay/token", "https://evil.example/pay/token"} {
		if pqapiCheckoutURLAllowed("https://shop.pqapi.shop", candidate) {
			t.Fatalf("expected %q to be rejected", candidate)
		}
	}
}
