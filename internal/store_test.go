package internal_test

import (
	"testing"

	"github.com/Muxcore-Media/media-books/internal"
)

func TestStoreAuthorBookRoundTrip(t *testing.T) {
	s := internal.NewStore()
	au, err := s.AddAuthor(internal.Author{Name: "Ursula K. Le Guin", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	bk, err := s.AddBook(internal.Book{AuthorID: au.ID, Title: "A Wizard of Earthsea", Year: 1968, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if bk.Title != "A Wizard of Earthsea" {
		t.Fatalf("%+v", bk)
	}
	if len(s.ListBooks(au.ID)) != 1 {
		t.Fatal("expected 1 book")
	}
	if err := s.RemoveAuthor(au.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.ListBooks("")) != 0 {
		t.Fatal("expected books cleared")
	}
}
