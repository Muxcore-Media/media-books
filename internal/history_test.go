package internal

import (
	"path/filepath"
	"testing"

	mediaadminv1 "github.com/Muxcore-Media/contracts-media-admin/gen/muxcore/media/admin/v1"
)

func TestAuthorHistoryRecordsAddAndImport(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	s, err := OpenStore(ctx, filepath.Join(dir, "books.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	m := &Module{store: s}
	au, err := s.AddAuthor(ctx, Author{Name: "Le Guin", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.ListHistory(ctx, &mediaadminv1.ListHistoryRequest{ItemId: au.ID, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetRecords()) != 1 || got.GetRecords()[0].GetEventType() != mediaadminv1.HistoryEventType_HISTORY_EVENT_TYPE_IMPORT {
		t.Fatalf("history=%+v", got.GetRecords())
	}
}
