package validate

import "testing"

func TestTicketSaleURL(t *testing.T) {
	t.Parallel()
	got, err := TicketSaleURL("https://www.flamengo.com.br/noticias/futebol")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://www.flamengo.com.br/noticias/futebol" {
		t.Fatalf("got %q", got)
	}
}

func TestTicketSaleURLRejectsNonHTTP(t *testing.T) {
	t.Parallel()
	_, err := TicketSaleURL("ftp://example.com/x")
	if err == nil {
		t.Fatal("want error")
	}
}
