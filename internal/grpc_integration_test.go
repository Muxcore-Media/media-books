package internal_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Muxcore-Media/media-books/internal"
	booksv1 "github.com/Muxcore-Media/media-books/proto/gen/muxcore/books/v1"
)

func startGRPCModule(t *testing.T) (*internal.Module, booksv1.BookManagementServiceClient) {
	t.Helper()
	data := t.TempDir()
	lib := filepath.Join(data, "library")
	if err := copyTree(filepath.Join("testdata", "library"), lib); err != nil {
		t.Fatal(err)
	}
	m := internal.NewModule(internal.Config{
		DataDir: data, LibraryDir: lib,
		GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
	})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })
	conn, err := grpc.NewClient(m.GRPCListenAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return m, booksv1.NewBookManagementServiceClient(conn)
}

func TestGRPCAuthorBookLifecycle(t *testing.T) {
	_, cli := startGRPCModule(t)
	ctx := context.Background()

	addAu, err := cli.AddAuthor(ctx, &booksv1.AddAuthorRequest{Name: "gRPC Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	getAu, err := cli.GetAuthor(ctx, &booksv1.GetAuthorRequest{Id: addAu.GetAuthor().GetId()})
	if err != nil || getAu.GetAuthor().GetName() != "gRPC Author" {
		t.Fatalf("get author: %+v err=%v", getAu, err)
	}
	mon := false
	_, err = cli.UpdateAuthor(ctx, &booksv1.UpdateAuthorRequest{Id: addAu.GetAuthor().GetId(), Monitored: &mon})
	if err != nil {
		t.Fatal(err)
	}

	addBk, err := cli.AddBook(ctx, &booksv1.AddBookRequest{
		AuthorId: addAu.GetAuthor().GetId(), Title: "gRPC Book", Monitored: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	getBk, err := cli.GetBook(ctx, &booksv1.GetBookRequest{Id: addBk.GetBook().GetId()})
	if err != nil || getBk.GetBook().GetTitle() != "gRPC Book" {
		t.Fatalf("get book: %+v err=%v", getBk, err)
	}
	title := "Updated Book"
	_, err = cli.UpdateBook(ctx, &booksv1.UpdateBookRequest{Id: addBk.GetBook().GetId(), Title: &title})
	if err != nil {
		t.Fatal(err)
	}

	listAu, err := cli.ListAuthors(ctx, &booksv1.ListAuthorsRequest{})
	if err != nil || len(listAu.GetAuthors()) < 1 {
		t.Fatalf("list authors: %+v err=%v", listAu, err)
	}
	listBk, err := cli.ListBooks(ctx, &booksv1.ListBooksRequest{AuthorId: addAu.GetAuthor().GetId()})
	if err != nil || len(listBk.GetBooks()) < 1 {
		t.Fatalf("list books: %+v err=%v", listBk, err)
	}

	if _, err := cli.RemoveBook(ctx, &booksv1.RemoveBookRequest{Id: addBk.GetBook().GetId()}); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.RemoveAuthor(ctx, &booksv1.RemoveAuthorRequest{Id: addAu.GetAuthor().GetId()}); err != nil {
		t.Fatal(err)
	}
}

func TestGRPCScanListMissingImport(t *testing.T) {
	m, cli := startGRPCModule(t)
	ctx := context.Background()

	scan, err := cli.ScanLibrary(ctx, &booksv1.ScanLibraryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if scan.GetFilesFound() < 2 {
		t.Fatalf("scan: %+v", scan)
	}

	missing, err := cli.ListMissing(ctx, &booksv1.ListMissingRequest{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	_ = missing

	listBk, err := cli.ListBooks(ctx, &booksv1.ListBooksRequest{})
	if err != nil || len(listBk.GetBooks()) == 0 {
		t.Fatal("expected scanned books")
	}
	bk := listBk.GetBooks()[0]
	if len(bk.GetFiles()) < 1 {
		t.Fatal("expected files on list books")
	}

	files, err := cli.ListBookFiles(ctx, &booksv1.ListBookFilesRequest{BookId: bk.GetId()})
	if err != nil || len(files.GetFiles()) < 1 {
		t.Fatalf("list files: %+v err=%v", files, err)
	}

	stub := filepath.Join(m.LibraryDir(), "Fixture Author", "Sample Book", "03-import.epub")
	if err := os.WriteFile(stub, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	imp, err := cli.ImportBookFile(ctx, &booksv1.ImportBookFileRequest{
		BookId: bk.GetId(), Path: stub,
	})
	if err != nil {
		t.Fatal(err)
	}
	if imp.GetFile().GetId() == "" {
		t.Fatal("expected imported file id")
	}
}

func TestGRPCRemoveAuthorDeleteFiles(t *testing.T) {
	data := t.TempDir()
	lib := filepath.Join(data, "library")
	bookDir := filepath.Join(lib, "Del Author", "Del Book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(bookDir, "book.epub")
	if err := os.WriteFile(stub, []byte("ebook"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := internal.NewModule(internal.Config{
		DataDir: data, LibraryDir: lib,
		GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
	})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ScanLibrary(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })

	conn, err := grpc.NewClient(m.GRPCListenAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	cli := booksv1.NewBookManagementServiceClient(conn)
	ctx := context.Background()

	listAu, err := cli.ListAuthors(ctx, &booksv1.ListAuthorsRequest{Query: "Del Author"})
	if err != nil || len(listAu.GetAuthors()) != 1 {
		t.Fatalf("authors: %+v err=%v", listAu, err)
	}
	if _, err := cli.RemoveAuthor(ctx, &booksv1.RemoveAuthorRequest{
		Id: listAu.GetAuthors()[0].GetId(), DeleteFiles: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stub); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, stat err=%v", err)
	}

	outside := filepath.Join(data, "outside.epub")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	au, err := cli.AddAuthor(ctx, &booksv1.AddAuthorRequest{Name: "Keep Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	bk, err := cli.AddBook(ctx, &booksv1.AddBookRequest{
		AuthorId: au.GetAuthor().GetId(), Title: "Keep Book", Monitored: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cli.ImportBookFile(ctx, &booksv1.ImportBookFileRequest{
		BookId: bk.GetBook().GetId(), Path: outside,
	}); err == nil {
		t.Fatal("expected import outside library to fail")
	}
}
