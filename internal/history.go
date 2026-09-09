package internal

import (
	"context"

	mediaadminv1 "github.com/Muxcore-Media/contracts-media-admin/gen/muxcore/media/admin/v1"
)

func (m *Module) ListHistory(ctx context.Context, req *mediaadminv1.ListHistoryRequest) (*mediaadminv1.ListHistoryResponse, error) {
	if m.store == nil {
		return nil, errString("not initialized")
	}
	page := int(req.GetPage())
	if page < 1 {
		page = 1
	}
	pageSize := int(req.GetPageSize())
	if pageSize < 1 {
		pageSize = 20
	}
	rows, total, err := m.store.scanHistory(ctx, page, pageSize, req.GetItemId(), historyEventFilter(req.GetEventType()))
	if err != nil {
		return nil, err
	}
	records := make([]*mediaadminv1.HistoryRecord, 0, len(rows))
	for _, r := range rows {
		records = append(records, &mediaadminv1.HistoryRecord{
			ItemId: r["item_id"], EventType: storeEventToProto(r["event_type"]),
			SourceTitle: r["source_title"], Quality: r["quality"], CreatedAt: r["date"],
		})
	}
	return &mediaadminv1.ListHistoryResponse{
		Records: records, Total: int32(total), Page: int32(page), PageSize: int32(pageSize),
	}, nil
}

func historyEventFilter(et mediaadminv1.HistoryEventType) string {
	switch et {
	case mediaadminv1.HistoryEventType_HISTORY_EVENT_TYPE_GRAB:
		return historyGrab
	case mediaadminv1.HistoryEventType_HISTORY_EVENT_TYPE_IMPORT:
		return historyImport
	case mediaadminv1.HistoryEventType_HISTORY_EVENT_TYPE_DELETE_ITEM:
		return historyDeleteItem
	case mediaadminv1.HistoryEventType_HISTORY_EVENT_TYPE_DELETE_FILE:
		return historyDeleteFile
	default:
		return ""
	}
}

func storeEventToProto(ev string) mediaadminv1.HistoryEventType {
	switch ev {
	case historyGrab:
		return mediaadminv1.HistoryEventType_HISTORY_EVENT_TYPE_GRAB
	case historyImport:
		return mediaadminv1.HistoryEventType_HISTORY_EVENT_TYPE_IMPORT
	case historyDeleteItem:
		return mediaadminv1.HistoryEventType_HISTORY_EVENT_TYPE_DELETE_ITEM
	case historyDeleteFile:
		return mediaadminv1.HistoryEventType_HISTORY_EVENT_TYPE_DELETE_FILE
	default:
		return mediaadminv1.HistoryEventType_HISTORY_EVENT_TYPE_UNSPECIFIED
	}
}

type errString string

func (e errString) Error() string { return string(e) }
