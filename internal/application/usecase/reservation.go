package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/application/port"
	"github.com/code-sena/design-software-training-environment-service/internal/domain/model"
)

var (
	ErrReservationOverlap            = errors.New("reservation overlaps an existing active reservation")
	ErrReservationValidation         = errors.New("invalid reservation")
	ErrReservationInvalidEnvironment = errors.New("referenced environment_id does not exist")
	ErrReservationNotFound           = errors.New("reservation not found")
	ErrReservationQuery              = errors.New("invalid reservation query")
)

type reservationUseCase struct {
	repo    port.ReservationRepository
	envRepo port.EnvironmentRepository
}

func NewReservationUseCase(repo port.ReservationRepository, envRepo port.EnvironmentRepository) port.ReservationUseCase {
	return &reservationUseCase{repo: repo, envRepo: envRepo}
}

func (u *reservationUseCase) Create(ctx context.Context, command port.CreateReservationCommand) (*model.Reservation, error) {
	startTime, err := model.NormalizeReservationClock(command.StartTime)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReservationValidation, model.ErrInvalidReservationStartTime)
	}
	endTime, err := model.NormalizeReservationClock(command.EndTime)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReservationValidation, model.ErrInvalidReservationEndTime)
	}

	now := time.Now().UTC()
	reservation := &model.Reservation{
		ID:              newUUID(),
		EnvironmentID:   strings.TrimSpace(command.EnvironmentID),
		ReservationDate: dateOnly(command.ReservationDate),
		StartTime:       startTime,
		EndTime:         endTime,
		Reason:          command.Reason,
		RequesterID:     strings.TrimSpace(command.RequesterID),
		Status:          model.ReservationStatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := reservation.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReservationValidation, err)
	}

	if u.envRepo != nil {
		environment, err := u.envRepo.FindByID(ctx, reservation.EnvironmentID)
		if err != nil {
			return nil, err
		}
		if environment == nil {
			return nil, ErrReservationInvalidEnvironment
		}
	}

	if err := u.repo.Create(ctx, reservation); err != nil {
		return nil, err
	}
	return reservation, nil
}

func (u *reservationUseCase) GetByID(ctx context.Context, id, trainingCenterID string) (*model.Reservation, error) {
	id = strings.TrimSpace(id)
	trainingCenterID = strings.TrimSpace(trainingCenterID)
	if !model.IsValidUUID(id) {
		return nil, ErrReservationNotFound
	}
	if trainingCenterID != "" && !model.IsValidUUID(trainingCenterID) {
		return nil, ErrReservationQuery
	}
	item, err := u.repo.FindByID(ctx, id, trainingCenterID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrReservationNotFound
	}
	return item, nil
}

func (u *reservationUseCase) Query(ctx context.Context, query port.ReservationListQuery) (*port.ReservationPage, error) {
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 100 {
		return nil, ErrReservationQuery
	}

	query.EnvironmentID = strings.TrimSpace(query.EnvironmentID)
	query.RequesterID = strings.TrimSpace(query.RequesterID)
	query.Status = strings.TrimSpace(query.Status)
	query.TrainingCenterID = strings.TrimSpace(query.TrainingCenterID)

	if query.EnvironmentID != "" && !model.IsValidUUID(query.EnvironmentID) {
		return nil, ErrReservationQuery
	}
	if query.RequesterID != "" && !model.IsValidUUID(query.RequesterID) {
		return nil, ErrReservationQuery
	}
	if query.TrainingCenterID != "" && !model.IsValidUUID(query.TrainingCenterID) {
		return nil, ErrReservationQuery
	}
	if query.Status != "" {
		switch query.Status {
		case model.ReservationStatusPending, model.ReservationStatusConfirmed, model.ReservationStatusCancelled:
		default:
			return nil, ErrReservationQuery
		}
	}
	if query.From != nil && query.To != nil && !query.From.Before(*query.To) {
		return nil, ErrReservationQuery
	}

	allowedSort := map[string]struct{}{
		"id": {}, "environment_id": {}, "requester_id": {}, "reservation_date": {},
		"start_time": {}, "end_time": {}, "status": {}, "created_at": {}, "updated_at": {},
	}
	if query.SortField == "" {
		query.SortField = "created_at"
		query.SortDescending = true
	}
	if _, ok := allowedSort[query.SortField]; !ok {
		return nil, ErrReservationQuery
	}

	page, err := u.repo.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	if page.Items == nil {
		page.Items = []model.Reservation{}
	}
	return page, nil
}
