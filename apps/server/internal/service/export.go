package service

import (
	"context"
	"math"

	exportdomain "github.com/Actify/echonote/apps/server/internal/domain/export"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	maxSelectedExportSegments = 200
	exportExcerptSegments     = 4
)

type ExportService struct {
	repository *repository.ExportRepository
}

type ExportRequest struct {
	Options    exportdomain.Options
	SegmentIDs []pgtype.UUID
}

func NewExportService(repository *repository.ExportRepository) *ExportService {
	return &ExportService{repository: repository}
}

func (service *ExportService) Export(
	ctx context.Context,
	userID, episodeID pgtype.UUID,
	request ExportRequest,
) (exportdomain.Output, error) {
	if service == nil || service.repository == nil || !userID.Valid || !episodeID.Valid {
		return exportdomain.Output{}, exportdomain.ErrInvalidOptions
	}
	if err := request.Options.Validate(); err != nil {
		return exportdomain.Output{}, err
	}
	if len(request.SegmentIDs) > maxSelectedExportSegments || hasDuplicateExportIDs(request.SegmentIDs) {
		return exportdomain.Output{}, exportdomain.ErrInvalidOptions
	}
	snapshot := repository.ExportSnapshotRequest{}
	switch request.Options.Mode {
	case exportdomain.ModeNotesOnly:
		if len(request.SegmentIDs) > 0 {
			return exportdomain.Output{}, exportdomain.ErrInvalidOptions
		}
		snapshot.NeedNotes = true
	case exportdomain.ModeOrganizedNote:
		snapshot.NeedNotes = request.Options.IncludeUserNotes
		snapshot.NeedArtifact = request.Options.IncludeSummary || request.Options.IncludeKeyPoints || request.Options.IncludeWorthReviewing
		snapshot.NeedTranscript = request.Options.IncludeTranscript
		if !snapshot.NeedTranscript && len(request.SegmentIDs) > 0 {
			return exportdomain.Output{}, exportdomain.ErrInvalidOptions
		}
		snapshot.SegmentIDs = request.SegmentIDs
		snapshot.SegmentLimit = exportExcerptSegments
		if len(request.SegmentIDs) > 0 {
			snapshot.SegmentLimit = int32(len(request.SegmentIDs))
		}
	case exportdomain.ModeSelectedTranscript:
		if len(request.SegmentIDs) == 0 {
			return exportdomain.Output{}, exportdomain.ErrInvalidOptions
		}
		snapshot.NeedTranscript, snapshot.SegmentIDs = true, request.SegmentIDs
		snapshot.SegmentLimit = int32(len(request.SegmentIDs))
	case exportdomain.ModeFullTranscript:
		if len(request.SegmentIDs) > 0 {
			return exportdomain.Output{}, exportdomain.ErrInvalidOptions
		}
		snapshot.NeedTranscript, snapshot.SegmentLimit = true, math.MaxInt32
	default:
		return exportdomain.Output{}, exportdomain.ErrInvalidOptions
	}
	input, err := service.repository.Snapshot(ctx, userID, episodeID, snapshot)
	if err != nil {
		return exportdomain.Output{}, err
	}
	return exportdomain.Compose(input, request.Options)
}

func hasDuplicateExportIDs(ids []pgtype.UUID) bool {
	seen := make(map[[16]byte]struct{}, len(ids))
	for _, id := range ids {
		if !id.Valid {
			return true
		}
		if _, exists := seen[id.Bytes]; exists {
			return true
		}
		seen[id.Bytes] = struct{}{}
	}
	return false
}
