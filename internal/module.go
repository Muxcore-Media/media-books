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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mediaadminv1 "github.com/Muxcore-Media/contracts-media-admin/gen/muxcore/media/admin/v1"
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
	imageDir   string
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
		cfg.LibraryDir = cfg.DataDir
	}
	imageDir := os.Getenv("BOOKS_IMAGE_DIR")
	if imageDir == "" {
		imageDir = filepath.Join(cfg.DataDir, "images")
	}
	return &Module{
		id: cfg.ID, grpcAddr: cfg.GRPCAddr, httpAddr: cfg.HTTPAddr,
		dataDir: cfg.DataDir, libraryDir: cfg.LibraryDir, imageDir: imageDir,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID: m.id, Name: "Book Manager", Version: "0.3.0",
		Roles:        []string{"media", "books"},
		Description:  "Readarr-class book library manager with SQLite persistence",
		Capabilities: []string{"media.books", "books", "settings"},
		Contracts: []contracts.ContractDeclaration{
			{Repo: "github.com/Muxcore-Media/contracts-media-admin", Interface: "MediaAdminService", Version: "v0.1.0"},
		},
		HTTPAddr: m.httpAddr,
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
	if _, err := m.ScanLibrary(ctx); err != nil {
		return fmt.Errorf("startup library scan: %w", err)
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
	mediaadminv1.RegisterMediaAdminServiceServer(m.grpcSrv, mediaAdminServer{m: m})
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
	mux.HandleFunc("/images/", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/images/", http.FileServer(http.Dir(m.getImageDir()))).ServeHTTP(w, r)
	})
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

func (m *Module) libraryRoot() string {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return m.libraryDir
}

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
	return m.store.Ping(ctx)
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

func (s *bookServer) UpdateAuthor(ctx context.Context, req *booksv1.UpdateAuthorRequest) (*booksv1.UpdateAuthorResponse, error) {
	fields := map[string]any{}
	if req.Name != nil {
		fields["name"] = req.GetName()
	}
	if req.GoodreadsId != nil {
		fields["goodreads_id"] = req.GetGoodreadsId()
	}
	if req.Monitored != nil {
		fields["monitored"] = req.GetMonitored()
	}
	if req.Path != nil {
		fields["path"] = req.GetPath()
	}
	a, err := s.m.store.UpdateAuthor(ctx, req.GetId(), fields)
	if err != nil {
		return nil, err
	}
	return &booksv1.UpdateAuthorResponse{Author: toPBAuthor(a)}, nil
}

func (s *bookServer) RemoveAuthor(ctx context.Context, req *booksv1.RemoveAuthorRequest) (*booksv1.RemoveAuthorResponse, error) {
	if err := s.m.store.RemoveAuthorFiles(ctx, req.GetId(), s.m.libraryRoot(), req.GetDeleteFiles()); err != nil {
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

func (s *bookServer) GetBook(ctx context.Context, req *booksv1.GetBookRequest) (*booksv1.GetBookResponse, error) {
	b, err := s.m.store.GetBook(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &booksv1.GetBookResponse{Book: s.m.bookWithFiles(ctx, b)}, nil
}

func (s *bookServer) ListBooks(ctx context.Context, req *booksv1.ListBooksRequest) (*booksv1.ListBooksResponse, error) {
	items, err := s.m.store.ListBooks(ctx, req.GetAuthorId())
	if err != nil {
		return nil, err
	}
	out := make([]*booksv1.Book, 0, len(items))
	for _, b := range items {
		out = append(out, s.m.bookWithFiles(ctx, b))
	}
	return &booksv1.ListBooksResponse{Books: out}, nil
}

func (s *bookServer) UpdateBook(ctx context.Context, req *booksv1.UpdateBookRequest) (*booksv1.UpdateBookResponse, error) {
	fields := map[string]any{}
	if req.Title != nil {
		fields["title"] = req.GetTitle()
	}
	if req.Isbn != nil {
		fields["isbn"] = req.GetIsbn()
	}
	if req.Year != nil {
		fields["year"] = req.GetYear()
	}
	if req.Monitored != nil {
		fields["monitored"] = req.GetMonitored()
	}
	b, err := s.m.store.UpdateBook(ctx, req.GetId(), fields)
	if err != nil {
		return nil, err
	}
	return &booksv1.UpdateBookResponse{Book: s.m.bookWithFiles(ctx, b)}, nil
}

func (s *bookServer) RemoveBook(ctx context.Context, req *booksv1.RemoveBookRequest) (*booksv1.RemoveBookResponse, error) {
	if err := s.m.store.RemoveBookFiles(ctx, req.GetId(), s.m.libraryRoot(), req.GetDeleteFiles()); err != nil {
		return nil, err
	}
	return &booksv1.RemoveBookResponse{Success: true}, nil
}

func (s *bookServer) ScanLibrary(ctx context.Context, _ *booksv1.ScanLibraryRequest) (*booksv1.ScanLibraryResponse, error) {
	res, err := s.m.ScanLibrary(ctx)
	if err != nil {
		return nil, err
	}
	return &booksv1.ScanLibraryResponse{
		FilesFound: int32(res.FilesFound), FilesImported: int32(res.FilesImported),
		FilesSkipped: int32(res.FilesSkipped), FilesRemoved: int32(res.FilesRemoved),
	}, nil
}

func (s *bookServer) ListBookFiles(ctx context.Context, req *booksv1.ListBookFilesRequest) (*booksv1.ListBookFilesResponse, error) {
	files, err := s.m.store.ListBookFiles(ctx, req.GetBookId())
	if err != nil {
		return nil, err
	}
	out := make([]*booksv1.BookFile, 0, len(files))
	for _, f := range files {
		out = append(out, toPBBookFile(f))
	}
	return &booksv1.ListBookFilesResponse{Files: out}, nil
}

func (s *bookServer) ListMissing(ctx context.Context, req *booksv1.ListMissingRequest) (*booksv1.ListMissingResponse, error) {
	page := int(req.GetPage())
	if page < 1 {
		page = 1
	}
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 {
		pageSize = 100
	}
	items, total, err := s.m.store.ListMissingBooksFiltered(ctx, page, pageSize, req.GetAuthorId())
	if err != nil {
		return nil, err
	}
	out := make([]*booksv1.MissingBookItem, 0, len(items))
	for _, it := range items {
		out = append(out, &booksv1.MissingBookItem{
			BookId: it.BookID, AuthorId: it.AuthorID, Title: it.Title,
			AuthorName: it.AuthorName, Year: it.Year,
		})
	}
	return &booksv1.ListMissingResponse{
		Items: out, Total: int32(total), Page: int32(page), PageSize: int32(pageSize),
	}, nil
}

func (s *bookServer) ImportBookFile(ctx context.Context, req *booksv1.ImportBookFileRequest) (*booksv1.ImportBookFileResponse, error) {
	f, err := s.m.store.ImportBookFile(ctx, req.GetBookId(), req.GetPath(), s.m.libraryRoot())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	return &booksv1.ImportBookFileResponse{File: toPBBookFile(f)}, nil
}

func (m *Module) bookWithFiles(ctx context.Context, b *Book) *booksv1.Book {
	pb := toPBBook(b)
	if m.store == nil {
		return pb
	}
	files, err := m.store.ListBookFiles(ctx, b.ID)
	if err != nil {
		return pb
	}
	for _, f := range files {
		pb.Files = append(pb.Files, toPBBookFile(f))
	}
	return pb
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

func toPBBookFile(f *BookFile) *booksv1.BookFile {
	return &booksv1.BookFile{Id: f.ID, Title: f.Title, Path: f.Path}
}
