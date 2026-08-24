package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	booksv1 "github.com/Muxcore-Media/media-books/proto/gen/muxcore/books/v1"
)

type Module struct {
	lis        net.Listener
	store      *Store
	grpcSrv    *grpc.Server
	httpSrv    *http.Server
	id         string
	grpcAddr   string
	httpAddr   string
	dataDir    string
	libraryDir string
	cfgMu      sync.RWMutex
}

type Config struct {
	ID         string
	DataDir    string
	LibraryDir string
	GRPCAddr   string
	HTTPAddr   string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "media-books"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9650"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9651"
	}
	if v := os.Getenv("BOOKS_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("BOOKS_LIBRARY_DIR"); v != "" {
		cfg.LibraryDir = v
	}
	if v := os.Getenv("MUXCORE_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.LibraryDir == "" {
		cfg.LibraryDir = filepath.Join(cfg.DataDir, "books")
	}
	return &Module{
		id: cfg.ID, grpcAddr: cfg.GRPCAddr, httpAddr: cfg.HTTPAddr,
		dataDir: cfg.DataDir, libraryDir: cfg.LibraryDir,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID: m.id, Name: "Book Manager", Version: "0.2.0",
		Roles:        []string{"media", "books"},
		Description:  "Readarr-class book library manager with SQLite persistence",
		Capabilities: []string{"media.books", "books", "settings"},
		HTTPAddr:     m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	if err := os.MkdirAll(m.dataDir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(m.libraryDir, 0o700); err != nil {
		return fmt.Errorf("create library dir: %w", err)
	}
	dbPath := filepath.Join(m.dataDir, "books.db")
	store, err := OpenStore(ctx, dbPath)
	if err != nil {
		return err
	}
	m.store = store
	slog.Info("book library store open", "db", dbPath, "library", m.libraryDir)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	if m.store == nil {
		return fmt.Errorf("store not initialized")
	}
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC %s: %w", m.grpcAddr, err)
	}
	m.lis = lis
	m.grpcAddr = lis.Addr().String()
	m.grpcSrv = grpc.NewServer()
	booksv1.RegisterBookManagementServiceServer(m.grpcSrv, &bookServer{m: m})
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)
	go func() {
		slog.Info("books gRPC listening", "addr", m.grpcAddr)
		if serveErr := m.grpcSrv.Serve(lis); serveErr != nil {
			slog.Error("gRPC serve", "error", serveErr)
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	m.registerBooksHTTPAPI(mux)
	httpLis, err := lc.Listen(ctx, "tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen HTTP %s: %w", m.httpAddr, err)
	}
	m.httpAddr = httpLis.Addr().String()
	m.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		slog.Info("health listening", "addr", m.httpAddr)
		if serveErr := m.httpSrv.Serve(httpLis); serveErr != nil && serveErr != http.ErrServerClosed {
			slog.Error("health serve", "error", serveErr)
		}
	}()
	return nil
}

// GRPCListenAddr returns the bound gRPC address after Start.
func (m *Module) GRPCListenAddr() string { return m.grpcAddr }

// HTTPListenAddr returns the bound health/HTTP API address after Start.
func (m *Module) HTTPListenAddr() string { return m.httpAddr }

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	if m.httpSrv != nil {
		_ = m.httpSrv.Shutdown(ctx)
	}
	if m.store != nil {
		_ = m.store.Close()
		m.store = nil
	}
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	if m.store == nil {
		return fmt.Errorf("store not open")
	}
	return nil
}

// ScanLibrary scans the configured library root into SQLite.
func (m *Module) ScanLibrary(ctx context.Context) (*ScanResult, error) {
	m.cfgMu.RLock()
	root := m.libraryDir
	store := m.store
	m.cfgMu.RUnlock()
	if store == nil {
		return nil, fmt.Errorf("store not open")
	}
	return store.ScanLibraryRoot(ctx, root)
}

type bookServer struct {
	booksv1.UnimplementedBookManagementServiceServer
	m *Module
}

func (s *bookServer) AddAuthor(ctx context.Context, req *booksv1.AddAuthorRequest) (*booksv1.AddAuthorResponse, error) {
	a, err := s.m.store.AddAuthor(ctx, Author{
		Name: req.GetName(), GoodreadsID: req.GetGoodreadsId(),
		Monitored: req.GetMonitored(), Path: req.GetPath(),
	})
	if err != nil {
		return nil, err
	}
	return &booksv1.AddAuthorResponse{Author: toPBAuthor(a)}, nil
}

func (s *bookServer) GetAuthor(ctx context.Context, req *booksv1.GetAuthorRequest) (*booksv1.GetAuthorResponse, error) {
	a, err := s.m.store.GetAuthor(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &booksv1.GetAuthorResponse{Author: toPBAuthor(a)}, nil
}

func (s *bookServer) ListAuthors(ctx context.Context, req *booksv1.ListAuthorsRequest) (*booksv1.ListAuthorsResponse, error) {
	items, err := s.m.store.ListAuthors(ctx, req.GetQuery())
	if err != nil {
		return nil, err
	}
	out := make([]*booksv1.Author, 0, len(items))
	for _, a := range items {
		out = append(out, toPBAuthor(a))
	}
	return &booksv1.ListAuthorsResponse{Authors: out}, nil
}

func (s *bookServer) RemoveAuthor(ctx context.Context, req *booksv1.RemoveAuthorRequest) (*booksv1.RemoveAuthorResponse, error) {
	if err := s.m.store.RemoveAuthor(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &booksv1.RemoveAuthorResponse{Success: true}, nil
}

func (s *bookServer) AddBook(ctx context.Context, req *booksv1.AddBookRequest) (*booksv1.AddBookResponse, error) {
	b, err := s.m.store.AddBook(ctx, Book{
		AuthorID: req.GetAuthorId(), Title: req.GetTitle(),
		ISBN: req.GetIsbn(), Year: req.GetYear(), Monitored: req.GetMonitored(),
	})
	if err != nil {
		return nil, err
	}
	return &booksv1.AddBookResponse{Book: toPBBook(b)}, nil
}

func (s *bookServer) ListBooks(ctx context.Context, req *booksv1.ListBooksRequest) (*booksv1.ListBooksResponse, error) {
	items, err := s.m.store.ListBooks(ctx, req.GetAuthorId())
	if err != nil {
		return nil, err
	}
	out := make([]*booksv1.Book, 0, len(items))
	for _, b := range items {
		out = append(out, toPBBook(b))
	}
	return &booksv1.ListBooksResponse{Books: out}, nil
}

func toPBAuthor(a *Author) *booksv1.Author {
	return &booksv1.Author{
		Id: a.ID, Name: a.Name, GoodreadsId: a.GoodreadsID,
		Monitored: a.Monitored, Path: a.Path,
	}
}

func toPBBook(b *Book) *booksv1.Book {
	return &booksv1.Book{
		Id: b.ID, AuthorId: b.AuthorID, Title: b.Title,
		Isbn: b.ISBN, Year: b.Year, Monitored: b.Monitored,
	}
}
